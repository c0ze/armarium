package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/config"
)

func noConfigPassword(c *config.Config) { c.PasswordHash = "" }

func storePassword(t *testing.T, e *env, pw string) {
	t.Helper()
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.srv.Store.SetOwnerPassword(context.Background(), h); err != nil {
		t.Fatal(err)
	}
}

func loginWith(e *env, pw string) int {
	return e.do(req{method: "POST", path: "/api/login", body: `{"password":"` + pw + `"}`, origin: "https://armarium.test"}).Code
}

func TestLoginWithStoredPassword(t *testing.T) {
	e := newEnvWith(t, noConfigPassword)
	if c := loginWith(e, "stored secret"); c != 403 {
		t.Fatalf("no password anywhere: %d, want 403", c)
	}
	storePassword(t, e, "stored secret")
	if c := loginWith(e, "stored secret"); c != 200 {
		t.Fatalf("stored password: %d", c)
	}
	s := decode[map[string]any](t, e.do(req{path: "/api/session"}))
	if s["passwordSet"] != true || s["passwordFromConfig"] != false || s["setupOpen"] != false {
		t.Fatalf("session: %v", s)
	}
}

func TestConfigPasswordWins(t *testing.T) {
	e := newEnv(t)
	storePassword(t, e, "stored secret")
	if c := loginWith(e, "stored secret"); c != 401 {
		t.Fatalf("stored password with a config hash: %d, want 401", c)
	}
	if c := loginWith(e, testPassword); c != 200 {
		t.Fatalf("config password: %d", c)
	}
	s := decode[map[string]any](t, e.do(req{path: "/api/session"}))
	if s["passwordFromConfig"] != true {
		t.Fatalf("session: %v", s)
	}
}

func TestSessionSetupOpenWithoutPassword(t *testing.T) {
	e := newEnvWith(t, noConfigPassword)
	s := decode[map[string]any](t, e.do(req{path: "/api/session"}))
	if s["setupOpen"] != true || s["passwordSet"] != false {
		t.Fatalf("session: %v", s)
	}
}

func changePassword(e *env, body string) (int, string) {
	w := e.do(req{method: "PUT", path: "/api/password", body: body, origin: "https://armarium.test", cookie: true})
	return w.Code, w.Header().Get("Set-Cookie")
}

func TestChangePassword(t *testing.T) {
	e := newEnvWith(t, noConfigPassword)
	storePassword(t, e, "first secret")
	e.login2("first secret")
	if c, _ := changePassword(e, `{"current":"wrong","new":"second secret"}`); c != 401 {
		t.Fatalf("wrong current: %d", c)
	}
	if c, _ := changePassword(e, `{"current":"first secret","new":"short"}`); c != 400 {
		t.Fatalf("short: %d", c)
	}
	c, set := changePassword(e, `{"current":"first secret","new":"second secret"}`)
	if c != 200 || !strings.HasPrefix(set, sessionCookie+"=") {
		t.Fatalf("change: %d %q", c, set)
	}
	if w := e.do(req{path: "/api/tokens", cookie: true}); w.Code != 401 {
		t.Fatalf("old cookie still works: %d", w.Code)
	}
	e.cookie = strings.Split(set, ";")[0]
	if w := e.do(req{path: "/api/tokens", cookie: true}); w.Code != 200 {
		t.Fatalf("new cookie: %d", w.Code)
	}
	if c := loginWith(e, "second secret"); c != 200 {
		t.Fatalf("login with new password: %d", c)
	}
}

func TestChangePasswordRefusedWithConfigHash(t *testing.T) {
	e := newEnv(t)
	e.login()
	w := e.do(req{method: "PUT", path: "/api/password", body: `{"current":"` + testPassword + `","new":"second secret"}`, origin: "https://armarium.test", cookie: true})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "password_hash is set in armarium.toml") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestChangePasswordCSRF(t *testing.T) {
	e := newEnvWith(t, noConfigPassword)
	storePassword(t, e, "first secret")
	e.login2("first secret")
	body := `{"current":"first secret","new":"second secret"}`
	if w := e.do(req{method: "PUT", path: "/api/password", body: body, origin: "https://evil.example", cookie: true}); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	if w := e.do(req{method: "PUT", path: "/api/password", body: body, origin: "https://armarium.test"}); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := e.do(req{method: "PUT", path: "/api/password", body: body, origin: "https://armarium.test", token: e.token}); w.Code != 403 {
		t.Fatalf("token: %d", w.Code)
	}
}
