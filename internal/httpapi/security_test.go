package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestEverythingButHealthzNeedsAuth(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{
		"/api/libraries", "/api/items", fmt.Sprintf("/api/items/%d", e.comicID),
		fmt.Sprintf("/api/items/%d/pages/1", e.comicID), fmt.Sprintf("/api/items/%d/file", e.comicID),
		fmt.Sprintf("/read/%d/chapter/0", e.bookID), "/opds/v1.2/catalog", "/api/scan", "/api/tokens",
	} {
		if w := e.do(req{path: p}); w.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d without auth", p, w.Code)
		}
	}
	if w := e.do(req{path: "/opds/v1.2/catalog"}); !strings.Contains(w.Header().Get("WWW-Authenticate"), "Basic") {
		t.Error("OPDS 401 must offer Basic")
	}
	if w := e.do(req{path: "/healthz", host: "anything.example"}); w.Code != 200 {
		t.Errorf("healthz: %d", w.Code)
	}
	if w := e.do(req{path: "/api/libraries", token: "wrong"}); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token: %d", w.Code)
	}
}

func TestTokenFormsAreAccepted(t *testing.T) {
	e := newEnv(t)
	for name, q := range map[string]req{
		"bearer": {path: "/api/libraries", token: e.token},
		"basic":  {path: "/opds/v1.2/catalog", basic: e.token},
		"query":  {path: "/api/libraries?token=" + e.token},
		"path":   {path: "/opds/t/" + e.token + "/v1.2/catalog"},
	} {
		if w := e.do(q); w.Code != 200 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}

func TestAdminRoutesNeedTheOwnerSession(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/api/tokens", token: e.token}); w.Code != http.StatusForbidden {
		t.Fatalf("token reached admin route: %d", w.Code)
	}
	e.login()
	if w := e.do(req{path: "/api/tokens", cookie: true}); w.Code != 200 {
		t.Fatalf("session: %d", w.Code)
	}
	w := e.do(req{method: "POST", path: "/api/tokens", body: `{"name":"panelflow"}`, cookie: true, origin: "https://armarium.test"})
	secret := decode[map[string]any](t, w)["secret"].(string)
	if w := e.do(req{path: "/api/libraries", token: secret}); w.Code != 200 {
		t.Fatalf("new token rejected: %d", w.Code)
	}
}

// DNS rebinding: a request for our content under a foreign Host is refused.
func TestHostAllowlist(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/api/libraries", token: e.token, host: "evil.example"}); w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("foreign host: %d", w.Code)
	}
	if w := e.do(req{path: "/api/libraries", token: e.token, host: "localhost:8580"}); w.Code != 200 {
		t.Fatalf("allowed host with port: %d", w.Code)
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	e.login()
	path := fmt.Sprintf("/api/items/%d/progress", e.comicID)
	body := `{"page":2}`
	cases := []struct {
		name string
		q    req
		code int
	}{
		{"cookie, no origin", req{method: "PUT", path: path, body: body, cookie: true}, 403},
		{"cookie, foreign origin", req{method: "PUT", path: path, body: body, cookie: true, origin: "https://evil.example"}, 403},
		{"cookie, form post", req{method: "PUT", path: path, body: body, cookie: true, origin: "https://armarium.test", ctype: "text/plain"}, 415},
		{"cookie, own origin", req{method: "PUT", path: path, body: body, cookie: true, origin: "https://armarium.test"}, 200},
		{"basic, foreign origin", req{method: "PUT", path: path, body: body, basic: e.token, origin: "https://evil.example"}, 403},
		{"bearer, cors origin", req{method: "PUT", path: path, body: body, token: e.token, origin: "https://comics.skriv.ist"}, 200},
		{"bearer, no origin (server client)", req{method: "PUT", path: path, body: body, token: e.token}, 200},
		{"bearer, foreign origin", req{method: "PUT", path: path, body: body, token: e.token, origin: "https://evil.example"}, 403},
		{"logout cross-site", req{method: "POST", path: "/api/logout", body: `{}`, cookie: true, origin: "https://evil.example"}, 403},
	}
	for _, c := range cases {
		if w := e.do(c.q); w.Code != c.code {
			t.Errorf("%s: %d, want %d (%s)", c.name, w.Code, c.code, w.Body)
		}
	}
}

func TestCORSPreflightAndHeaders(t *testing.T) {
	e := newEnv(t)
	preflight := func(origin string) *httptest.ResponseRecorder {
		return e.do(req{method: "OPTIONS", path: "/api/items/1/progress", origin: origin, headers: map[string]string{
			"Access-Control-Request-Method": "PUT", "Access-Control-Request-Headers": "authorization, content-type"}})
	}
	w := preflight("https://comics.skriv.ist")
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "https://comics.skriv.ist" ||
		!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	if w := preflight("https://evil.example"); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin got CORS headers")
	}
	got := e.do(req{path: "/api/libraries", token: e.token, origin: "https://comics.skriv.ist"})
	if got.Header().Get("Access-Control-Allow-Origin") != "https://comics.skriv.ist" || got.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("simple request CORS headers: %v", got.Header())
	}
}

// Garbage and overflowing IDs are a 404, never a 500 (audit: int overflow).
func TestBadIDsAre404(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{
		"/api/items/99999999999999999999999", "/api/items/-1", "/api/items/abc", "/api/items/1e3",
		"/api/items/99999999999999999999999/pages/1", fmt.Sprintf("/api/items/%d/pages/0", e.comicID),
		fmt.Sprintf("/api/items/%d/pages/99999999999999999999", e.comicID), fmt.Sprintf("/api/items/%d/pages/4", e.comicID),
		fmt.Sprintf("/read/%d/chapter/99", e.bookID), fmt.Sprintf("/read/%d/res/-1", e.bookID),
		fmt.Sprintf("/api/items/%d/toc", e.comicID), "/api/series/0", "/api/libraries/77/series",
	} {
		if w := e.do(req{path: p, token: e.token}); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", p, w.Code, w.Body)
		}
	}
}

// XSS through book content (audit: svg/style payload, raw /res serving).
func TestBookContentIsSanitizedAndLockedDown(t *testing.T) {
	e := newEnv(t)
	w := e.do(req{path: fmt.Sprintf("/read/%d/chapter/1", e.bookID), token: e.token})
	body := strings.ToLower(w.Body.String())
	if w.Code != 200 || strings.Contains(body, "onerror") || strings.Contains(body, "<svg") || !strings.Contains(body, `<p id="x">two</p>`) {
		t.Fatalf("chapter: %d %s", w.Code, body)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Fatalf("csp: %q", csp)
	}
	w = e.do(req{path: fmt.Sprintf("/read/%d/chapter/0", e.bookID), token: e.token})
	if !strings.Contains(w.Body.String(), fmt.Sprintf(`/read/%d/chapter/1#x`, e.bookID)) ||
		!strings.Contains(w.Body.String(), fmt.Sprintf(`<img src="/read/%d/res/`, e.bookID)) {
		t.Fatalf("links not rewritten: %s", w.Body)
	}
	// Manifest: 0 nav, 1 cover, 2 css, 3-4 chapters, 5 js, 6 svg.
	for idx, want := range map[int]int{1: 200, 2: 200, 0: 404, 3: 404, 5: 404, 6: 404} {
		w := e.do(req{path: fmt.Sprintf("/read/%d/res/%d", e.bookID, idx), token: e.token})
		if w.Code != want {
			t.Errorf("res %d: %d want %d (%s)", idx, w.Code, want, w.Header().Get("Content-Type"))
		}
	}
	css := e.do(req{path: fmt.Sprintf("/read/%d/res/2", e.bookID), token: e.token}).Body.String()
	if strings.Contains(css, "evil.example") || !strings.Contains(css, fmt.Sprintf("/read/%d/res/1", e.bookID)) {
		t.Fatalf("css not rewritten: %s", css)
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	e := newEnv(t)
	codes := []int{}
	for range 6 {
		w := e.do(req{method: "POST", path: "/api/login", body: `{"password":"nope"}`, origin: "https://armarium.test"})
		codes = append(codes, w.Code)
	}
	if codes[0] != 401 || codes[5] != 429 {
		t.Fatalf("codes %v", codes)
	}
}

// Parallel logins must not all pass the limiter (each check is a 64 MiB argon2id).
func TestParallelLoginsAreCountedBeforeChecking(t *testing.T) {
	e := newEnv(t)
	codes := make(chan int, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.do(req{method: "POST", path: "/api/login", body: `{"password":"nope"}`, origin: "https://armarium.test"}).Code
		}()
	}
	wg.Wait()
	close(codes)
	checked := 0
	for c := range codes {
		if c == http.StatusUnauthorized {
			checked++
		}
	}
	if checked != 5 {
		t.Fatalf("%d password checks ran, want 5", checked)
	}
}

func TestResourceTypeNeverEchoesDeclaredType(t *testing.T) {
	for declared, want := range map[string]string{
		"font/woff2": "font/woff2", "application/x-font-ttf": "font/ttf", "font/x,text/html": "",
		"image/svg+xml": "", "text/html": "", "image/png": "image/png",
	} {
		if got := resourceType(declared); got != want {
			t.Errorf("%q → %q, want %q", declared, got, want)
		}
	}
}
