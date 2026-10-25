// Command armarium-desktop is Armarium as a desktop app: the server runs
// while the user is logged in, with a menu-bar (macOS) or tray (Windows)
// icon to open it, pair a phone, change settings and quit.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strconv"

	"fyne.io/systray"
	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/desktop"
	"github.com/c0ze/armarium/web"
)

// version is set with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("armarium-desktop", version)
		return
	}
	paths, err := desktop.DefaultPaths()
	if err != nil {
		fail(err)
	}
	release, err := desktop.Lock(paths.Data)
	if errors.Is(err, desktop.ErrRunning) {
		// Already running: launching it again just opens Armarium.
		desktop.OpenBrowser(localURL(paths))
		return
	}
	if err != nil {
		fail(err)
	}
	defer release()
	logs, err := desktop.NewLogFile(paths.Logs, 5<<20, 3)
	if err != nil {
		fail(err)
	}
	defer logs.Close()
	var out io.Writer = logs
	if runtime.GOOS != "windows" { // a GUI exe on Windows has no stderr
		out = io.MultiWriter(logs, os.Stderr)
	}
	log := slog.New(slog.NewTextHandler(out, nil))
	log.Info("armarium desktop starting", "version", version, "config", paths.Config)

	ctx, cancel := context.WithCancel(context.Background())
	t := &tray{paths: paths, log: log, cancel: cancel, stopped: make(chan struct{})}
	t.sup = &desktop.Supervisor{
		Paths: paths, Log: log, UI: web.FS(), Version: version,
		OnState: t.onState, LoginItem: newLoginItem(),
	}
	systray.Run(func() { t.ready(ctx) }, t.exit)
}

// localURL is where this computer's browser reaches the configured port.
func localURL(paths desktop.Paths) string {
	port := config.DefaultPort
	if c, err := config.Load(paths.Config); err == nil && c.Desktop != nil && c.Desktop.Port != 0 {
		port = c.Desktop.Port
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "armarium:", err)
	os.Exit(1)
}
