package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/c0ze/armarium/internal/config"
	tu "github.com/c0ze/armarium/internal/testutil"
)

// managedEnv is a desktop-app server: a [desktop] config and a host.
func managedEnv(t *testing.T) (*env, *fakeHost) {
	t.Helper()
	e := newEnvWith(t, func(c *config.Config) {
		c.Desktop = &config.Desktop{Network: "local", Port: 8580}
		c.AllowedHosts = append(c.AllowedHosts, "127.0.0.1")
	})
	h := &fakeHost{folder: "/Users/owner/Comics"}
	e.srv.Host = h
	e.h = e.srv.Handler()
	e.login()
	return e, h
}

func changeJSON(t *testing.T, ch Change) string {
	t.Helper()
	b, err := json.Marshal(ch)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSettingsReadOnlyWithoutHost(t *testing.T) {
	e := newEnv(t)
	e.login()
	w := e.do(req{path: "/api/settings", cookie: true})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := decode[map[string]any](t, w)
	if got["editable"] != false || !strings.Contains(got["snippet"].(string), "allowed_hosts") {
		t.Fatalf("read-only settings: %v", got)
	}
	ch := Change{Settings: e.srv.Cfg.Settings()}
	if w := e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, ch), origin: "https://armarium.test", cookie: true}); w.Code != 403 {
		t.Fatalf("PUT on a hand-written config: %d", w.Code)
	}
}

func TestSettingsPutAppliesChange(t *testing.T) {
	e, h := managedEnv(t)
	w := e.do(req{path: "/api/settings", cookie: true})
	if got := decode[map[string]any](t, w); got["editable"] != true {
		t.Fatalf("managed settings not editable: %v", got)
	}
	s := e.srv.Cfg.Settings()
	s.Network = "home"
	s.Libraries[0].Name = "Graphic novels"
	yes := true
	ch := Change{Settings: s, Renames: map[string]string{"Comics": "Graphic novels"}, StartAtLogin: &yes}
	w = e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, ch), origin: "https://armarium.test", cookie: true})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"reloadUrl":"http://127.0.0.1:8580/settings"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got, ok := h.last()
	if !ok || got.Settings.Network != "home" || got.Renames["Comics"] != "Graphic novels" || got.StartAtLogin == nil || !*got.StartAtLogin {
		t.Fatalf("host got %+v", got)
	}
}

func TestSettingsPutRefusesLeftOutLibrary(t *testing.T) {
	e, h := managedEnv(t)
	s := e.srv.Cfg.Settings()
	s.Libraries = s.Libraries[1:] // "Comics" dropped without saying what to do with it
	w := e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, Change{Settings: s}), origin: "https://armarium.test", cookie: true})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Comics") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, ok := h.last(); ok {
		t.Fatal("host was called")
	}
	w = e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, Change{Settings: s, Removals: []string{"Comics"}}), origin: "https://armarium.test", cookie: true})
	if w.Code != 200 {
		t.Fatalf("explicit removal: %d %s", w.Code, w.Body)
	}
}

func TestSettingsPutRefusedWhileScanning(t *testing.T) {
	e, h := managedEnv(t)
	e.srv.scanning = func() bool { return true }
	w := e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, Change{Settings: e.srv.Cfg.Settings()}), origin: "https://armarium.test", cookie: true})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "Wait for the scan to finish") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, ok := h.last(); ok {
		t.Fatal("host was called during a scan")
	}
}

func TestSettingsPutReportsHostError(t *testing.T) {
	e, h := managedEnv(t)
	h.err = fmt.Errorf("port 9000 is in use by another program")
	w := e.do(req{method: "PUT", path: "/api/settings", body: changeJSON(t, Change{Settings: e.srv.Cfg.Settings()}), origin: "https://armarium.test", cookie: true})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "port 9000 is in use") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestSettingsNeedsOwner(t *testing.T) {
	e, _ := managedEnv(t)
	body := changeJSON(t, Change{Settings: e.srv.Cfg.Settings()})
	if w := e.do(req{path: "/api/settings"}); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := e.do(req{path: "/api/settings", token: e.token}); w.Code != 403 {
		t.Fatalf("token: %d", w.Code)
	}
	if w := e.do(req{method: "PUT", path: "/api/settings", body: body, origin: "https://evil.example", cookie: true}); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
}

func checkFolder(e *env, path string) map[string]any {
	b, _ := json.Marshal(map[string]string{"path": path})
	w := e.do(req{method: "POST", path: "/api/settings/check-folder", body: string(b), origin: "https://armarium.test", cookie: true})
	if w.Code != 200 {
		e.t.Fatalf("check-folder %s: %d %s", path, w.Code, w.Body)
	}
	return decode[map[string]any](e.t, w)
}

func TestCheckFolder(t *testing.T) {
	e, _ := managedEnv(t)
	comics, books, cal := t.TempDir(), t.TempDir(), t.TempDir()
	tu.WriteZip(t, comics, "Saga/Saga #01.cbz", tu.Entry{Name: "1.png", Data: tu.PNG(2, 2, 1)})
	tu.WriteZip(t, comics, "Saga/Saga #02.cbz", tu.Entry{Name: "1.png", Data: tu.PNG(2, 2, 1)})
	tu.WriteEPUB(t, books, "Author/Book.epub", tu.EPUBOpts{Title: "b", Chapters: []string{"<p>1</p>"}})
	os.WriteFile(filepath.Join(cal, "metadata.db"), []byte("x"), 0o600)
	file := filepath.Join(books, "note.txt")
	os.WriteFile(file, []byte("x"), 0o600)

	cases := []struct {
		path, kind, err string
		ok, calibre     bool
	}{
		{path: comics, ok: true, kind: "comics"},
		{path: books, ok: true, kind: "books"},
		{path: cal, ok: true, kind: "books", calibre: true},
		{path: filepath.Join(books, "missing"), err: "That folder doesn't exist."},
		{path: file, err: "That's a file, not a folder."},
		{path: "relative/path", err: "Use the full path to the folder."},
	}
	for _, c := range cases {
		got := checkFolder(e, c.path)
		if got["ok"] != c.ok || got["kind"] != c.kind || got["calibre"] != c.calibre || got["error"] != c.err {
			t.Errorf("%s: %v", c.path, got)
		}
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		locked := t.TempDir()
		os.Chmod(locked, 0)
		defer os.Chmod(locked, 0o700)
		if got := checkFolder(e, locked); got["ok"] != false || got["error"] != "Armarium isn't allowed to read that folder." {
			t.Errorf("unreadable: %v", got)
		}
	}
}

func TestCheckFolderIsQuickOnBigFolders(t *testing.T) {
	e, _ := managedEnv(t)
	big := t.TempDir()
	for i := range 5000 {
		os.WriteFile(filepath.Join(big, fmt.Sprintf("%05d.cbz", i)), nil, 0o600)
	}
	start := time.Now()
	got := checkFolder(e, big)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
	if got["kind"] != "comics" {
		t.Fatalf("%v", got)
	}
}

func chooseFolder(e *env, q req) int {
	q.method, q.path, q.body, q.cookie = "POST", "/api/settings/folder", "{}", true
	if q.origin == "" {
		q.origin = "http://" + q.host
	}
	return e.do(q).Code
}

func TestFolderDialogLocalOnly(t *testing.T) {
	e, h := managedEnv(t)
	if c := chooseFolder(e, req{host: "localhost:8580", remote: "127.0.0.1:5000"}); c != 200 {
		t.Fatalf("local: %d", c)
	}
	if c := chooseFolder(e, req{host: "127.0.0.1:8580", remote: "[::1]:5000"}); c != 200 {
		t.Fatalf("local v6 peer: %d", c)
	}
	if c := chooseFolder(e, req{host: "armarium.test", remote: "192.168.1.5:5000", origin: "https://armarium.test"}); c != 403 {
		t.Fatalf("remote: %d", c)
	}
	h.folder = ""
	if c := chooseFolder(e, req{host: "localhost:8580", remote: "127.0.0.1:5000"}); c != 204 {
		t.Fatalf("cancelled: %d", c)
	}
}

func TestFolderDialogRefusesProxiedLoopback(t *testing.T) {
	e, _ := managedEnv(t)
	e.srv.Cfg.AllowedHosts = append(e.srv.Cfg.AllowedHosts, "studio.tail1234.ts.net")
	e.h = e.srv.Handler()
	if c := chooseFolder(e, req{host: "localhost:8580", remote: "127.0.0.1:5000", headers: map[string]string{"X-Forwarded-For": "100.64.0.7"}}); c != 403 {
		t.Fatalf("forwarded: %d", c)
	}
	if c := chooseFolder(e, req{host: "studio.tail1234.ts.net", remote: "127.0.0.1:5000", origin: "https://studio.tail1234.ts.net"}); c != 403 {
		t.Fatalf("tailnet host: %d", c)
	}
}

func TestFolderDialogNeedsHost(t *testing.T) {
	e := newEnv(t)
	e.login()
	e.srv.Cfg.AllowedHosts = append(e.srv.Cfg.AllowedHosts, "127.0.0.1")
	e.h = e.srv.Handler()
	if c := chooseFolder(e, req{host: "localhost:8580", remote: "127.0.0.1:5000"}); c != 403 {
		t.Fatalf("without the desktop app: %d", c)
	}
}
