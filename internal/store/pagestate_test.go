package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func pageOrderOf(t *testing.T, db *sql.DB, table string, itemID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT page_order FROM "+table+" WHERE item_id = ?", itemID).Scan(&n); err != nil {
		t.Fatalf("%s %d: %v", table, itemID, err)
	}
	return n
}

func hashed(hex string) PanelCorrection {
	return PanelCorrection{ImageHash: strings.Repeat(hex, 32), Enabled: true, PageSize: PanelSize{W: 2, H: 2},
		Direction: "ltr", Panels: []PanelBox{}}
}

func TestUpgradeMarksThePositionsThatDependOnPageOrder(t *testing.T) {
	p := databaseAtVersion4(t)
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO item (id, library_id, series_id, path, format, size, mtime, title, sort_title, pages, added_at, author, has_cover, search)
		 VALUES (50, 1, 7, 'e.epub', 'epub', 1, 1, 'E', 'e', 9, 1, '', 0, 'e'),
		        (51, 1, 7, 'f.cbz', 'cbz', 1, 1, 'F', 'f', 3, 1, '', 1, 'f'),
		        (52, 1, 7, 'g.cbz', 'cbz', 1, 1, 'G', 'g', 3, 1, '', 1, 'g')`,
		`INSERT INTO progress (item_id, page, status, updated_at) VALUES (50, 3, 'reading', 5), (51, 3, 'read', 5), (52, 0, 'unread', 5)`,
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
	// Only a comic being read has a page whose image can change; an EPUB chapter,
	// a finished comic (its last page) and one not started mean the same in both orders.
	for id, want := range map[int64]int{42: 1, 50: 2, 51: 2, 52: 2} {
		if got := pageOrderOf(t, s.DB, "progress", id); got != want {
			t.Errorf("progress of item %d: page_order %d, want %d", id, got, want)
		}
	}
	if got := pageOrderOf(t, s.DB, "panel_correction", 42); got != 1 {
		t.Errorf("correction from before the upgrade: page_order %d, want 1", got)
	}
}

func TestWritesUseTheCurrentNumbering(t *testing.T) {
	s := openTest(t)
	_, ids := seed(t, s, "a.cbz")
	ctx, id := context.Background(), ids[0]
	// A position from before v0.5.0 whose comic has not been remapped yet.
	if _, err := s.DB.Exec("INSERT INTO progress (item_id, page, status, updated_at, page_order) VALUES (?, 7, 'reading', 5, 1)", id); err != nil {
		t.Fatal(err)
	}
	// The two numberings do not compare: a write in the current one replaces it, lower page or not.
	p, applied, err := s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 2})
	if err != nil || !applied || p.Page != 2 || pageOrderOf(t, s.DB, "progress", id) != PageOrder {
		t.Fatalf("progress %+v applied %v err %v, page_order %d", p, applied, err, pageOrderOf(t, s.DB, "progress", id))
	}
	// Within one numbering, the furthest page still wins.
	if _, applied, _ := s.SaveProgress(ctx, id, 10, ProgressUpdate{Page: 1}); applied {
		t.Fatal("a lower page in the same numbering moved progress back")
	}
	saved, err := s.SavePanelCorrection(ctx, id, 3, hashed("ab"))
	if err != nil {
		t.Fatal(err)
	}
	if pageOrderOf(t, s.DB, "panel_correction", id) != PageOrder {
		t.Fatal("a new correction must be stored in the current numbering")
	}
	s.DB.Exec("UPDATE panel_correction SET page_order = 1 WHERE item_id = ?", id)
	edit := hashed("ab")
	edit.Revision = saved.Revision
	if _, err := s.SavePanelCorrection(ctx, id, 3, edit); err != nil {
		t.Fatal(err)
	}
	if pageOrderOf(t, s.DB, "panel_correction", id) != PageOrder {
		t.Fatal("an edited correction describes the page as numbered now")
	}
	s.DB.Exec("UPDATE progress SET page_order = 1 WHERE item_id = ?", id)
	if _, err := s.ImportProgress(ctx, id, Progress{Page: 4, Status: "reading", UpdatedAt: 1 << 40}); err != nil {
		t.Fatal(err)
	}
	if pageOrderOf(t, s.DB, "progress", id) != PageOrder {
		t.Fatal("imported progress is stored in the current numbering")
	}
}

func TestRemapMovesOnlyOldRowsAndKeepsNewerCorrections(t *testing.T) {
	s := openTest(t)
	_, ids := seed(t, s, "a.cbz")
	ctx, id := context.Background(), ids[0]
	for page, hex := range map[int]string{1: "01", 2: "02"} {
		if _, err := s.SavePanelCorrection(ctx, id, page, hashed(hex)); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Exec("UPDATE panel_correction SET page_order = 1 WHERE item_id = ?", id)
	s.DB.Exec("INSERT INTO progress (item_id, page, status, updated_at, page_order) VALUES (?, 2, 'reading', 5, 1)", id)
	// Written since the upgrade, on the page old page 1 now maps to.
	if _, err := s.SavePanelCorrection(ctx, id, 3, hashed("ee")); err != nil {
		t.Fatal(err)
	}
	to := map[int]int{1: 3, 2: 1, 3: 2}
	for range 2 { // the second run finds nothing old left to move
		if err := s.RemapPages(ctx, id, to); err != nil {
			t.Fatal(err)
		}
		at := func(page int) string {
			c, _ := s.PanelCorrection(ctx, id, page)
			if c == nil {
				return ""
			}
			return c.ImageHash[:2]
		}
		if at(1) != "02" || at(2) != "" || at(3) != "ee" {
			t.Fatalf("corrections at 1,2,3: %q %q %q; want old page 2's on 1 and the newer one kept on 3", at(1), at(2), at(3))
		}
		if p, _ := s.ProgressOf(ctx, id); p.Page != 1 {
			t.Fatalf("progress on page %d, want 1", p.Page)
		}
	}
	var old int
	s.DB.QueryRow("SELECT (SELECT COUNT(*) FROM progress WHERE page_order < 2) + (SELECT COUNT(*) FROM panel_correction WHERE page_order < 2)").Scan(&old)
	if old != 0 {
		t.Fatalf("%d rows still in the old numbering", old)
	}
}
