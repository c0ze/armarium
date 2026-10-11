package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

// nested is a comic whose cover sits at the archive root beside a chapter
// folder. Before v0.5.0 its pages were chapter1/01, chapter1/02, cover; now
// they are cover, chapter1/01, chapter1/02.
func nested() []tu.Entry {
	return []tu.Entry{
		{Name: "chapter1/01.png", Data: tu.PNG(2, 2, 10)},
		{Name: "chapter1/02.png", Data: tu.PNG(2, 2, 20)},
		{Name: "cover.png", Data: tu.PNG(2, 2, 30)},
	}
}

func correction(t *testing.T, st *store.Store, itemID int64, page int) {
	t.Helper()
	c := store.PanelCorrection{ImageHash: strings.Repeat("ab", 32), Enabled: true, PageSize: store.PanelSize{W: 2, H: 2},
		Direction: "ltr", Panels: []store.PanelBox{}}
	if _, err := st.SavePanelCorrection(context.Background(), itemID, page, c); err != nil {
		t.Fatal(err)
	}
}

func pageOf(t *testing.T, st *store.Store, itemID int64) store.Progress {
	t.Helper()
	p, err := st.ProgressOf(context.Background(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func correctionAt(st *store.Store, itemID int64, page int) bool {
	c, _ := st.PanelCorrection(context.Background(), itemID, page)
	return c != nil
}

// beforeTheUpgrade marks an item's state as written in the old numbering, as
// migration 006 does for an installation upgrading to v0.5.0.
func beforeTheUpgrade(t *testing.T, st *store.Store, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		for _, q := range []string{
			"UPDATE progress SET page_order = 1 WHERE item_id = ? AND page > 0 AND status <> 'read'",
			"UPDATE panel_correction SET page_order = 1 WHERE item_id = ?",
		} {
			if _, err := st.DB.Exec(q, id); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// scanned builds a library from the files and returns item ids by path.
func scanned(t *testing.T, files map[string][]tu.Entry) (*Scanner, *store.Store, map[string]int64, string) {
	t.Helper()
	root := t.TempDir()
	for name, entries := range files {
		tu.WriteZip(t, root, name, entries...)
	}
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	ctx := context.Background()
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	libs, _ := st.Libraries(ctx)
	known, _ := st.KnownItems(ctx, libs[0].ID)
	ids := map[string]int64{}
	for p, k := range known {
		ids[p] = k.ID
	}
	return sc, st, ids, root
}

func TestRemapPageOrderKeepsEachPlace(t *testing.T) {
	sc, st, ids, _ := scanned(t, map[string][]tu.Entry{"S/nested.cbz": nested(), "S/done.cbz": nested()})
	ctx := context.Background()
	reading, done := ids["S/nested.cbz"], ids["S/done.cbz"]
	st.SaveProgress(ctx, reading, 3, store.ProgressUpdate{Page: 2})              // old page 2: chapter1/02
	st.SaveProgress(ctx, done, 3, store.ProgressUpdate{Page: 3, Status: "read"}) // finished: the end of the book
	correction(t, st, reading, 3)                                                // drawn on the cover (old page 3)
	beforeTheUpgrade(t, st, reading, done)
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, reading); p.Page != 3 || p.Status != "reading" {
		t.Fatalf("reading position: %+v, want page 3 (chapter1/02)", p)
	}
	if p := pageOf(t, st, done); p.Page != 3 || p.Status != "read" {
		t.Fatalf("a finished comic stays at its end: %+v", p)
	}
	if !correctionAt(st, reading, 1) || correctionAt(st, reading, 3) {
		t.Fatal("the cover's correction must move to page 1")
	}
	// A second start changes nothing.
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, reading); p.Page != 3 || !correctionAt(st, reading, 1) {
		t.Fatalf("second run moved pages again: %+v", p)
	}
}

func TestRemapPageOrderIsCrashSafe(t *testing.T) {
	sc, st, ids, root := scanned(t, map[string][]tu.Entry{"S/nested.cbz": nested(), "S/gone.cbz": nested()})
	ctx := context.Background()
	id, gone := ids["S/nested.cbz"], ids["S/gone.cbz"]
	st.SaveProgress(ctx, id, 3, store.ProgressUpdate{Page: 2})
	st.SaveProgress(ctx, gone, 3, store.ProgressUpdate{Page: 2})
	beforeTheUpgrade(t, st, id, gone)
	// A crash after this item's own transaction, before the others.
	if err := st.RemapPages(ctx, id, pageMapping([]string{"cover.png", "chapter1/01.png", "chapter1/02.png"})); err != nil {
		t.Fatal(err)
	}
	// An archive that cannot be opened now is retried at the next start.
	os.Remove(filepath.Join(root, "S/gone.cbz"))
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, id); p.Page != 3 {
		t.Fatalf("an item already remapped was mapped again: page %d", p.Page)
	}
	if p := pageOf(t, st, gone); p.Page != 2 {
		t.Fatalf("an archive that cannot be opened keeps its old page for a later start: page %d", p.Page)
	}
}

// The first start could not open one comic, so the remap runs again at the next
// start. A comic first read and corrected in between must keep its new numbers.
func TestRemapLeavesStateWrittenSince(t *testing.T) {
	sc, st, ids, root := scanned(t, map[string][]tu.Entry{"S/gone.cbz": nested(), "S/later.cbz": nested()})
	ctx := context.Background()
	gone, later := ids["S/gone.cbz"], ids["S/later.cbz"]
	st.SaveProgress(ctx, gone, 3, store.ProgressUpdate{Page: 2})
	beforeTheUpgrade(t, st, gone)
	os.Remove(filepath.Join(root, "S/gone.cbz"))
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	st.SaveProgress(ctx, later, 3, store.ProgressUpdate{Page: 1}) // the cover, in the new numbering
	correction(t, st, later, 1)
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, later); p.Page != 1 || !correctionAt(st, later, 1) || correctionAt(st, later, 2) {
		t.Fatalf("state written since the upgrade was remapped: page %d", p.Page)
	}
}

// A comic missing when the remap ran keeps its old numbers until it is back, and
// is remapped by the scan that finds it again.
func TestComicMissingAtTheUpgradeIsRemappedWhenItReturns(t *testing.T) {
	sc, st, ids, root := scanned(t, map[string][]tu.Entry{"S/away.cbz": nested()})
	ctx := context.Background()
	id := ids["S/away.cbz"]
	st.SaveProgress(ctx, id, 3, store.ProgressUpdate{Page: 2}) // old page 2: chapter1/02
	beforeTheUpgrade(t, st, id)
	aside := filepath.Join(t.TempDir(), "away.cbz")
	if err := os.Rename(filepath.Join(root, "S/away.cbz"), aside); err != nil {
		t.Fatal(err)
	}
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, id); p.Page != 2 {
		t.Fatalf("a missing comic was remapped without its archive: page %d", p.Page)
	}
	if err := os.Rename(aside, filepath.Join(root, "S/away.cbz")); err != nil {
		t.Fatal(err)
	}
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if p := pageOf(t, st, id); p.Page != 3 {
		t.Fatalf("the returning comic resumes on page %d, want 3 (chapter1/02)", p.Page)
	}
}

func TestPageMappingFollowsRepeatedNames(t *testing.T) {
	// Archive order: two entries named ch/a.png, then ch/b.png, then the cover.
	// Old: a#1, a#2, b, cover. New: cover, a#1, a#2, b.
	got := pageMapping([]string{"cover.png", "ch/a.png", "ch/a.png", "ch/b.png"})
	want := map[int]int{1: 2, 2: 3, 3: 4, 4: 1}
	for old, new := range want {
		if got[old] != new {
			t.Fatalf("mapping %v, want %v", got, want)
		}
	}
	if pageMapping([]string{"01.png", "02.png"}) != nil {
		t.Fatal("an unchanged order needs no mapping")
	}
}

func TestRemapStagesCorrectionsClearOfExistingPages(t *testing.T) {
	sc, st, ids, _ := scanned(t, map[string][]tu.Entry{"S/nested.cbz": nested()})
	ctx := context.Background()
	id := ids["S/nested.cbz"]
	correction(t, st, id, 2)
	correction(t, st, id, 1000002) // valid in the table when max_entries is large
	beforeTheUpgrade(t, st, id)
	if err := sc.RemapPageOrder(ctx); err != nil {
		t.Fatal(err)
	}
	if !correctionAt(st, id, 3) || !correctionAt(st, id, 1000002) {
		t.Fatal("old page 2 (chapter1/02) belongs on page 3; the out-of-range row stays where it was")
	}
}
