package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/c0ze/armarium/internal/store"
)

// setup makes a Armarium store with one library holding the given paths.
func setup(t *testing.T, kind string, paths ...string) (*store.Store, int64, map[string]int64) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	st.SyncLibraries(ctx, []store.Library{{Name: "L", Root: "/x", Kind: kind}})
	libs, _ := st.Libraries(ctx)
	b, _ := st.Begin(ctx, libs[0].ID)
	for _, p := range paths {
		format := p[len(p)-3:]
		if format == "pub" {
			format = "epub"
		}
		b.Upsert(ctx, store.ScannedItem{SeriesPath: ".", SeriesName: "S", Path: p, Format: format, Title: p, Pages: 10})
	}
	b.Commit()
	known, _ := st.KnownItems(ctx, libs[0].ID)
	ids := map[string]int64{}
	for p, k := range known {
		ids[p] = k.ID
	}
	return st, libs[0].ID, ids
}

func sourceDB(t *testing.T, stmts ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "src.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return p
}

func TestSkrivistImport(t *testing.T) {
	st, lib, ids := setup(t, "books", "oreilly_downloads/Go.epub", "oreilly_downloads/Go.pdf", "manning_downloads/Rust.epub")
	src := sourceDB(t,
		`CREATE TABLE files (id INTEGER PRIMARY KEY, book_id INT, path TEXT, format TEXT)`,
		`CREATE TABLE progress (book_id INT PRIMARY KEY, chapter INT, scroll REAL, updated REAL, completed INT, total_chapters INT)`,
		`CREATE TABLE book_topics (book_id INT, topic TEXT)`,
		`INSERT INTO files VALUES (1, 1, 'oreilly_downloads/Go.epub', 'epub'), (2, 1, 'oreilly_downloads/Go.pdf', 'pdf'),
		 (3, 2, 'manning_downloads/Rust.epub', 'epub'), (4, 3, 'gone_downloads/X.epub', 'epub')`,
		`INSERT INTO progress VALUES (1, 4, 0.5, 1700000000.5, 0, 0), (2, 9, 0.1, 1700000001, 1, 0), (3, 1, 0, 1700000002, 0, 0)`,
		`INSERT INTO book_topics VALUES (1, 'Go'), (1, 'Programming')`,
	)
	ctx := context.Background()
	res, err := Skrivist(ctx, st, lib, src)
	if err != nil {
		t.Fatal(err)
	}
	if res.Progress != 3 || res.Skipped != 1 || res.Tags != 2 {
		t.Fatalf("result %+v", res)
	}
	goEpub, _ := st.Item(ctx, ids["oreilly_downloads/Go.epub"])
	if goEpub.Progress.Page != 5 || goEpub.Progress.Status != "reading" || goEpub.Progress.Locator != `{"chapter":4,"scroll":0.5}` || len(goEpub.Tags) != 2 {
		t.Fatalf("go epub %+v", goEpub)
	}
	goPDF, _ := st.Item(ctx, ids["oreilly_downloads/Go.pdf"])
	if goPDF.Progress.Page != 0 || goPDF.Progress.Status != "reading" {
		t.Fatalf("go pdf %+v", goPDF.Progress)
	}
	rust, _ := st.Item(ctx, ids["manning_downloads/Rust.epub"])
	if rust.Progress.Status != "read" {
		t.Fatalf("rust %+v", rust.Progress)
	}
	// Re-running must not overwrite newer Armarium progress.
	st.SaveProgress(ctx, rust.ID, 10, store.ProgressUpdate{Page: 1, Status: "unread", Reset: true})
	Skrivist(ctx, st, lib, src)
	if again, _ := st.Item(ctx, rust.ID); again.Progress.Status != "unread" {
		t.Fatal("import clobbered newer progress")
	}
}

func TestKavitaImport(t *testing.T) {
	st, lib, ids := setup(t, "comics", "Saga/Saga 01.cbz", "Saga/Saga 02.cbz",
		"Pack/Part 1.cbz", "Pack/Part 2.cbz", "Pack/Part 10.cbz")
	src := sourceDB(t,
		`CREATE TABLE Chapter (Id INTEGER PRIMARY KEY, Pages INT)`,
		`CREATE TABLE MangaFile (Id INTEGER PRIMARY KEY, FilePath TEXT, ChapterId INT, Pages INT)`,
		`CREATE TABLE AppUserProgresses (Id INTEGER PRIMARY KEY, PagesRead INT, ChapterId INT, AppUserId INT, LastModifiedUtc TEXT)`,
		`INSERT INTO Chapter VALUES (1, 10), (2, 10), (3, 5), (4, 30)`,
		`INSERT INTO MangaFile VALUES (1, '/comics/Saga/Saga 01.cbz', 1, 10), (2, '/comics/Saga/Saga 02.cbz', 2, 10),
		 (3, '/comics/Other/x.cbz', 3, 5),
		 (4, '/comics/Pack/Part 10.cbz', 4, 10), (5, '/comics/Pack/Part 2.cbz', 4, 10), (6, '/comics/Pack/Part 1.cbz', 4, 10)`,
		`INSERT INTO AppUserProgresses VALUES (1, 4, 1, 1, '2026-01-02 03:04:05.1234567'), (2, 10, 2, 1, '2026-01-02 03:04:05'),
		 (3, 2, 3, 1, '2026-01-01 00:00:00'), (4, 13, 4, 1, '2026-01-03 00:00:00')`,
	)
	ctx := context.Background()
	res, err := Kavita(ctx, st, lib, src, "/comics/")
	if err != nil {
		t.Fatal(err)
	}
	if res.Progress != 4 || res.Skipped != 1 {
		t.Fatalf("result %+v", res)
	}
	get := func(p string) store.Progress { it, _ := st.Item(ctx, ids[p]); return it.Progress }
	if p := get("Saga/Saga 01.cbz"); p.Page != 4 || p.Status != "reading" || p.UpdatedAt != 1767323045 {
		t.Fatalf("saga 01 %+v", p)
	}
	if get("Saga/Saga 02.cbz").Status != "read" {
		t.Fatal("saga 02 should be read")
	}
	// One Kavita chapter over three files, 13 pages read: Part 1 read, Part 2 on
	// page 3, Part 10 untouched (natural order, not "Part 10" before "Part 2").
	if p := get("Pack/Part 1.cbz"); p.Status != "read" || p.Page != 10 {
		t.Fatalf("part 1 %+v", p)
	}
	if p := get("Pack/Part 2.cbz"); p.Status != "reading" || p.Page != 3 {
		t.Fatalf("part 2 %+v", p)
	}
	if p := get("Pack/Part 10.cbz"); p.Status != "unread" {
		t.Fatalf("part 10 %+v", p)
	}
}
