package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// uiCSP is the policy for the app shell. Chapters load in a same-origin iframe.
const uiCSP = "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; " +
	"frame-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// ui serves the embedded SPA: real files as-is, every other path gets index.html.
func (s *Server) ui() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/opds/") || strings.HasPrefix(r.URL.Path, "/read/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		w.Header().Set("Content-Security-Policy", uiCSP)
		index, err := fs.ReadFile(s.uiFS(), "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Armarium UI is not built. Run `make web` and rebuild.\n"))
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(s.uiFS(), name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.uiFS(), name)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

func (s *Server) uiFS() fs.FS {
	if s.UI == nil {
		return emptyFS{}
	}
	return s.UI
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
