package httpapi

import (
	_ "embed"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/sanitize"
)

// readCSP locks down everything under /read/: book content can never run script,
// load remote resources or be framed by another site. Inline styles are allowed
// because books need them (code listings, Sigil classes); with no script and
// every fetch pinned to 'self', CSS has no way to reach anywhere else.
const readCSP = "default-src 'none'; img-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

const maxChapterBytes = 8 << 20

//go:embed base.css
var baseCSS []byte

func serveBaseCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(baseCSS)
}

func (s *Server) getTOC(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	e, done, err := s.openEPUB(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer done()
	writeJSON(w, http.StatusOK, map[string]any{"chapters": len(e.Spine), "toc": e.TOC(), "rtl": e.RTL})
}

// resourceType is the allowlist for /read/{id}/res: images (not SVG), CSS and
// fonts. It returns the media type to serve, or "". Fonts declared as a generic
// binary are recognised by their file name.
func resourceType(m formats.ManifestItem) string {
	t := strings.ToLower(strings.TrimSpace(m.MediaType))
	switch {
	case t == "application/octet-stream" || t == "application/x-font":
		return fontExts[strings.ToLower(path.Ext(m.Href))]
	case t == "image/svg+xml":
		return ""
	case t == "image/jpeg", t == "image/png", t == "image/gif", t == "image/webp", t == "image/avif":
		return t
	case t == "text/css":
		return "text/css; charset=utf-8"
	}
	return fontTypes[t]
}

// fontTypes maps the font media types EPUBs declare to fixed values; the declared
// string is never echoed, so it cannot smuggle in something like "font/x,text/html".
var fontTypes = map[string]string{
	"font/woff": "font/woff", "font/woff2": "font/woff2", "font/ttf": "font/ttf", "font/otf": "font/otf",
	"font/sfnt": "font/sfnt", "font/collection": "font/collection",
	"application/font-woff": "font/woff", "application/font-sfnt": "font/sfnt",
	"application/vnd.ms-opentype": "font/otf", "application/x-font-ttf": "font/ttf",
	"application/x-font-truetype": "font/ttf", "application/x-font-otf": "font/otf",
	"application/x-font-opentype": "font/otf", "font/opentype": "font/otf", "font/truetype": "font/ttf",
	"application/vnd-ms-opentype": "font/otf", "application/x-font-woff": "font/woff",
}

var fontExts = map[string]string{".ttf": "font/ttf", ".otf": "font/otf", ".woff": "font/woff", ".woff2": "font/woff2"}

func (s *Server) bookURLs(id int64, e *formats.EPUB, dir string) sanitize.Options {
	return sanitize.Options{
		Resource: func(ref string) (string, bool) {
			p, ok := formats.ResolveHref(dir, ref)
			if !ok {
				return "", false
			}
			idx, ok := e.IndexOf(p)
			if !ok || resourceType(e.Manifest[idx]) == "" {
				return "", false
			}
			return fmt.Sprintf("/read/%d/res/%d", id, idx), true
		},
		Link: func(ref string) (string, bool) {
			p, ok := formats.ResolveHref(dir, ref)
			if !ok {
				return "", false
			}
			idx, ok := e.IndexOf(p)
			if !ok {
				return "", false
			}
			ch, ok := e.SpineIndexOf(idx)
			if !ok {
				return "", false
			}
			u := fmt.Sprintf("/read/%d/chapter/%d", id, ch)
			if _, frag, found := strings.Cut(ref, "#"); found && frag != "" {
				u += "#" + frag
			}
			return u, true
		},
	}
}

// getChapter serves spine item n (0-based) as a sanitized standalone document,
// meant for a sandboxed iframe.
func (s *Server) getChapter(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		s.fail(w, r, errNotFound)
		return
	}
	e, done, err := s.openEPUB(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer done()
	if n < 0 || n >= len(e.Spine) {
		s.fail(w, r, errNotFound)
		return
	}
	idx := e.Spine[n]
	src, err := e.ReadEntry(idx, maxChapterBytes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page := sanitize.Chapter(src, s.bookURLs(it.ID, e, path.Dir(e.Manifest[idx].Href)))
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", readCSP)
	h.Set("Cache-Control", "private, max-age=3600")
	w.Write(sanitize.Document(page, "/read/base.css"))
}

// getResource serves manifest item idx if its type is on the allowlist. CSS has
// its url() references rewritten to same-book resources.
func (s *Server) getResource(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		s.fail(w, r, errNotFound)
		return
	}
	e, done, err := s.openEPUB(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer done()
	if idx < 0 || idx >= len(e.Manifest) {
		s.fail(w, r, errNotFound)
		return
	}
	typ := resourceType(e.Manifest[idx])
	if typ == "" {
		s.fail(w, r, errNotFound)
		return
	}
	b, err := e.ReadEntry(idx, s.limits.MaxEntryBytes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.HasPrefix(typ, "text/css") {
		urls := s.bookURLs(it.ID, e, path.Dir(e.Manifest[idx].Href))
		b = sanitize.CSS(b, urls.Resource)
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Content-Security-Policy", readCSP)
	h.Set("Cache-Control", "private, max-age=86400")
	w.Write(b)
}
