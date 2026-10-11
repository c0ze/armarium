// Command armarium is a small self-hosted library server for comics and ebooks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/covers"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/httpapi"
	"github.com/c0ze/armarium/internal/scan"
	"github.com/c0ze/armarium/internal/store"
	"github.com/c0ze/armarium/web"
)

// version is set by the Makefile via -ldflags.
var version = "dev"

const usage = `usage: armarium [--config armarium.toml] <command>

commands:
  serve                     run the server (default)
  scan [library]            scan libraries once and exit
  token add <name>          create an API token (printed once)
  token list | token rm <name>
  library list              libraries in the database, configured or not
  library rename <old> <new>   keep a library's items and progress under a new name
  library rm <name>         delete a library with its items and reading progress
  hash-password             read a password on stdin, print its argon2id hash
  import-skrivist --db reader_index.db --library <name>
  import-kavita   --db kavita.db --library <name> --root <kavita path prefix>
  version
  healthcheck               exit 0 if the local server answers /healthz (for Docker)
`

// app is everything a command needs, opened from the config.
type app struct {
	cfg     config.Config
	store   *store.Store
	scanner *scan.Scanner
	log     *slog.Logger
}

func main() {
	fs := flag.NewFlagSet("armarium", flag.ExitOnError)
	cfgPath := fs.String("config", os.Getenv("ARMARIUM_CONFIG"), "path to armarium.toml")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	fs.Parse(os.Args[1:])
	args := fs.Args()
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd { // commands that need no config or database
	case "hash-password":
		exit(hashPassword())
	case "version":
		fmt.Println("armarium", version)
		exit(nil)
	case "healthcheck":
		exit(healthcheck())
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// library repairs the mismatch that open refuses, so it skips the check.
	a, err := open(*cfgPath, log, cmd != "library")
	if err != nil {
		exit(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	switch cmd {
	case "serve":
		err = a.serve(ctx)
	case "scan":
		only := ""
		if len(args) > 0 {
			only = args[0]
		}
		err = a.scanner.Run(ctx, only)
	case "token":
		err = a.token(ctx, args)
	case "library":
		err = a.library(ctx, args)
	case "import-skrivist":
		err = a.importSkrivist(ctx, args)
	case "import-kavita":
		err = a.importKavita(ctx, args)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	stop()
	a.store.Close() // os.Exit skips defers; close so the WAL is checkpointed
	exit(err)
}

func exit(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "armarium:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func open(cfgPath string, log *slog.Logger, sync bool) (*app, error) {
	if cfgPath == "" {
		if _, err := os.Stat("armarium.toml"); err == nil {
			cfgPath = "armarium.toml"
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
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
	if sync {
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
	return &app{cfg: cfg, store: st, scanner: sc, log: log}, nil
}

func (a *app) serve(ctx context.Context) error {
	cv := &covers.Service{
		Dir:       filepath.Join(a.cfg.DataDir, "covers"),
		MaxPixels: a.cfg.Limits.MaxImagePixels,
		MaxBytes:  a.cfg.Limits.MaxEntryBytes,
	}
	if cleared, err := a.store.ResetSessionsIfPasswordChanged(ctx, a.cfg.PasswordHash); err != nil {
		return err
	} else if cleared {
		a.log.Info("owner sessions cleared: the password changed")
	}
	// Page numbers stored before the root-first order move to the new numbers
	// once; a comic that cannot be opened now is retried at the next start.
	if err := a.scanner.RemapPageOrder(ctx); err != nil {
		a.log.Error("page order remap", "err", err)
	}
	srv := httpapi.New(a.cfg, a.store, a.scanner, cv, web.FS(), a.log)
	if a.cfg.PasswordHash == "" {
		a.log.Warn("no password_hash configured: the web UI cannot log in (tokens still work); run `armarium hash-password`")
	}
	if h := a.cfg.Scan.IntervalH; h > 0 {
		go a.scheduledScans(ctx, time.Duration(h)*time.Hour)
	}
	a.log.Info("armarium listening", "version", version, "addr", a.cfg.Listen, "libraries", len(a.cfg.Libraries))
	return srv.Serve(ctx)
}

// scheduledScans is off by default: scans cost NAS I/O, so they are opt-in.
func (a *app) scheduledScans(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.scanner.Run(ctx, ""); err != nil && !errors.Is(err, scan.ErrRunning) {
				a.log.Error("scheduled scan", "err", err)
			}
		}
	}
}
