// Package importer copies read progress (and tags) from the apps Armarium replaces:
// Kavita and the Skrivist Books reader. Both match items by library-relative path.
package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/c0ze/armarium/internal/store"
)

type Result struct {
	Progress int // progress rows written
	Tags     int // items whose tags were set
	Skipped  int // source rows with no matching Armarium item
}

func openRO(p string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+p+"?mode=ro&_pragma=busy_timeout(5000)")
}

// Skrivist imports reader_index.db. Its paths ("oreilly_downloads/X.epub") are
// relative to the folder holding the *_downloads folders, which should be the
// Armarium library root.
func Skrivist(ctx context.Context, st *store.Store, libraryID int64, dbPath string) (Result, error) {
	var res Result
	src, err := openRO(dbPath)
	if err != nil {
		return res, err
	}
	defer src.Close()
	known, err := st.KnownItems(ctx, libraryID)
	if err != nil {
		return res, err
	}
	rows, err := src.QueryContext(ctx, `SELECT f.path, f.format, p.chapter, p.scroll, p.updated, p.completed
		FROM progress p JOIN files f ON f.book_id = p.book_id`)
	if err != nil {
		return res, fmt.Errorf("not a Skrivist reader database: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p, format string
		var chapter int
		var scroll, updated float64
		var completed bool
		if err := rows.Scan(&p, &format, &chapter, &scroll, &updated, &completed); err != nil {
			return res, err
		}
		k, ok := known[p]
		if !ok {
			res.Skipped++
			continue
		}
		pr := store.Progress{Status: "reading", UpdatedAt: int64(updated)}
		if completed {
			pr.Status = "read"
		}
		if format == "epub" { // chapter is an EPUB spine index; meaningless for the PDF twin
			pr.Page = chapter + 1
			loc, _ := json.Marshal(map[string]any{"chapter": chapter, "scroll": scroll})
			pr.Locator = string(loc)
		}
		if ok, err := st.ImportProgress(ctx, k.ID, pr); err != nil {
			return res, err
		} else if ok {
			res.Progress++
		}
	}
	if err := rows.Err(); err != nil {
		return res, err
	}
	return res, skrivistTopics(ctx, st, src, libraryID, known, &res)
}

func skrivistTopics(ctx context.Context, st *store.Store, src *sql.DB, libraryID int64, known map[string]store.Known, res *Result) error {
	rows, err := src.QueryContext(ctx, `SELECT f.path, t.topic FROM book_topics t JOIN files f ON f.book_id = t.book_id ORDER BY f.path, t.topic`)
	if err != nil {
		return nil // older databases have no topics table
	}
	defer rows.Close()
	tags := map[string][]string{}
	for rows.Next() {
		var p, topic string
		if err := rows.Scan(&p, &topic); err != nil {
			return err
		}
		tags[p] = append(tags[p], topic)
	}
	b, err := st.Begin(ctx, libraryID)
	if err != nil {
		return err
	}
	defer b.Rollback()
	for p, ts := range tags {
		if k, ok := known[p]; ok {
			if err := b.SetTags(ctx, k.ID, ts); err != nil {
				return err
			}
			res.Tags++
		}
	}
	return b.Commit()
}

// Kavita imports AppUserProgresses from kavita.db. root is the path prefix Kavita
// saw for this library (e.g. "/comics"); it is stripped to get the relative path.
func Kavita(ctx context.Context, st *store.Store, libraryID int64, dbPath, root string) (Result, error) {
	var res Result
	src, err := openRO(dbPath)
	if err != nil {
		return res, err
	}
	defer src.Close()
	known, err := st.KnownItems(ctx, libraryID)
	if err != nil {
		return res, err
	}
	modified := "LastModifiedUtc"
	var n int
	if src.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('AppUserProgresses') WHERE name = 'LastModifiedUtc'`).Scan(&n); n == 0 {
		modified = "LastModified"
	}
	rows, err := src.QueryContext(ctx, `SELECT mf.FilePath, MAX(p.PagesRead), MAX(p.`+modified+`), MAX(c.Pages)
		FROM AppUserProgresses p JOIN MangaFile mf ON mf.ChapterId = p.ChapterId JOIN Chapter c ON c.Id = p.ChapterId
		GROUP BY mf.FilePath`)
	if err != nil {
		return res, fmt.Errorf("not a Kavita database: %w", err)
	}
	defer rows.Close()
	prefix := strings.TrimSuffix(strings.ReplaceAll(root, `\`, "/"), "/") + "/"
	for rows.Next() {
		var fp, stamp string
		var read, pages int
		if err := rows.Scan(&fp, &read, &stamp, &pages); err != nil {
			return res, err
		}
		rel := path.Clean(strings.TrimPrefix(strings.ReplaceAll(fp, `\`, "/"), prefix))
		k, ok := known[rel]
		if !ok || read <= 0 {
			res.Skipped++
			continue
		}
		pr := store.Progress{Page: read, Status: "reading", UpdatedAt: kavitaTime(stamp)}
		if pages > 0 && read >= pages {
			pr.Page, pr.Status = pages, "read"
		}
		if ok, err := st.ImportProgress(ctx, k.ID, pr); err != nil {
			return res, err
		} else if ok {
			res.Progress++
		}
	}
	return res, rows.Err()
}

// kavitaTime parses EF Core's SQLite datetime text; unknown shapes count as now.
func kavitaTime(s string) int64 {
	for _, layout := range []string{"2006-01-02 15:04:05.9999999", "2006-01-02T15:04:05.9999999Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}
