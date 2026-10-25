package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/httpapi"
	"github.com/c0ze/armarium/internal/server"
	"github.com/c0ze/armarium/internal/tailscale"
)

// LoginItem starts Armarium when the user logs in.
type LoginItem interface {
	Enabled() bool
	SetEnabled(on bool) error
}

// State is what the tray shows.
type State struct {
	Running  bool
	URL      string // where this computer's browser reaches Armarium
	SetupURL string // the first-run setup link, while no password exists
	Err      string // why Armarium stopped, or why the last change was undone
}

// Supervisor runs the server on the managed config and restarts it when
// Settings change. It is the httpapi.Host of the server it runs.
type Supervisor struct {
	Paths       Paths
	Log         *slog.Logger
	UI          fs.FS
	Version     string
	DefaultPort int // 0 means config.DefaultPort
	OnState     func(State)
	LoginItem   LoginItem // nil where the platform has none

	beforeReload func() // test hook: runs just before a restart

	mu          sync.Mutex
	app         *server.App
	cancelServe context.CancelFunc
	state       State
	previous    []byte // the config before the last Apply, put back if the new one won't start
	carriedErr  string // shown with the next running state
	tsErr       string // why tailscale serve couldn't be set up
}

// Run serves until ctx is cancelled. The first run writes a managed config
// for this computer only, with no libraries; setup fills it in.
func (s *Supervisor) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.Paths.Config), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(s.Paths.Config); errors.Is(err, fs.ErrNotExist) {
		port := s.DefaultPort
		if port == 0 {
			port = config.DefaultPort
		}
		if err := config.WriteManaged(s.Paths.Config, config.Settings{Network: "local", Port: port}, s.Paths.Data); err != nil {
			return err
		}
	}
	for ctx.Err() == nil {
		started, err := s.runOnce(ctx)
		if ctx.Err() != nil {
			break
		}
		if err == nil {
			continue // a requested restart
		}
		msg := friendly(err)
		s.Log.Error("armarium stopped", "err", err)
		s.mu.Lock()
		prev := s.previous
		s.previous = nil
		s.mu.Unlock()
		if prev != nil && !started {
			// The new settings don't start: put the old ones back and say why.
			if werr := os.WriteFile(s.Paths.Config, prev, 0o600); werr == nil {
				s.mu.Lock()
				s.carriedErr = "Your last change was undone: " + msg
				s.mu.Unlock()
				continue
			}
		}
		s.setState(State{Err: msg})
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

// runOnce opens and serves the current config until a restart or ctx ends.
func (s *Supervisor) runOnce(ctx context.Context) (started bool, err error) {
	cfg, err := config.Load(s.Paths.Config)
	if err != nil {
		return false, err
	}
	app, err := server.Open(cfg, s.Log, true)
	if err != nil {
		return false, err
	}
	app.Version = s.Version
	serveCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.app, s.cancelServe = app, cancel
	s.mu.Unlock()
	url := "http://127.0.0.1:" + strconv.Itoa(port(cfg))
	err = app.Serve(serveCtx, server.ServeOptions{
		UI:   s.UI,
		Host: s,
		Ready: func(string) {
			started = true
			s.mu.Lock()
			msg := s.carriedErr
			s.carriedErr = ""
			s.mu.Unlock()
			s.setState(State{Running: true, URL: url, Err: msg})
		},
		OnSetup: func(link string) {
			s.mu.Lock()
			st := s.state
			s.mu.Unlock()
			st.SetupURL = link
			s.setState(st)
		},
	})
	cancel()
	app.Close()
	s.mu.Lock()
	s.app, s.cancelServe = nil, nil
	s.mu.Unlock()
	s.setState(State{})
	return started, err
}

func (s *Supervisor) setState(st State) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	if s.OnState != nil {
		s.OnState(st)
	}
}

// State is the latest state.
func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Scanning reports a running library scan.
func (s *Supervisor) Scanning() bool {
	if a := s.current(); a != nil {
		return a.Scanner.Status().Running
	}
	return false
}

func (s *Supervisor) current() *server.App {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.app
}

func port(c config.Config) int {
	if c.Desktop != nil && c.Desktop.Port != 0 {
		return c.Desktop.Port
	}
	return config.DefaultPort
}

func target(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) }

// Apply implements httpapi.Host: check the change, apply it, then restart.
func (s *Supervisor) Apply(ctx context.Context, ch httpapi.Change) error {
	app := s.current()
	if app == nil {
		return errors.New("Armarium is restarting; try again in a moment")
	}
	old := app.Cfg
	candidate := s.Paths.Config + ".new"
	defer os.Remove(candidate)
	if err := config.WriteManaged(candidate, ch.Settings, s.Paths.Data); err != nil {
		return err
	}
	next, err := config.Load(candidate)
	if err != nil {
		return err
	}
	oldPort, newPort := port(old), port(next)
	if newPort != oldPort {
		ln, err := net.Listen("tcp", next.Listen)
		if err != nil {
			return fmt.Errorf("Port %d is in use by another program. Choose another port.", newPort)
		}
		ln.Close()
	}
	wasTS := old.Desktop != nil && old.Desktop.Tailscale
	wantTS := ch.Settings.Tailscale
	tsOwnedOld := false
	if wasTS || wantTS {
		if tailscale.CLI() == "" {
			if wantTS {
				return errors.New("Tailscale isn't installed on this computer")
			}
		} else if st, err := tailscale.Serve(ctx, target(oldPort)); err == nil {
			tsOwnedOld = st.Owned
			if wantTS && st.Conflict != "" && newPort == oldPort {
				return errors.New(st.Conflict)
			}
		}
	}
	for from, to := range ch.Renames {
		if err := app.Store.RenameLibrary(ctx, from, to); err != nil {
			return err
		}
	}
	for _, name := range ch.Removals {
		if _, err := app.Store.RemoveLibrary(ctx, name); err != nil {
			return err
		}
	}
	prev, err := os.ReadFile(s.Paths.Config)
	if err != nil {
		return err
	}
	if err := os.Rename(candidate, s.Paths.Config); err != nil {
		return err
	}
	s.mu.Lock()
	s.previous = prev
	cancel := s.cancelServe
	s.mu.Unlock()
	if ch.StartAtLogin != nil && s.LoginItem != nil {
		if err := s.LoginItem.SetEnabled(*ch.StartAtLogin); err != nil {
			s.Log.Warn("start at login", "err", err)
		}
	}
	s.updateTailscale(ctx, tsOwnedOld && (!wantTS || newPort != oldPort), wantTS && (!wasTS || newPort != oldPort || !tsOwnedOld), newPort)
	// Restart once this request's response is on its way.
	time.AfterFunc(300*time.Millisecond, func() {
		if s.beforeReload != nil {
			s.beforeReload()
		}
		if cancel != nil {
			cancel()
		}
	})
	return nil
}

func (s *Supervisor) updateTailscale(ctx context.Context, off, on bool, port int) {
	if off {
		if err := tailscale.ServeOff(ctx); err != nil {
			s.Log.Warn("tailscale serve off", "err", err)
		}
	}
	msg := ""
	if on {
		if err := tailscale.ServeOn(ctx, target(port)); err != nil {
			s.Log.Warn("tailscale serve", "err", err)
			msg = "Tailscale couldn't serve Armarium. Check that HTTPS certificates are on for your tailnet, then run: " + tailscale.ServeCommand(target(port))
		}
	}
	s.mu.Lock()
	s.tsErr = msg
	s.mu.Unlock()
}

// ChooseFolder implements httpapi.Host.
func (s *Supervisor) ChooseFolder(ctx context.Context) (string, error) { return chooseFolder(ctx) }

// Tailscale implements httpapi.Host.
func (s *Supervisor) Tailscale(ctx context.Context) httpapi.TailscaleStatus {
	st := httpapi.TailscaleStatus{Available: tailscale.CLI() != ""}
	if !st.Available {
		return st
	}
	if st.Name = tailscale.Name(ctx); st.Name == "" {
		return st
	}
	p := config.DefaultPort
	if a := s.current(); a != nil {
		p = port(a.Cfg)
	}
	if ss, err := tailscale.Serve(ctx, target(p)); err == nil {
		st.Serving, st.Conflict = ss.Owned, ss.Conflict
	}
	s.mu.Lock()
	if s.tsErr != "" && !st.Serving {
		st.Conflict = s.tsErr
	}
	s.mu.Unlock()
	return st
}

// friendly turns a start-up error into a sentence for the tray and Settings.
func friendly(err error) string {
	msg := err.Error()
	if !strings.Contains(msg, "address already in use") && !strings.Contains(msg, "Only one usage of each socket address") {
		return msg
	}
	// "listen tcp 127.0.0.1:8580: bind: address already in use"
	if _, rest, ok := strings.Cut(msg, "listen tcp "); ok {
		addr, _, _ := strings.Cut(rest, ": ")
		if _, p, err := net.SplitHostPort(addr); err == nil {
			return fmt.Sprintf("Port %s is in use by another program. Quit it, or choose another port in Settings.", p)
		}
	}
	return "Armarium's port is in use by another program. Quit it, or choose another port in Settings."
}
