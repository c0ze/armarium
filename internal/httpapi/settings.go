package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/c0ze/armarium/internal/config"
)

// Host is the desktop app around the server. A server started from the
// command line has none, and its Settings are read-only.
type Host interface {
	// Apply checks ch, writes the managed config, renames and removes
	// libraries in the database, and restarts the server on the new config
	// once the response is sent. An error means nothing changed; its text is
	// shown to the owner.
	Apply(ctx context.Context, ch Change) error
	// ChooseFolder shows the system's folder dialog on this computer.
	ChooseFolder(ctx context.Context) (string, error)
	Tailscale(ctx context.Context) TailscaleStatus
}

// Change is one save from setup or Settings.
type Change struct {
	Settings config.Settings `json:"settings"`
	// Renames maps a library's current name to its new one; its items and
	// reading progress move with it.
	Renames map[string]string `json:"renames,omitempty"`
	// Removals are libraries deleted together with their items and progress.
	Removals     []string `json:"removals,omitempty"`
	StartAtLogin *bool    `json:"startAtLogin,omitempty"`
}

// TailscaleStatus is what setup and Settings show about Tailscale.
type TailscaleStatus struct {
	Available bool   `json:"available"` // the tailscale command is installed
	Name      string `json:"name"`      // this machine's ts.net name, "" when not running
	Serving   bool   `json:"serving"`   // Armarium's `tailscale serve` entry is on
	Conflict  string `json:"conflict"`  // why Armarium won't touch the existing serve setup
}

// ErrCancelled is ChooseFolder's answer when the owner closes the dialog.
var ErrCancelled = errors.New("cancelled")

func (s *Server) editable() bool { return s.Host != nil && s.Cfg.Desktop != nil }

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	_, fromConfig, err := s.passwordHash(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts, err := s.Store.LibraryItemCounts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := map[string]any{
		"items":    counts,
		"editable": s.editable(),
		"settings": s.Cfg.Settings(),
		"derived": map[string]any{
			"listen": s.Cfg.Listen, "allowedHosts": s.Cfg.AllowedHosts, "publicUrl": s.Cfg.PublicURL,
		},
		"passwordFromConfig": fromConfig,
		"snippet":            s.snippet(),
	}
	if s.Host != nil {
		out["tailscale"] = s.Host.Tailscale(r.Context())
	}
	writeJSON(w, http.StatusOK, out)
}

// snippet is the TOML an owner with a hand-written config edits for phones.
func (s *Server) snippet() string {
	quote := func(v []string) string {
		q := make([]string, len(v))
		for i, x := range v {
			q[i] = strconv.Quote(x)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "allowed_hosts = %s\n", quote(s.Cfg.AllowedHosts))
	if s.Cfg.PublicURL != "" {
		fmt.Fprintf(&b, "public_url    = %q\n", s.Cfg.PublicURL)
	} else {
		b.WriteString("# public_url  = \"https://armarium.example.com\"\n")
	}
	fmt.Fprintf(&b, "cors_origins  = %s\n", quote(config.SkrivistOrigins))
	return b.String()
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	if !s.editable() {
		writeError(w, http.StatusForbidden, "this Armarium's settings are in armarium.toml")
		return
	}
	var ch Change
	if err := decodeJSON(r, &ch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	s.applyChange(w, r, ch, func() {
		port := ch.Settings.Port
		if port == 0 {
			port = config.DefaultPort
		}
		writeJSON(w, http.StatusOK, map[string]string{"reloadUrl": "http://127.0.0.1:" + strconv.Itoa(port) + "/settings"})
	})
}

// applyChange checks that every library is accounted for and hands ch to
// the host; done answers the request on success.
func (s *Server) applyChange(w http.ResponseWriter, r *http.Request, ch Change, done func()) {
	if s.scanning() {
		writeError(w, http.StatusConflict, "Wait for the scan to finish")
		return
	}
	if err := checkLibraries(s.Cfg.Libraries, ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Host.Apply(r.Context(), ch); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	done()
}

// checkLibraries refuses a change that would leave a library out without
// saying whether to rename or delete it: the server would refuse to start.
func checkLibraries(current []config.Library, ch Change) error {
	names := map[string]bool{}
	for _, l := range ch.Settings.Libraries {
		if strings.TrimSpace(l.Name) == "" {
			return errors.New("every library needs a name")
		}
		if names[l.Name] {
			return fmt.Errorf("two libraries are called %q", l.Name)
		}
		names[l.Name] = true
	}
	renamedTo := map[string]bool{}
	for from, to := range ch.Renames {
		if !slices.ContainsFunc(current, func(l config.Library) bool { return l.Name == from }) {
			return fmt.Errorf("there is no library called %q to rename", from)
		}
		if !names[to] {
			return fmt.Errorf("library %q is renamed to %q, which isn't in the list", from, to)
		}
		renamedTo[to] = true
	}
	for _, name := range ch.Removals {
		if names[name] {
			return fmt.Errorf("library %q can't be both kept and removed", name)
		}
		if _, ok := ch.Renames[name]; ok {
			return fmt.Errorf("library %q can't be both renamed and removed", name)
		}
	}
	for _, l := range current {
		_, renamed := ch.Renames[l.Name]
		kept := names[l.Name] && !renamedTo[l.Name]
		if !kept && !renamed && !slices.Contains(ch.Removals, l.Name) {
			return fmt.Errorf("library %q was left out: remove it or keep it", l.Name)
		}
	}
	return nil
}

type folderCheck struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind"`
	Calibre bool   `json:"calibre"`
	Error   string `json:"error"`
}

func (s *Server) checkFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	writeJSON(w, http.StatusOK, inspectFolder(body.Path))
}

// inspectFolder says whether path can be a library and guesses its kind from
// a bounded look at its first few levels.
func inspectFolder(path string) folderCheck {
	fail := func(msg string) folderCheck { return folderCheck{Error: msg} }
	if !filepath.IsAbs(path) {
		return fail("Use the full path to the folder.")
	}
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fail("That folder doesn't exist.")
	case errors.Is(err, fs.ErrPermission):
		return fail("Armarium isn't allowed to read that folder.")
	case err != nil:
		return fail("That folder can't be read right now.")
	case !fi.IsDir():
		return fail("That's a file, not a folder.")
	}
	if _, err := os.Stat(filepath.Join(path, "metadata.db")); err == nil {
		return folderCheck{OK: true, Kind: "books", Calibre: true}
	}
	f, err := os.Open(path)
	if err == nil {
		_, err = f.Readdirnames(1)
		f.Close()
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return fail("Armarium isn't allowed to read that folder.")
	case err != nil && !errors.Is(err, io.EOF):
		return fail("That folder can't be read right now.")
	}
	return folderCheck{OK: true, Kind: guessKind(path)}
}

// guessKind reads at most 200 entries, three levels deep, breadth first.
func guessKind(root string) string {
	const maxEntries, maxDepth = 200, 3
	comics, books, seen := 0, 0, 0
	level := []string{root}
	for depth := 0; depth < maxDepth && len(level) > 0 && seen < maxEntries; depth++ {
		var next []string
		for _, dir := range level {
			f, err := os.Open(dir)
			if err != nil {
				continue
			}
			entries, _ := f.ReadDir(maxEntries - seen)
			f.Close()
			for _, e := range entries {
				seen++
				if e.IsDir() {
					next = append(next, filepath.Join(dir, e.Name()))
					continue
				}
				switch strings.ToLower(filepath.Ext(e.Name())) {
				case ".cbz", ".cbr":
					comics++
				case ".epub", ".pdf", ".mobi", ".azw3":
					books++
				}
			}
			if seen >= maxEntries {
				break
			}
		}
		level = next
	}
	if comics > books {
		return "comics"
	}
	if books > 0 {
		return "books"
	}
	return ""
}

// chooseFolder opens the folder dialog, only for the owner sitting at the
// computer that runs Armarium: a loopback peer, a loopback Host and no
// proxy in between. `tailscale serve` connects from loopback too, so the Host
// and X-Forwarded-For checks are what keep tailnet visitors out.
func (s *Server) chooseFolder(w http.ResponseWriter, r *http.Request) {
	if s.Host == nil || !onThisComputer(r) {
		writeError(w, http.StatusForbidden, "Choose a folder on the computer running Armarium")
		return
	}
	path, err := s.Host.ChooseFolder(r.Context())
	if errors.Is(err, ErrCancelled) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func onThisComputer(r *http.Request) bool {
	if !peer(r).IsLoopback() || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch strings.Trim(strings.ToLower(host), "[]") {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
