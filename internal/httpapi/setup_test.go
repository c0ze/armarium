package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c0ze/armarium/internal/config"
)

const setupCode = "test-setup-code-0123456789abcdef"

func setupEnv(t *testing.T, managed bool) (*env, *fakeHost) {
	t.Helper()
	e := newEnvWith(t, func(c *config.Config) {
		c.PasswordHash = ""
		if managed {
			c.Desktop = &config.Desktop{Network: "local", Port: 8580}
			c.Libraries = nil
		}
	})
	var h *fakeHost
	if managed {
		h = &fakeHost{folder: "/Users/owner/Comics", ts: TailscaleStatus{Available: true, Name: "studio.tail1234.ts.net"}}
		e.srv.Host = h
	}
	e.srv.SetupCode = setupCode
	e.h = e.srv.Handler()
	return e, h
}

func setupReq(method, path, body, code string) req {
	q := req{method: method, path: path, body: body, origin: "https://armarium.test"}
	if code != "" {
		q.headers = map[string]string{"X-Armarium-Setup": code}
	}
	return q
}

func TestSetupGating(t *testing.T) {
	e, _ := setupEnv(t, false)
	if w := e.do(setupReq("GET", "/api/setup", "", "")); w.Code != 401 {
		t.Fatalf("no code: %d", w.Code)
	}
	if w := e.do(setupReq("GET", "/api/setup", "", setupCode)); w.Code != 200 {
		t.Fatalf("right code: %d %s", w.Code, w.Body)
	}
	q := setupReq("GET", "/api/setup", "", setupCode)
	q.host = "evil.example"
	if w := e.do(q); w.Code != 421 {
		t.Fatalf("foreign host: %d", w.Code)
	}
	q = setupReq("POST", "/api/setup/finish", `{"password":"long enough"}`, setupCode)
	q.origin = "https://evil.example"
	if w := e.do(q); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	q = setupReq("POST", "/api/setup/finish", `{"password":"long enough"}`, setupCode)
	q.ctype = "text/plain"
	if w := e.do(q); w.Code != 415 {
		t.Fatalf("text/plain: %d", w.Code)
	}
	// The request without a code was the first failed attempt; four more are
	// allowed in the minute, the sixth is refused before the code is checked.
	for i := 2; i <= 6; i++ {
		w := e.do(setupReq("GET", "/api/setup", "", "wrong"))
		want := 401
		if i == 6 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("wrong code try %d: %d, want %d", i, w.Code, want)
		}
	}
}

func TestSetupClosedOncePasswordExists(t *testing.T) {
	e := newEnv(t) // password_hash configured
	e.srv.SetupCode = setupCode
	e.h = e.srv.Handler()
	w := e.do(setupReq("GET", "/api/setup", "", setupCode))
	if w.Code != 410 || !strings.Contains(w.Body.String(), "Armarium is already set up") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestSetupFinishSetsPasswordAndLogsIn(t *testing.T) {
	e, _ := setupEnv(t, false)
	w := e.do(setupReq("POST", "/api/setup/finish", `{"password":"my new pass"}`, setupCode))
	if w.Code != 200 {
		t.Fatalf("finish: %d %s", w.Code, w.Body)
	}
	e.cookie = strings.Split(w.Header().Get("Set-Cookie"), ";")[0]
	if s := decode[map[string]any](t, e.do(req{path: "/api/session", cookie: true})); s["admin"] != true || s["setupOpen"] != false {
		t.Fatalf("session after setup: %v", s)
	}
	if w := e.do(setupReq("POST", "/api/setup/finish", `{"password":"another pass"}`, setupCode)); w.Code != 410 {
		t.Fatalf("second finish: %d", w.Code)
	}
	if c := loginWith(e, "my new pass"); c != 200 {
		t.Fatalf("login: %d", c)
	}
}

func TestSetupFinishRejectsShortPassword(t *testing.T) {
	e, _ := setupEnv(t, false)
	if w := e.do(setupReq("POST", "/api/setup/finish", `{"password":"short"}`, setupCode)); w.Code != 400 {
		t.Fatalf("short: %d", w.Code)
	}
	if w := e.do(setupReq("GET", "/api/setup", "", setupCode)); w.Code != 200 {
		t.Fatalf("code no longer valid after a refused finish: %d", w.Code)
	}
}

func TestSetupDescribesManagedServer(t *testing.T) {
	e, _ := setupEnv(t, true)
	got := decode[map[string]any](t, e.do(setupReq("GET", "/api/setup", "", setupCode)))
	if got["desktop"] != true || got["managed"] != true || got["settings"] == nil {
		t.Fatalf("%v", got)
	}
	if ts := got["tailscale"].(map[string]any); ts["name"] != "studio.tail1234.ts.net" {
		t.Fatalf("tailscale %v", ts)
	}
}

func TestSetupFinishAppliesSettingsThroughHost(t *testing.T) {
	e, h := setupEnv(t, true)
	comics := t.TempDir()
	yes := true
	body, _ := json.Marshal(map[string]any{
		"password": "my new pass", "startAtLogin": yes,
		"settings": config.Settings{Libraries: []config.Library{{Name: "Comics", Root: comics, Kind: "comics"}}, Network: "home", Port: 8580},
	})
	w := e.do(setupReq("POST", "/api/setup/finish", string(body), setupCode))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	ch, ok := h.last()
	if !ok || ch.Settings.Network != "home" || len(ch.Settings.Libraries) != 1 || ch.StartAtLogin == nil || !*ch.StartAtLogin {
		t.Fatalf("host got %+v", ch)
	}
}

func TestSetupFinishKeepsNoPasswordWhenHostFails(t *testing.T) {
	e, h := setupEnv(t, true)
	h.err = errString("port 8580 is in use by another program")
	body := `{"password":"my new pass","settings":{"libraries":[],"network":"local","port":8580}}`
	w := e.do(setupReq("POST", "/api/setup/finish", body, setupCode))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "port 8580 is in use") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if s := decode[map[string]any](t, e.do(req{path: "/api/session"})); s["setupOpen"] != true {
		t.Fatalf("a password was set although applying failed: %v", s)
	}
}

func TestSetupFolderRoutesNeedCode(t *testing.T) {
	e, _ := setupEnv(t, true)
	q := setupReq("POST", "/api/setup/folder", "{}", setupCode)
	q.host, q.remote, q.origin = "localhost:8580", "127.0.0.1:5000", "http://localhost:8580"
	if w := e.do(q); w.Code != 200 || !strings.Contains(w.Body.String(), "/Users/owner/Comics") {
		t.Fatalf("folder with code: %d %s", w.Code, w.Body)
	}
	q.headers = nil
	if w := e.do(q); w.Code != 401 {
		t.Fatalf("folder without code: %d", w.Code)
	}
	w := e.do(setupReq("POST", "/api/setup/check-folder", `{"path":"relative"}`, setupCode))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Use the full path") {
		t.Fatalf("check-folder: %d %s", w.Code, w.Body)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
