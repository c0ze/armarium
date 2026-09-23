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
	a, err := open(*cfgPath, log)
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

func open(cfgPath string, log *slog.Logger) (*app, error) {
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
	if err := st.SyncLibraries(context.Background(), libs); err != nil {
		st.Close()
		return nil, err
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
