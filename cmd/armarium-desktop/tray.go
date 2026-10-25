package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/c0ze/armarium/internal/desktop"
)

type tray struct {
	paths   desktop.Paths
	log     *slog.Logger
	sup     *desktop.Supervisor
	cancel  context.CancelFunc
	stopped chan struct{}

	mu         sync.Mutex
	status     *systray.MenuItem
	openedOnce bool
	exiting    bool // menu updates stop: they would wait on the main thread exit blocks
}

const (
	statusRunning  = "Armarium is running"
	statusScanning = "Scanning your library…"
	statusSetup    = "Finish setup in your browser"
	statusStarting = "Starting…"
)

func (t *tray) ready(ctx context.Context) {
	if runtime.GOOS == "darwin" {
		systray.SetTemplateIcon(trayMac, trayMac)
	} else {
		systray.SetIcon(trayWin)
	}
	systray.SetTooltip("Armarium")
	listenForReopen(func() { t.browse("/") })

	open := systray.AddMenuItem("Open Armarium", "Open your library in the browser")
	pair := systray.AddMenuItem("Pair a phone…", "Show the QR code for the Skrivist apps")
	settings := systray.AddMenuItem("Settings…", "Libraries, who can read, password")
	t.mu.Lock()
	t.status = systray.AddMenuItem(statusStarting, "")
	t.status.Disable()
	t.mu.Unlock()
	systray.AddSeparator()
	login := systray.AddMenuItemCheckbox("Start at login", "Start Armarium when you log in", false)
	if t.sup.LoginItem == nil {
		login.Hide()
	} else if t.sup.LoginItem.Enabled() {
		login.Check()
	}
	logs := systray.AddMenuItem("Show log folder", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit Armarium", "Stop serving your library")

	go func() {
		defer close(t.stopped)
		if err := t.sup.Run(ctx); err != nil {
			t.log.Error("supervisor", "err", err)
			t.setStatus("Stopped: " + err.Error())
		}
	}()
	go t.watchScans(ctx)

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				t.browse("/")
			case <-pair.ClickedCh:
				t.browse("/admin#pair")
			case <-settings.ClickedCh:
				t.browse("/settings")
			case <-login.ClickedCh:
				on := !login.Checked()
				if err := t.sup.LoginItem.SetEnabled(on); err != nil {
					t.log.Warn("start at login", "err", err)
					continue
				}
				if on {
					login.Check()
				} else {
					login.Uncheck()
				}
			case <-logs.ClickedCh:
				desktop.OpenFolder(t.paths.Logs)
			case <-quit.ClickedCh:
				t.setStatus("Stopping…")
				t.cancel()
				select { // let the server close the database
				case <-t.stopped:
				case <-time.After(15 * time.Second):
				}
				systray.Quit()
				return
			}
		}
	}()
}

// exit runs when the app is quitting by any route (the menu, logout, a quit
// Apple Event): stop the server so the database closes cleanly.
//
// It runs on the main thread, and systray applies menu changes there and
// waits for them, so the menu must not change while it waits for the server.
func (t *tray) exit() {
	t.mu.Lock()
	t.exiting = true
	t.mu.Unlock()
	t.cancel()
	select {
	case <-t.stopped:
	case <-time.After(15 * time.Second):
	}
}

// browse opens a page of the running Armarium, or setup while it isn't set up.
func (t *tray) browse(path string) {
	st := t.sup.State()
	switch {
	case st.SetupURL != "":
		desktop.OpenBrowser(st.SetupURL)
	case st.URL != "":
		desktop.OpenBrowser(st.URL + path)
	}
}

func (t *tray) onState(st desktop.State) {
	switch {
	case st.Err != "" && !st.Running:
		t.setStatus("Stopped: " + st.Err)
	case st.Err != "":
		t.setStatus(st.Err)
	case st.SetupURL != "":
		t.setStatus(statusSetup)
		t.mu.Lock()
		first := !t.openedOnce
		t.openedOnce = true
		t.mu.Unlock()
		if first {
			desktop.OpenBrowser(st.SetupURL)
		}
	case st.Running:
		t.setStatus(statusRunning)
	default:
		t.setStatus(statusStarting)
	}
}

func (t *tray) watchScans(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	scanning := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			now := t.sup.Scanning()
			if now == scanning {
				continue
			}
			scanning = now
			if now {
				t.setStatus(statusScanning)
			} else {
				t.onState(t.sup.State())
			}
		}
	}
}

func (t *tray) setStatus(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status != nil && !t.exiting {
		t.status.SetTitle(s)
	}
}
