// Package calibre reads a Calibre library's metadata.db (read-only) so Armarium can
// serve it the way Calibre-Web does: titles, authors, series, tags and covers
// come from Calibre instead of file names.
package calibre

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type File struct {
	Format string // lowercase, e.g. "epub", "mobi"
	Path   string // relative to the library root
}

type Book struct {
	ID          int64
	Title, Sort string
	Dir         string // the book's folder, relative to the library root
	Authors     []string
	Series      string
	SeriesIndex float64
	Tags        []string
	HasCover    bool // Dir/cover.jpg
	Files       []File
}

// Read loads every book. The database is opened read-only (Calibre or
// Calibre-Web may have it open) and closed before returning.
func Read(ctx context.Context, root string) ([]Book, error) {
	db, err := sql.Open("sqlite", "file:"+path.Join(root, "metadata.db")+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	byID := map[int64]*Book{}
	var order []int64
	err = each(ctx, db, "SELECT id, title, sort, path, has_cover, COALESCE(series_index, 1) FROM books", func(r *sql.Rows) error {
		var b Book
		var sortTitle sql.NullString
		if err := r.Scan(&b.ID, &b.Title, &sortTitle, &b.Dir, &b.HasCover, &b.SeriesIndex); err != nil {
			return err
		}
		if b.Sort = sortTitle.String; b.Sort == "" {
			b.Sort = b.Title
		}
		byID[b.ID] = &b
		order = append(order, b.ID)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("not a Calibre library (%s): %w", root, err)
	}
	link := func(q string, set func(*Book, string)) error {
		return each(ctx, db, q, func(r *sql.Rows) error {
			var id int64
			var v string
			if err := r.Scan(&id, &v); err != nil {
				return err
			}
			if b := byID[id]; b != nil {
				set(b, v)
			}
			return nil
		})
	}
	steps := []error{
		link("SELECT l.book, a.name FROM books_authors_link l JOIN authors a ON a.id = l.author ORDER BY l.id",
			func(b *Book, v string) { b.Authors = append(b.Authors, v) }),
		link("SELECT l.book, s.name FROM books_series_link l JOIN series s ON s.id = l.series",
			func(b *Book, v string) { b.Series = v }),
		link("SELECT l.book, t.name FROM books_tags_link l JOIN tags t ON t.id = l.tag ORDER BY t.name",
			func(b *Book, v string) { b.Tags = append(b.Tags, v) }),
		link("SELECT book, lower(format) || char(0) || name FROM data",
			func(b *Book, v string) {
				format, name, _ := strings.Cut(v, "\x00")
				b.Files = append(b.Files, File{Format: format, Path: path.Join(b.Dir, name+"."+format)})
			}),
	}
	for _, err := range steps {
		if err != nil {
			return nil, err
		}
	}
	out := make([]Book, 0, len(order))
	for _, id := range order {
		b := byID[id]
		if !safeRel(b.Dir) {
			continue // a crafted path must never leave the library root
		}
		sort.SliceStable(b.Files, func(i, j int) bool { return rank(b.Files[i].Format) < rank(b.Files[j].Format) })
		out = append(out, *b)
	}
	return out, nil
}

// preference orders a book's formats: the first one present becomes the item
// (readable in the browser if possible), the others are extra downloads.
var preference = []string{"epub", "cbz", "cbr", "pdf", "azw3", "mobi", "azw", "fb2", "djvu"}

func rank(format string) int {
	for i, f := range preference {
		if f == format {
			return i
		}
	}
	return len(preference)
}

func safeRel(p string) bool {
	c := path.Clean(p)
	return p != "" && c != "." && c != ".." && !strings.HasPrefix(c, "../") && !path.IsAbs(c)
}

func each(ctx context.Context, db *sql.DB, q string, fn func(*sql.Rows) error) error {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
