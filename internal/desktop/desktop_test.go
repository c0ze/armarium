package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/httpapi"
	"github.com/c0ze/armarium/internal/scan"
	tu "github.com/c0ze/armarium/internal/testutil"
)

func TestPathsFor(t *testing.T) {
	env := map[string]string{"LOCALAPPDATA": `C:\Users\ana\AppData\Local`}
	getenv := func(k string) string { return env[k] }
	w, err := pathsFor("windows", getenv, `C:\Users\ana`)
	if err != nil {
		t.Fatal(err)
	}
	if w.Config != filepath.Join(`C:\Users\ana\AppData\Local`, "Armarium", "armarium.toml") ||
		w.Data != filepath.Join(`C:\Users\ana\AppData\Local`, "Armarium", "data") ||
		w.Logs != filepath.Join(`C:\Users\ana\AppData\Local`, "Armarium", "logs") {
		t.Errorf("windows: %+v", w)
	}
	m, _ := pathsFor("darwin", getenv, "/Users/ana")
	support := filepath.Join("/Users/ana", "Library", "Application Support", "Armarium")
	if m.Config != filepath.Join(support, "armarium.toml") || m.Data != filepath.Join(support, "data") ||
		m.Logs != filepath.Join("/Users/ana", "Library", "Logs", "Armarium") {
		t.Errorf("darwin: %+v", m)
	}
	if _, err := pathsFor("windows", func(string) string { return "" }, `C:\Users\ana`); err == nil {
		t.Error("windows without LOCALAPPDATA")
	}
}

func TestLockExclusive(t *testing.T) {
	dir := t.TempDir()
	release, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrRunning) {
		t.Fatalf("second lock: %v", err)
	}
	release()
	again, err := Lock(dir)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}

func TestLogFileRotates(t *testing.T) {
	dir := t.TempDir()
	f, err := NewLogFile(dir, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 39) + "\n"
	for range 8 { // 320 bytes: rotates at 100
		io.WriteString(f, line)
	}
	f.Close()
	names, _ := filepath.Glob(filepath.Join(dir, "armarium*.log"))
	want := []string{"armarium.1.log", "armarium.2.log", "armarium.log"}
	var got []string
	for _, n := range names {
		got = append(got, filepath.Base(n))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files %v", got)
	}
	for _, n := range names {
		if fi, _ := os.Stat(n); fi.Size() > 100 {
			t.Errorf("%s is %d bytes", n, fi.Size())
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// harness runs a Supervisor in temp folders and records its states.
type harness struct {
	t      *testing.T
	s      *Supervisor
	mu     sync.Mutex
	states []State
	done   chan error
	cancel context.CancelFunc
}

func start(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, done: make(chan error, 1)}
	h.s = &Supervisor{
		Paths:       Paths{Config: filepath.Join(dir, "armarium.toml"), Data: filepath.Join(dir, "data"), Logs: filepath.Join(dir, "logs")},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultPort: freePort(t),
		OnState: func(st State) {
			h.mu.Lock()
			h.states = append(h.states, st)
			h.mu.Unlock()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() { h.done <- h.s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(10 * time.Second):
			t.Error("supervisor did not stop")
		}
	})
	h.wait(func(st State) bool { return st.Running })
	return h
}

// wait returns the first state after `after` that matches.
func (h *harness) waitFrom(after int, ok func(State) bool) (State, int) {
	h.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		for i := after; i < len(h.states); i++ {
			if ok(h.states[i]) {
				st := h.states[i]
				h.mu.Unlock()
				return st, i + 1
			}
		}
		h.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.t.Fatalf("state not reached; states: %+v", h.states)
	return State{}, 0
}

func (h *harness) wait(ok func(State) bool) State { st, _ := h.waitFrom(0, ok); return st }

func (h *harness) count() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.states) }

func TestSupervisorFirstRunWritesManagedConfig(t *testing.T) {
	h := start(t)
	st := h.wait(func(st State) bool { return st.SetupURL != "" })
	if !strings.Contains(st.SetupURL, "/setup#code=") || !strings.HasPrefix(st.URL, "http://127.0.0.1:") {
		t.Fatalf("state %+v", st)
	}
	c, err := config.Load(h.s.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if c.Desktop == nil || c.Desktop.Network != "local" || c.Desktop.Port != h.s.DefaultPort {
		t.Fatalf("config %+v", c.Desktop)
	}
}

func comicsFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tu.WriteZip(t, dir, "Saga/Saga #01.cbz", tu.Entry{Name: "1.png", Data: tu.PNG(2, 2, 1)})
	tu.WriteZip(t, dir, "Saga/Saga #02.cbz", tu.Entry{Name: "1.png", Data: tu.PNG(2, 2, 1)})
	return dir
}

// apply saves ch and waits for the server to come back on the new config.
func (h *harness) apply(ch httpapi.Change) {
	h.t.Helper()
	n := h.count()
	if err := h.s.Apply(context.Background(), ch); err != nil {
		h.t.Fatal(err)
	}
	h.waitFrom(n, func(st State) bool { return !st.Running })
	h.waitFrom(n, func(st State) bool { return st.Running })
}

func (h *harness) items() map[string]int {
	h.t.Helper()
	a := h.s.current()
	// The supervisor may already be scanning a new library; wait and scan again.
	for {
		err := a.Scanner.Run(context.Background(), "")
		if err == nil {
			break
		}
		if !errors.Is(err, scan.ErrRunning) {
			h.t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	counts, err := a.Store.LibraryItemCounts(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return counts
}

func TestApplyAddsLibraryAndReloads(t *testing.T) {
	h := start(t)
	comics := comicsFolder(t)
	s := config.Settings{Network: "local", Port: h.s.DefaultPort, Libraries: []config.Library{{Name: "Comics", Root: comics, Kind: "comics"}}}
	h.apply(httpapi.Change{Settings: s})
	if got := h.items(); got["Comics"] != 2 {
		t.Fatalf("items %v", got)
	}
	c, _ := config.Load(h.s.Paths.Config)
	if len(c.Libraries) != 1 || c.Libraries[0].Root != comics {
		t.Fatalf("config libraries %+v", c.Libraries)
	}
}

func TestApplyRenameKeepsItems(t *testing.T) {
	h := start(t)
	comics := comicsFolder(t)
	s := config.Settings{Network: "local", Port: h.s.DefaultPort, Libraries: []config.Library{{Name: "Comics", Root: comics, Kind: "comics"}}}
	h.apply(httpapi.Change{Settings: s})
	before := h.items()["Comics"]
	s.Libraries[0].Name = "Graphic novels"
	h.apply(httpapi.Change{Settings: s, Renames: map[string]string{"Comics": "Graphic novels"}})
	got := h.items()
	if got["Graphic novels"] != before || got["Comics"] != 0 {
		t.Fatalf("after rename %v (before %d)", got, before)
	}
}

func TestApplyRemovalDeletes(t *testing.T) {
	h := start(t)
	comics := comicsFolder(t)
	s := config.Settings{Network: "local", Port: h.s.DefaultPort, Libraries: []config.Library{{Name: "Comics", Root: comics, Kind: "comics"}}}
	h.apply(httpapi.Change{Settings: s})
	h.items()
	s.Libraries = nil
	h.apply(httpapi.Change{Settings: s, Removals: []string{"Comics"}})
	if got := h.items(); len(got) != 0 {
		t.Fatalf("after removal %v", got)
	}
}

func TestApplyRefusesBusyPort(t *testing.T) {
	h := start(t)
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	before, _ := os.ReadFile(h.s.Paths.Config)
	err = h.s.Apply(context.Background(), httpapi.Change{Settings: config.Settings{Network: "local", Port: port}})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("Port %d is in use", port)) {
		t.Fatalf("apply: %v", err)
	}
	if after, _ := os.ReadFile(h.s.Paths.Config); string(after) != string(before) {
		t.Fatal("config changed")
	}
}

func TestApplyRollsBackWhenListenFails(t *testing.T) {
	h := start(t)
	oldURL := h.wait(func(st State) bool { return st.Running }).URL
	before, _ := os.ReadFile(h.s.Paths.Config)
	newPort := freePort(t)
	var squatter net.Listener
	h.s.beforeReload = func() { // the port is taken between the check and the restart
		squatter, _ = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", newPort))
	}
	defer func() {
		if squatter != nil {
			squatter.Close()
		}
	}()
	n := h.count()
	if err := h.s.Apply(context.Background(), httpapi.Change{Settings: config.Settings{Network: "local", Port: newPort}}); err != nil {
		t.Fatal(err)
	}
	st, _ := h.waitFrom(n, func(st State) bool { return st.Running && st.Err != "" })
	if st.URL != oldURL || !strings.Contains(st.Err, "in use") {
		t.Fatalf("after failed reload: %+v (old %s)", st, oldURL)
	}
	if after, _ := os.ReadFile(h.s.Paths.Config); string(after) != string(before) {
		t.Fatal("the previous config was not restored")
	}
}

func TestOpenBrowserCommand(t *testing.T) {
	name, args := browserCommand("darwin", "http://127.0.0.1:8580/")
	if name != "open" || len(args) != 1 {
		t.Errorf("darwin: %s %v", name, args)
	}
	name, args = browserCommand("windows", "http://127.0.0.1:8580/")
	if name != "rundll32" || args[0] != "url.dll,FileProtocolHandler" {
		t.Errorf("windows: %s %v", name, args)
	}
	_ = runtime.GOOS
}

func TestFriendly(t *testing.T) {
	if got := friendly(errors.New("listen tcp 127.0.0.1:8581: bind: address already in use")); !strings.HasPrefix(got, "Port 8581 is in use") {
		t.Errorf("%q", got)
	}
	if got := friendly(errors.New("listen tcp :8580: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.")); !strings.HasPrefix(got, "Port 8580 is in use") {
		t.Errorf("%q", got)
	}
	if got := friendly(errors.New("config: bad")); got != "config: bad" {
		t.Errorf("%q", got)
	}
}

func TestApplyScansNewLibraryAfterRestart(t *testing.T) {
	h := start(t)
	comics := comicsFolder(t)
	s := config.Settings{Network: "local", Port: h.s.DefaultPort, Libraries: []config.Library{{Name: "Comics", Root: comics, Kind: "comics"}}}
	h.apply(httpapi.Change{Settings: s})
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if a := h.s.current(); a != nil {
			if counts, _ := a.Store.LibraryItemCounts(context.Background()); counts["Comics"] == 2 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the new library was not scanned after the restart")
}
