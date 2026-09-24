package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/covers"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/scan"
	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

const testPassword = "open sesame"

// env is a running Armarium over a tiny comics + books library.
type env struct {
	t       *testing.T
	srv     *Server
	h       http.Handler
	token   string
	cookie  string
	items   map[string]store.Item // by title
	comicID int64
	bookID  int64
}

var passwordHash = func() string { h, _ := auth.HashPassword(testPassword); return h }()

func newEnv(t *testing.T) *env {
	t.Helper()
	comics, books, cal := t.TempDir(), t.TempDir(), t.TempDir()
	tu.WriteCalibre(t, cal,
		tu.CalibreBook{Title: "Infection", Author: "John Betancourt", Series: "Double Helix", SeriesIndex: 1,
			Cover: tu.PNG(40, 60, 200), Files: map[string][]byte{"EPUB": mustRead(t, tu.WriteEPUB(t, t.TempDir(), "i.epub",
				tu.EPUBOpts{Title: "x", Chapters: []string{"<p>1</p>"}})), "MOBI": []byte("MOBI-BYTES")}},
		tu.CalibreBook{Title: "Mobi Only", Author: "Ann Other", Files: map[string][]byte{"MOBI": []byte("M")}},
		tu.CalibreBook{Title: "No Cover File", Author: "Ann Other", Files: map[string][]byte{"EPUB": mustRead(t, tu.WriteEPUB(t, t.TempDir(), "n.epub",
			tu.EPUBOpts{Title: "n", Chapters: []string{"<p>1</p>"}}))}},
	)
	pages := []tu.Entry{{Name: "01.png", Data: tu.PNG(8, 12, 10)}, {Name: "02.png", Data: tu.PNG(8, 12, 20)}, {Name: "03.jpg", Data: []byte("jpeg")}}
	tu.WriteZip(t, comics, "Saga/Saga #01.cbz", pages...)
	tu.WriteRAR(t, comics, "Saga/Saga #02.cbr", pages...)
	tu.WriteEPUB(t, books, "oreilly_downloads/Book.epub", tu.EPUBOpts{
		Title: "Book",
		Chapters: []string{
			`<h1>One</h1><img src="../images/cover.png"/><a href="ch1.xhtml#x">next</a>`,
			`<svg><style><img src=x onerror=alert(1)></style></svg><p id="x">Two</p>`,
		},
		Extra:    []tu.Entry{{Name: "evil.js", Data: []byte("alert(1)")}, {Name: "evil.svg", Data: []byte("<svg onload=alert(1)/>")}},
		ExtraOPF: `<item id="js" href="evil.js" media-type="application/javascript"/><item id="svg" href="evil.svg" media-type="image/svg+xml"/>`,
	})

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.PasswordHash = passwordHash
	cfg.AllowedHosts = []string{"armarium.test", "localhost"}
	cfg.CORSOrigins = []string{"https://comics.skriv.ist"}
	cfg.Libraries = []config.Library{
		{Name: "Comics", Root: comics, Kind: "comics"},
		{Name: "Books", Root: books, Kind: "books", Include: []string{"*_downloads"}},
		{Name: "Calibre", Root: cal, Kind: "books", Calibre: true},
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "armarium.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	st.SyncLibraries(ctx, []store.Library{
		{Name: "Comics", Root: comics, Kind: "comics"}, {Name: "Books", Root: books, Kind: "books"},
		{Name: "Calibre", Root: cal, Kind: "books"},
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	lim := formats.Limits{MaxEntryBytes: cfg.Limits.MaxEntryBytes, MaxEntries: cfg.Limits.MaxEntries}
	sc := &scan.Scanner{Store: st, Libraries: cfg.Libraries, Limits: lim, Workers: 1, Log: log}
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	cv := &covers.Service{Dir: filepath.Join(cfg.DataDir, "covers"), MaxPixels: 1 << 24, MaxBytes: 1 << 20}
	srv := New(cfg, st, sc, cv, nil, log)
	e := &env{t: t, srv: srv, h: srv.Handler(), items: map[string]store.Item{}}

	e.token = auth.NewSecret()
	st.AddToken(ctx, "test", auth.Digest(e.token))
	items, _, _ := st.Items(ctx, store.ItemFilter{Limit: 100})
	for _, it := range items {
		e.items[it.Title] = it
	}
	e.comicID, e.bookID = e.items["Saga #01"].ID, e.items["Book"].ID
	if e.comicID == 0 || e.bookID == 0 {
		t.Fatalf("fixtures not scanned: %v", e.items)
	}
	return e
}

type req struct {
	method, path, body string
	host, origin       string
	token, basic       string
	cookie             bool
	ctype              string
	headers            map[string]string
}

func (e *env) do(q req) *httptest.ResponseRecorder {
	e.t.Helper()
	if q.method == "" {
		q.method = http.MethodGet
	}
	r := httptest.NewRequest(q.method, q.path, strings.NewReader(q.body))
	r.Host = "armarium.test"
	if q.host != "" {
		r.Host = q.host
	}
	if q.origin != "" {
		r.Header.Set("Origin", q.origin)
	}
	if q.token != "" {
		r.Header.Set("Authorization", "Bearer "+q.token)
	}
	if q.basic != "" {
		r.SetBasicAuth("anyone", q.basic)
	}
	if q.cookie {
		r.Header.Set("Cookie", e.cookie)
	}
	for k, v := range q.headers {
		r.Header.Set(k, v)
	}
	if q.ctype != "" {
		r.Header.Set("Content-Type", q.ctype)
	} else if q.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

// login performs a real password login and keeps the session cookie.
func (e *env) login() {
	e.t.Helper()
	w := e.do(req{method: "POST", path: "/api/login", body: `{"password":"` + testPassword + `"}`, origin: "https://armarium.test"})
	if w.Code != 200 {
		e.t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	e.cookie = strings.Split(w.Header().Get("Set-Cookie"), ";")[0]
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
