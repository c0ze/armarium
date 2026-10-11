package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
)

// databaseAtVersion4 builds the schema every installation had before 005, with
// rows in every table that references item or series, and returns its path.
func databaseAtVersion4(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// As migrate does: rebuilding a table must not cascade into its dependents.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_init.sql", "002_calibre.sql", "003_search.sql", "004_panel_corrections.sql"} {
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := strconv.Atoi(name[:3])
		if err := apply(ctx, conn, string(body), n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO library (id, name, root, kind) VALUES (1, 'Comics', '/c', 'comics'), (2, 'Books', '/b', 'books')`,
		`INSERT INTO series (id, library_id, path, name, sort_name, search) VALUES (7, 1, 'S', 'S', 's', 's')`,
		`INSERT INTO item (id, library_id, series_id, path, format, size, mtime, title, sort_title, pages, added_at, author, has_cover, search)
		 VALUES (10, 1, 7, 'a.cbz', 'cbz', 1, 1, 'A', 'a', 3, 1, '', 1, 'a'),
		        (11, 1, 7, 'b.cbz', 'cbz', 1, 1, 'B', 'b', 3, 1, '', 1, 'b'),
		        (42, 1, 7, 'c.cbz', 'cbz', 1, 1, 'C', 'c', 3, 1, '', 1, 'c')`,
		`INSERT INTO progress (item_id, page, status, updated_at) VALUES (42, 2, 'reading', 5)`,
		`INSERT INTO panel_correction (item_id, page, revision, document) VALUES (42, 1, 1, '{}')`,
		`INSERT INTO tag (id, name) VALUES (3, 'x')`,
		`INSERT INTO item_tag (item_id, tag_id) VALUES (11, 3)`,
		`INSERT INTO item_file (item_id, format, path, size) VALUES (10, 'mobi', 'a.mobi', 1)`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return p
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestUpgradeKeepsIDsAndDependents(t *testing.T) {
	s, err := Open(databaseAtVersion4(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.DB
	if v := count(t, db, "PRAGMA user_version"); v < 5 {
		t.Fatalf("user_version %d", v)
	}
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM item WHERE id IN (10, 11, 42) AND series_id = 7":                              3,
		"SELECT COUNT(*) FROM series WHERE id = 7 AND library_id = 1":                                       1,
		"SELECT COUNT(*) FROM progress WHERE item_id = 42 AND page = 2":                                     1,
		"SELECT COUNT(*) FROM panel_correction WHERE item_id = 42":                                          1,
		"SELECT COUNT(*) FROM item_tag WHERE item_id = 11 AND tag_id = 3":                                   1,
		"SELECT COUNT(*) FROM item_file WHERE item_id = 10 AND format = 'mobi'":                             1,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN ('item_series', 'item_added')": 2,
		"SELECT COUNT(*) FROM sqlite_sequence WHERE name = 'item' AND seq = 42":                             1,
		"SELECT COUNT(*) FROM sqlite_sequence WHERE name = 'series' AND seq = 7":                            1,
	} {
		if got := count(t, db, q); got != want {
			t.Errorf("%s = %d, want %d", q, got, want)
		}
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key check failed after the upgrade")
	}
}

func TestDeletedIDsAreNeverReused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ids.db")
	ctx := context.Background()
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	_, ids := seed(t, s, "a.cbz", "b.cbz", "c.cbz")
	top := ids[2]
	var series int64
	s.DB.QueryRow("SELECT series_id FROM item WHERE id = ?", top).Scan(&series)
	// The newest item and its series go, as a pruned file or removed library would.
	if _, err := s.DB.Exec("DELETE FROM item WHERE id = ?", top); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("DELETE FROM item WHERE series_id = ?", series); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("DELETE FROM series WHERE id = ?", series); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s, err = Open(p); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	libs, _ := s.Libraries(ctx)
	b, _ := s.Begin(ctx, libs[0].ID)
	if err := b.Upsert(ctx, ScannedItem{SeriesPath: "T", SeriesName: "T", SeriesSort: "t", Path: "d.cbz",
		Format: "cbz", Title: "d", SortTitle: "d", Pages: 1, Size: 1, MTime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	known, _ := s.KnownItems(ctx, libs[0].ID)
	var newSeries int64
	s.DB.QueryRow("SELECT series_id FROM item WHERE id = ?", known["d.cbz"].ID).Scan(&newSeries)
	if known["d.cbz"].ID <= top || newSeries <= series {
		t.Fatalf("ids reused: item %d (deleted %d), series %d (deleted %d)", known["d.cbz"].ID, top, newSeries, series)
	}
}
