package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "armarium.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seed creates one library with one series holding the given item paths.
func seed(t *testing.T, s *Store, paths ...string) (Library, []int64) {
	t.Helper()
	ctx := context.Background()
	if err := s.SyncLibraries(ctx, []Library{{Name: "Comics", Root: "/c", Kind: "comics"}}); err != nil {
		t.Fatal(err)
	}
	libs, _ := s.Libraries(ctx)
	b, _ := s.Begin(ctx, libs[0].ID)
	for _, p := range paths {
		if err := b.Upsert(ctx, ScannedItem{SeriesPath: "S", SeriesName: "S", SeriesSort: "s", Path: p,
			Format: "cbz", Title: p, SortTitle: p, Pages: 10, Size: 1, MTime: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	known, _ := s.KnownItems(ctx, libs[0].ID)
	var ids []int64
	for _, p := range paths {
		ids = append(ids, known[p].ID)
	}
	return libs[0], ids
}

func TestMigrateIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s.Close()
}

func TestProgressIsMonotonicUnlessReset(t *testing.T) {
	s := openTest(t)
	_, ids := seed(t, s, "a.cbz")
	ctx := context.Background()
	id := ids[0]

	p, ok, _ := s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 5})
	if !ok || p.Page != 5 || p.Status != "reading" {
		t.Fatalf("first write: %+v %v", p, ok)
	}
	p, ok, _ = s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 3})
	if ok || p.Page != 5 {
		t.Fatalf("backwards write must be ignored: %+v %v", p, ok)
	}
	p, _, _ = s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 10})
	if p.Status != "read" {
		t.Fatalf("last page must mark read: %+v", p)
	}
	p, _, _ = s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 10, Status: "reading"})
	if p.Status != "read" {
		t.Fatalf("read must not be downgraded without reset: %+v", p)
	}
	p, ok, _ = s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 0, Status: "unread", Reset: true})
	if !ok || p.Page != 0 || p.Status != "unread" {
		t.Fatalf("reset: %+v %v", p, ok)
	}
}

func TestMissingItemsKeepProgressThenPrune(t *testing.T) {
	s := openTest(t)
	lib, ids := seed(t, s, "a.cbz", "b.cbz")
	ctx := context.Background()
	s.SaveProgress(ctx, ids[0], 10, ProgressUpdate{Page: 4})

	b, _ := s.Begin(ctx, lib.ID)
	b.MarkMissing(ctx, ids[0])
	b.Commit()
	if _, err := s.Item(ctx, ids[0]); err != ErrNotFound {
		t.Fatalf("missing item should be hidden, got %v", err)
	}
	b, _ = s.Begin(ctx, lib.ID)
	b.Seen(ctx, ids[0])
	b.Commit()
	it, err := s.Item(ctx, ids[0])
	if err != nil || it.Progress.Page != 4 {
		t.Fatalf("progress lost after reappearing: %+v %v", it, err)
	}

	b, _ = s.Begin(ctx, lib.ID)
	b.MarkMissing(ctx, ids[1])
	b.Commit()
	s.Now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	n, err := s.Prune(ctx, 30*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
}

func TestItemsFilterAndSearch(t *testing.T) {
	s := openTest(t)
	lib, ids := seed(t, s, "Batman 01.cbz", "Batman 02.cbz", "Saga 1.cbz")
	ctx := context.Background()
	s.SaveProgress(ctx, ids[0], 10, ProgressUpdate{Page: 10})

	items, total, err := s.Items(ctx, ItemFilter{LibraryID: lib.ID, Query: "batman", Limit: 50})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("search: %d %v", total, err)
	}
	_, total, _ = s.Items(ctx, ItemFilter{Status: "read", Limit: 50})
	if total != 1 {
		t.Fatalf("status filter: %d", total)
	}
	_, total, _ = s.Items(ctx, ItemFilter{Query: "100%_", Limit: 50})
	if total != 0 {
		t.Fatal("LIKE wildcards must be escaped")
	}
	series, _, _ := s.SeriesList(ctx, lib.ID, "", 0, 10)
	if len(series) != 1 || series[0].Items != 3 || series[0].Unread != 2 {
		t.Fatalf("series counts: %+v", series)
	}
}

func TestTokensAndSessions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if _, err := s.AddToken(ctx, "panelflow", "h1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.TokenValid(ctx, "h1"); !ok {
		t.Fatal("token should be valid")
	}
	if ok, _ := s.TokenValid(ctx, "nope"); ok {
		t.Fatal("unknown token accepted")
	}
	s.AddSession(ctx, "sess", time.Hour)
	if ok, _ := s.SessionValid(ctx, "sess"); !ok {
		t.Fatal("session should be valid")
	}
	s.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if ok, _ := s.SessionValid(ctx, "sess"); ok {
		t.Fatal("expired session accepted")
	}
}

// Migration 002 rebuilds the item table; progress and tags must survive it.
func TestMigrationKeepsProgressAndTags(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+p+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := migrations.ReadFile("migrations/001_init.sql")
	for _, q := range []string{string(v1), "PRAGMA user_version = 1",
		`INSERT INTO library (id, name, root, kind) VALUES (1, 'L', '/l', 'books')`,
		`INSERT INTO series (id, library_id, path, name, sort_name) VALUES (1, 1, '.', 'S', 's')`,
		`INSERT INTO item (id, library_id, series_id, path, format, size, mtime, title, sort_title, pages, added_at)
		 VALUES (1, 1, 1, 'a.pdf', 'pdf', 1, 1, 'A', 'a', 10, 1)`,
		`INSERT INTO progress (item_id, page, status, updated_at) VALUES (1, 4, 'reading', 5)`,
		`INSERT INTO tag (id, name) VALUES (1, 'Go')`, `INSERT INTO item_tag VALUES (1, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	it, err := s.Item(context.Background(), 1)
	if err != nil || it.Progress.Page != 4 || len(it.Tags) != 1 || it.HasCover {
		t.Fatalf("after migration %+v %v", it, err)
	}
}
