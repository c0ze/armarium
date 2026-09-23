// Package httpapi serves the JSON API, OPDS feeds, the EPUB reader and the UI.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/covers"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/scan"
	"github.com/c0ze/armarium/internal/store"
)

type Server struct {
	Cfg     config.Config
	Store   *store.Store
	Scanner *scan.Scanner
	Covers  *covers.Service
	UI      fs.FS // built web app; may be nil
	Log     *slog.Logger

	limits formats.Limits
	comics *formats.Cache[formats.Container]
	books  *formats.Cache[*formats.EPUB]
	sem    chan struct{} // bounds concurrent archive reads
	logins *loginLimiter
}

func New(cfg config.Config, st *store.Store, sc *scan.Scanner, cv *covers.Service, ui fs.FS, log *slog.Logger) *Server {
	s := &Server{
		Cfg: cfg, Store: st, Scanner: sc, Covers: cv, UI: ui, Log: log,
		limits: formats.Limits{MaxEntryBytes: cfg.Limits.MaxEntryBytes, MaxEntries: cfg.Limits.MaxEntries},
		comics: formats.NewCache[formats.Container](8),
		books:  formats.NewCache[*formats.EPUB](4),
		sem:    make(chan struct{}, cfg.Limits.MaxOpenArchives),
		logins: newLoginLimiter(),
	}
	if sc != nil {
		sc.OnChange = s.Forget
	}
	return s
}

// Forget drops cached open archives for an item whose file changed.
func (s *Server) Forget(itemID int64) {
	s.comics.Forget(itemID)
	s.books.Forget(itemID)
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	route := func(pattern string, level Access, h http.HandlerFunc) { m.Handle(pattern, s.guard(level, h)) }

	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	route("GET /api/session", Public, s.getSession)
	route("POST /api/login", Public, s.login)
	route("POST /api/logout", Public, s.logout)

	route("GET /api/libraries", Reader, s.listLibraries)
	route("GET /api/libraries/{id}/series", Reader, s.listSeries)
	route("GET /api/series/{id}", Reader, s.getSeries)
	route("GET /api/facets", Reader, s.getFacets)
	route("GET /api/items", Reader, s.listItems)
	route("GET /api/items/{id}", Reader, s.getItem)
	route("GET /api/items/{id}/cover", Reader, s.getCover)
	route("GET /api/items/{id}/pages/{n}", Reader, s.getPage)
	route("GET /api/items/{id}/file", Reader, s.getFile)
	route("GET /api/items/{id}/toc", Reader, s.getTOC)
	route("PUT /api/items/{id}/progress", Reader, s.putProgress)

	route("GET /api/scan", Admin, s.getScan)
	route("POST /api/scan", Admin, s.postScan)
	route("GET /api/tokens", Admin, s.listTokens)
	route("POST /api/tokens", Admin, s.createToken)
	route("DELETE /api/tokens/{id}", Admin, s.deleteToken)

	m.HandleFunc("GET /read/base.css", serveBaseCSS)
	route("GET /read/{id}/chapter/{n}", Reader, s.getChapter)
	route("GET /read/{id}/res/{idx}", Reader, s.getResource)

	for _, prefix := range []string{"/opds/v1.2", "/opds/t/{token}/v1.2"} {
		s.opdsRoutes(prefix, route)
	}

	m.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	}))
	m.Handle("/", s.ui())
	return s.base(s.hosts(s.cors(m)))
}

// Serve runs the HTTP server until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}
