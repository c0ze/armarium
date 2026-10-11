package httpapi

import (
	"net/http"
	"reflect"
	"testing"
)

func TestPairNeedsTheOwnerSession(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/api/pair"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := e.do(req{path: "/api/pair", token: e.token}); w.Code != http.StatusForbidden {
		t.Fatalf("token: %d", w.Code)
	}
	e.login()
	if w := e.do(req{path: "/api/pair", cookie: true}); w.Code != 200 {
		t.Fatalf("session: %d", w.Code)
	}
}

func TestPairPayload(t *testing.T) {
	e := newEnv(t)
	e.login()
	got := decode[map[string]any](t, e.do(req{path: "/api/pair", cookie: true}))
	want := map[string]any{
		"publicUrl": "",
		"readers": []any{
			map[string]any{"id": "comics", "name": "Skrivist Comics", "url": "https://comics.skriv.ist",
				"origin": "https://comics.skriv.ist", "corsAllowed": true, "desktop": false, "missing": []any{}},
			map[string]any{"id": "books", "name": "Skrivist Books", "url": "https://books.skriv.ist",
				"origin": "https://books.skriv.ist", "corsAllowed": false, "desktop": false, "missing": []any{"https://books.skriv.ist"}},
			map[string]any{"id": "comics-desktop", "name": "Skrivist Comics for Mac and Windows", "url": "skrivist-comics://pair",
				"origin": "", "corsAllowed": false, "desktop": true, "missing": []any{"tauri://localhost", "http://tauri.localhost"}},
			map[string]any{"id": "books-desktop", "name": "Skrivist Books for Mac and Windows", "url": "skrivist-books://pair",
				"origin": "", "corsAllowed": false, "desktop": true, "missing": []any{"tauri://localhost", "http://tauri.localhost"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	// Either Windows origin counts: https://tauri.localhost is the useHttpsScheme variant.
	e.srv.Cfg.CORSOrigins = append(e.srv.Cfg.CORSOrigins, "tauri://localhost", "https://tauri.localhost")
	for _, r := range decode[pairInfo](t, e.do(req{path: "/api/pair", cookie: true})).Readers {
		if r.Desktop && (!r.CORSAllowed || len(r.Missing) != 0) {
			t.Errorf("%s with both desktop origins: %+v", r.ID, r)
		}
	}
}

func TestPairEchoesPublicURLAndMatchesOriginsExactly(t *testing.T) {
	e := newEnv(t)
	e.login()
	e.srv.Cfg.PublicURL = "https://armarium.test"
	e.srv.Cfg.CORSOrigins = []string{"https://books.skriv.ist/"}
	got := decode[pairInfo](t, e.do(req{path: "/api/pair", cookie: true}))
	if got.PublicURL != "https://armarium.test" {
		t.Errorf("publicUrl: %q", got.PublicURL)
	}
	for _, r := range got.Readers {
		if r.CORSAllowed {
			t.Errorf("%s: a trailing-slash cors_origins entry never matches a browser Origin", r.ID)
		}
	}
}
