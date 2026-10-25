package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/c0ze/armarium/internal/config"
)

func TestServeAnswersHealthzAndStops(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Listen = "127.0.0.1:0"
	a, err := Open(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, ServeOptions{Ready: func(addr string) { ready <- addr }}) }()
	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("serve returned early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("not ready")
	}
	res, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "ok\n" {
		t.Fatalf("healthz: %q", body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestSetPassword(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	a, err := Open(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx := context.Background()
	if err := a.SetPassword(ctx, "short"); err == nil {
		t.Fatal("a 5-character password was accepted")
	}
	if err := a.SetPassword(ctx, "long enough"); err != nil {
		t.Fatal(err)
	}
	if h, _ := a.PasswordHash(ctx); h == "" {
		t.Fatal("no password stored")
	}
	a.Cfg.PasswordHash = "$argon2id$from-config"
	err = a.SetPassword(ctx, "another one")
	if err == nil || !strings.Contains(err.Error(), "password_hash in the config wins") {
		t.Fatalf("with a config hash: %v", err)
	}
}
