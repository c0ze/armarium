// Package server opens Armarium from a config and runs it: the command line
// and the desktop app both start the server through here.
package server

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
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/covers"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/httpapi"
	"github.com/c0ze/armarium/internal/scan"
	"github.com/c0ze/armarium/internal/store"
)

// App is everything a command needs, opened from the config.
type App struct {
	Cfg     config.Config
	Store   *store.Store
	Scanner *scan.Scanner
	Log     *slog.Logger
	Version string
}

// ServeOptions are what the caller adds to a running server.
type ServeOptions struct {
	UI fs.FS // built web app; nil serves no UI
	// Ready is called once the server listens, with its address.
	Ready func(addr string)
}

// Open opens the data folder and database for cfg. syncLibraries makes every
// configured library exist and refuses a library the database holds but the
// config no longer names; `armarium library` skips it to repair exactly that.
func Open(cfg config.Config, log *slog.Logger, syncLibraries bool) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "armarium.db"))
	if err != nil {
		return nil, err
	}
	libs := make([]store.Library, len(cfg.Libraries))
	for i, l := range cfg.Libraries {
		libs[i] = store.Library{Name: l.Name, Root: l.Root, Kind: l.Kind}
	}
	if syncLibraries {
		if err := st.SyncLibraries(context.Background(), libs); err != nil {
			st.Close()
			var unconfigured *store.UnconfiguredError
			if errors.As(err, &unconfigured) {
				name := strconv.Quote(unconfigured.Names[0])
				return nil, fmt.Errorf("%w. Put it back in the config, rename it with `armarium library rename %s NEW`, or delete it and its reading progress with `armarium library rm %s`", err, name, name)
			}
			return nil, err
		}
	}
	sc := &scan.Scanner{
		Store: st, Libraries: cfg.Libraries, Workers: cfg.Scan.Workers, Log: log,
		DirSleep: time.Duration(cfg.Scan.DirSleepMS) * time.Millisecond,
		Limits:   formats.Limits{MaxEntryBytes: cfg.Limits.MaxEntryBytes, MaxEntries: cfg.Limits.MaxEntries},
	}
	return &App{Cfg: cfg, Store: st, Scanner: sc, Log: log}, nil
}

// Close closes the database, checkpointing the WAL.
func (a *App) Close() error { return a.Store.Close() }

// Serve runs the HTTP server until ctx is cancelled.
func (a *App) Serve(ctx context.Context, opts ServeOptions) error {
	cv := &covers.Service{
		Dir:       filepath.Join(a.Cfg.DataDir, "covers"),
		MaxPixels: a.Cfg.Limits.MaxImagePixels,
		MaxBytes:  a.Cfg.Limits.MaxEntryBytes,
	}
	if cleared, err := a.Store.ResetSessionsIfPasswordChanged(ctx, a.Cfg.PasswordHash); err != nil {
		return err
	} else if cleared {
		a.Log.Info("owner sessions cleared: the password changed")
	}
	// Page numbers stored before the root-first order move to the new numbers
	// before anything is served; a comic that cannot be opened now is retried at
	// the next start or scan. A database error stops the start: serving would
	// resume the comics not yet renumbered on the wrong image.
	if err := a.Scanner.RemapPageOrder(ctx); err != nil {
		return fmt.Errorf("page order remap: %w", err)
	}
	srv := httpapi.New(a.Cfg, a.Store, a.Scanner, cv, opts.UI, a.Log)
	if a.Cfg.PasswordHash == "" {
		a.Log.Warn("no password_hash configured: the web UI cannot log in (tokens still work); run `armarium hash-password`")
	}
	ln, err := net.Listen("tcp", a.Cfg.Listen)
	if err != nil {
		return err
	}
	if h := a.Cfg.Scan.IntervalH; h > 0 {
		go a.scheduledScans(ctx, time.Duration(h)*time.Hour)
	}
	a.Log.Info("armarium listening", "version", a.Version, "addr", ln.Addr().String(), "libraries", len(a.Cfg.Libraries))
	if opts.Ready != nil {
		opts.Ready(ln.Addr().String())
	}
	return srv.ServeListener(ctx, ln)
}

// scheduledScans is off by default: scans cost NAS I/O, so they are opt-in.
func (a *App) scheduledScans(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.Scanner.Run(ctx, ""); err != nil && !errors.Is(err, scan.ErrRunning) {
				a.Log.Error("scheduled scan", "err", err)
			}
		}
	}
}
