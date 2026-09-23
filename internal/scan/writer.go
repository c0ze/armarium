package scan

import (
	"context"
	"path"
	"strings"
	"sync"

	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
)

// describe opens a new or changed file just enough to read its page count,
// title and author. A file that fails to open is still listed (it can be
// downloaded). Calibre books keep Calibre's metadata and only take the pages.
func (s *Scanner) describe(lib store.Library, labels Labels, f found) store.ScannedItem {
	if f.meta != nil {
		it := *f.meta
		it.Pages, _, _ = s.inspect(f.abs, f.rel, it.Format)
		return it
	}
	name := path.Base(f.rel)
	format := formats.FormatOf(name)
	dir := path.Dir(f.rel)
	it := store.ScannedItem{
		Path: f.rel, Format: format, Size: f.size, MTime: f.mt,
		Title: titleFromName(name), SeriesPath: dir,
		HasCover: format == "epub" || format == "cbz" || format == "cbr",
	}
	it.SeriesName = path.Base(dir)
	if dir == "." {
		it.SeriesName = lib.Name
	}
	if lib.Kind == "books" && dir != "." {
		top, _, _ := strings.Cut(f.rel, "/")
		it.Source = sourceKey(top)
		if !strings.Contains(dir, "/") {
			it.SeriesName = labels.Label(it.Source)
		}
	}
	if lib.Kind == "comics" {
		it.Number = numberFromName(name)
	}
	var title string
	it.Pages, title, it.Author = s.inspect(f.abs, f.rel, format)
	if title != "" {
		it.Title = title
	}
	it.SortTitle = formats.SortKey(it.Title)
	it.SeriesSort = formats.SortKey(it.SeriesName)
	return it
}

// inspect reads the page count (chapters for EPUB) and, for EPUB, the title and
// author. Download-only formats are not opened.
func (s *Scanner) inspect(abs, rel, format string) (pages int, title, author string) {
	switch format {
	case "cbz", "cbr":
		if c, err := formats.Open(abs, format, s.Limits); err == nil {
			pages = c.Pages()
			c.Close()
		} else {
			s.Log.Warn("scan: cannot open archive", "path", rel, "err", err)
		}
	case "epub":
		if e, err := formats.OpenEPUB(abs, s.Limits); err == nil {
			pages, title, author = len(e.Spine), e.Title, e.Author
			e.Close()
		} else {
			s.Log.Warn("scan: cannot open epub", "path", rel, "err", err)
		}
	case "pdf":
		pages = formats.PDFPageCount(abs)
	}
	return pages, title, author
}

// writer funnels all scanner writes into short transactions of batchSize rows.
// Rows are buffered in memory and the transaction is opened only to flush them,
// so SQLite's write lock is never held while workers open archives (over NFS on
// the NAS) and progress writes wait milliseconds at most.
type writer struct {
	s      *Scanner
	ctx    context.Context
	lib    store.Library
	labels Labels

	mu      sync.Mutex
	pending []func(b *store.Batch) error
	err     error
}

func (w *writer) do(f func(b *store.Batch) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return
	}
	if w.pending = append(w.pending, f); len(w.pending) >= batchSize {
		w.flush()
	}
}

func (w *writer) flush() {
	if len(w.pending) == 0 || w.err != nil {
		return
	}
	ops := w.pending
	w.pending = nil
	b, err := w.s.Store.Begin(w.ctx, w.lib.ID)
	if err != nil {
		w.err = err
		return
	}
	for _, op := range ops {
		if err := op(b); err != nil {
			b.Rollback()
			w.err = err
			return
		}
	}
	w.err = b.Commit()
}

func (w *writer) add(it store.ScannedItem, existed bool) {
	w.do(func(b *store.Batch) error { return b.Upsert(w.ctx, it) })
	w.s.bump(func(st *Status) {
		if existed {
			st.Updated++
		} else {
			st.Added++
		}
	})
}

// refresh rewrites an unchanged item's metadata (Calibre edits) without counting
// it as an update.
func (w *writer) refresh(it store.ScannedItem) {
	w.do(func(b *store.Batch) error { return b.Upsert(w.ctx, it) })
}

func (w *writer) seen(id int64) { w.do(func(b *store.Batch) error { return b.Seen(w.ctx, id) }) }

func (w *writer) missing(id int64) {
	w.do(func(b *store.Batch) error { return b.MarkMissing(w.ctx, id) })
	w.s.bump(func(st *Status) { st.Missing++ })
}

func (w *writer) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flush()
}
