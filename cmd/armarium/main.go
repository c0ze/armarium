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
	"strings"
	"syscall"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/server"
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
  password                  set the owner password (stored in the database)
  hash-password             read a password on stdin, print its argon2id hash
  import-skrivist --db reader_index.db --library <name>
  import-kavita   --db kavita.db --library <name> --root <kavita path prefix>
  version
  healthcheck               exit 0 if the local server answers /healthz (for Docker)
`

// app is the opened server with the command-line commands on it.
type app struct{ *server.App }

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
		err = a.Serve(ctx, server.ServeOptions{UI: web.FS()})
	case "scan":
		only := ""
		if len(args) > 0 {
			only = args[0]
		}
		err = a.Scanner.Run(ctx, only)
	case "token":
		err = a.token(ctx, args)
	case "library":
		err = a.library(ctx, args)
	case "password":
		err = a.password(ctx)
	case "import-skrivist":
		err = a.importSkrivist(ctx, args)
	case "import-kavita":
		err = a.importKavita(ctx, args)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	stop()
	a.Close() // os.Exit skips defers; close so the WAL is checkpointed
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
	if len(cfg.Overrides) > 0 {
		log.Warn("environment overrides armarium.toml", "keys", strings.Join(cfg.Overrides, ", "))
	}
	s, err := server.Open(cfg, log, sync)
	if err != nil {
		return nil, err
	}
	s.Version = version
	return &app{s}, nil
}
