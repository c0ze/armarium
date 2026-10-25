package httpapi

import (
	"testing"

	"github.com/c0ze/armarium/internal/config"
)

func TestHostsAcceptsOwnAddressOnlyWhenManaged(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/api/session", host: "127.0.0.1:8580"}); w.Code != 421 {
		t.Fatalf("unmanaged config admitted an unlisted own address: %d", w.Code)
	}
	e = newEnvWith(t, func(c *config.Config) { c.AllowLocalAddresses = true })
	if w := e.do(req{path: "/api/session", host: "127.0.0.1:8580"}); w.Code != 200 {
		t.Fatalf("own address refused: %d", w.Code)
	}
	if w := e.do(req{path: "/api/session", host: "evil.example"}); w.Code != 421 {
		t.Fatalf("foreign host admitted: %d", w.Code)
	}
	if w := e.do(req{path: "/api/session", host: "8.8.8.8"}); w.Code != 421 {
		t.Fatalf("foreign address admitted: %d", w.Code)
	}
}

func TestLoginFromOwnAddressPassesCSRF(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.AllowLocalAddresses = true })
	w := e.do(req{method: "POST", path: "/api/login", host: "127.0.0.1:8580", origin: "http://127.0.0.1:8580",
		body: `{"password":"` + testPassword + `"}`})
	if w.Code != 200 {
		t.Fatalf("login from an own-address origin: %d %s", w.Code, w.Body)
	}
}
