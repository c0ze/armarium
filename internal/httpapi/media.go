package httpapi

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/c0ze/armarium/internal/covers"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
)

// filePath resolves an item to its file and re-checks that the real path (after
// symlinks) is still under the library root. Paths come only from the database.
func (s *Server) filePath(ctx context.Context, it store.Item) (string, error) {
	lib, err := s.Store.Library(ctx, it.LibraryID)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(lib.Root)
	if err != nil {
		return "", errNotFound
	}
	p, err := filepath.EvalSymlinks(filepath.Join(lib.Root, filepath.FromSlash(it.Path)))
	if err != nil {
		return "", errNotFound
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", errNotFound
	}
	return p, nil
}

func (s *Server) openComic(ctx context.Context, it store.Item) (formats.Container, func(), error) {
	if it.Format != "cbz" && it.Format != "cbr" {
		return nil, nil, errNotFound
	}
	p, err := s.filePath(ctx, it)
	if err != nil {
		return nil, nil, err
	}
	return s.comics.Get(it.ID, func() (formats.Container, error) { return formats.Open(p, it.Format, s.limits) })
}

func (s *Server) openEPUB(ctx context.Context, it store.Item) (*formats.EPUB, func(), error) {
	if it.Format != "epub" {
		return nil, nil, errNotFound
	}
	p, err := s.filePath(ctx, it)
	if err != nil {
		return nil, nil, err
	}
	return s.books.Get(it.ID, func() (*formats.EPUB, error) { return formats.OpenEPUB(p, s.limits) })
}

// getPage serves comic page n (1-based) straight from the archive entry.
func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		s.fail(w, r, errNotFound)
		return
	}
	s.servePage(w, r, it, n-1)
}

// servePage streams page idx (0-based); shared with the OPDS-PSE route.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, it store.Item, idx int) {
	etag := fmt.Sprintf(`"%d-%d-%d-%d"`, it.ID, it.MTime, it.Size, idx)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	release, err := s.acquire(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer release()
	c, done, err := s.openComic(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer done()
	rc, typ, err := c.Page(idx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer rc.Close()
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("ETag", etag)
	h.Set("X-Armarium-Pages", strconv.Itoa(c.Pages()))
	io.Copy(w, rc)
}

// getCover serves a lazily generated thumbnail: the first page of a comic or the
// declared cover of an EPUB.
func (s *Server) getCover(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	s.serveCover(w, r, it)
}

func (s *Server) serveCover(w http.ResponseWriter, r *http.Request, it store.Item) {
	var src func() (io.ReadCloser, error)
	ctx := r.Context()
	if !it.HasCover {
		s.fail(w, r, errNotFound)
		return
	}
	switch {
	case s.calibreLibrary(ctx, it.LibraryID):
		// Calibre keeps the cover it chose (or the user set) next to the files.
		src = func() (io.ReadCloser, error) {
			p, err := s.filePath(ctx, it)
			if err != nil {
				return nil, err
			}
			f, err := os.Open(filepath.Join(filepath.Dir(p), "cover.jpg"))
			if err != nil {
				return nil, covers.ErrNoCover
			}
			return f, nil
		}
	case it.Format == "cbz" || it.Format == "cbr":
		src = func() (io.ReadCloser, error) {
			c, done, err := s.openComic(ctx, it)
			if err != nil {
				return nil, err
			}
			rc, _, err := c.Page(0)
			return releasing(rc, done, err)
		}
	case it.Format == "epub":
		src = func() (io.ReadCloser, error) {
			e, done, err := s.openEPUB(ctx, it)
			if err != nil {
				return nil, err
			}
			if e.Cover < 0 {
				done()
				return nil, covers.ErrNoCover
			}
			rc, err := e.Entry(e.Cover)
			return releasing(rc, done, err)
		}
	default:
		s.fail(w, r, errNotFound)
		return
	}
	release, err := s.acquire(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.Covers.Path(it.ID, it.MTime, src)
	release()
	if err == covers.ErrNoCover {
		err = errNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=604800")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, p)
}

// releasing ties a cache release to the reader's Close, so the archive stays
// open (not evicted and closed) while the entry is still being read.
func releasing(rc io.ReadCloser, done func(), err error) (io.ReadCloser, error) {
	if err != nil {
		done()
		return nil, err
	}
	return &releaseCloser{rc, done}, nil
}

type releaseCloser struct {
	io.ReadCloser
	done func()
}

func (r *releaseCloser) Close() error {
	err := r.ReadCloser.Close()
	r.done()
	return err
}

// getFile streams the original file with Range support. PDFs may be shown inline
// (?inline=1) for the browser's viewer; everything else is an attachment.
// ?format= picks another format of the same Calibre book.
func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	if f := r.URL.Query().Get("format"); f != "" && f != it.Format {
		p, err := s.Store.ItemFile(r.Context(), it.ID, f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		it.Path, it.Format = p, f
	}
	s.serveFile(w, r, it)
}

// calibreLibrary reports whether a library is configured as a Calibre library.
func (s *Server) calibreLibrary(ctx context.Context, libraryID int64) bool {
	lib, err := s.Store.Library(ctx, libraryID)
	if err != nil {
		return false
	}
	for _, l := range s.Cfg.Libraries {
		if l.Name == lib.Name {
			return l.Calibre
		}
	}
	return false
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, it store.Item) {
	p, err := s.filePath(r.Context(), it)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		s.fail(w, r, errNotFound)
		return
	}
	defer f.Close()
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	disp := "attachment"
	if it.Format == "pdf" && r.URL.Query().Get("inline") == "1" {
		disp = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", formats.MediaTypeOf(it.Format))
	h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": path.Base(it.Path)}))
	h.Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", time.Unix(it.MTime, 0), f)
}
