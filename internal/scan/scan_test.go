package scan

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

func newScanner(t *testing.T, libs ...config.Library) (*Scanner, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var sl []store.Library
	for _, l := range libs {
		sl = append(sl, store.Library{Name: l.Name, Root: l.Root, Kind: l.Kind})
	}
	if err := st.SyncLibraries(context.Background(), sl); err != nil {
		t.Fatal(err)
	}
	return &Scanner{Store: st, Libraries: libs, Workers: 2, Limits: formats.Limits{MaxEntryBytes: 1 << 20, MaxEntries: 100},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, st
}

func page() tu.Entry { return tu.Entry{Name: "001.png", Data: tu.PNG(2, 2, 1)} }

func TestComicScanIsIncrementalAndMarksMissing(t *testing.T) {
	root := t.TempDir()
	tu.WriteZip(t, root, "Saga/Saga #01 (2012).cbz", page(), page())
	tu.WriteRAR(t, root, "Saga/Saga #02 (2012).cbr", page())
	tu.WriteZip(t, root, "loose.cbz", page())
	os.WriteFile(filepath.Join(root, "Saga", "notes.txt"), []byte("x"), 0o644)
	sc, st := newScanner(t, config.Library{Name: "Comics", Root: root, Kind: "comics"})
	ctx := context.Background()

	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if s := sc.Status(); s.Added != 3 || s.Running {
		t.Fatalf("status %+v", s)
	}
	items, _, _ := st.Items(ctx, store.ItemFilter{Limit: 10})
	if len(items) != 3 {
		t.Fatalf("items %d", len(items))
	}
	var saga1 store.Item
	for _, it := range items {
		if it.Title == "Saga #01 (2012)" {
			saga1 = it
		}
	}
	if saga1.SeriesName != "Saga" || saga1.Pages != 2 || saga1.Number == nil || *saga1.Number != 1 {
		t.Fatalf("saga1 %+v", saga1)
	}

	// Second run: nothing changed, nothing reopened.
	if err := sc.Run(ctx, ""); err != nil || sc.Status().Added+sc.Status().Updated != 0 {
		t.Fatalf("rescan touched files: %+v %v", sc.Status(), err)
	}

	// Change one, delete one.
	later := time.Now().Add(time.Hour)
	tu.WriteZip(t, root, "loose.cbz", page(), page(), page())
	os.Chtimes(filepath.Join(root, "loose.cbz"), later, later)
	os.Remove(filepath.Join(root, "Saga/Saga #02 (2012).cbr"))
	var changed []int64
	sc.OnChange = func(id int64) { changed = append(changed, id) }
	sc.Run(ctx, "")
	if s := sc.Status(); s.Updated != 1 || s.Missing != 1 || len(changed) != 2 {
		t.Fatalf("status %+v changed %v", s, changed)
	}
	_, total, _ := st.Items(ctx, store.ItemFilter{Limit: 10})
	if total != 2 {
		t.Fatalf("missing item still listed: %d", total)
	}
}

func TestUnavailableRootDoesNotMarkEverythingMissing(t *testing.T) {
	root := t.TempDir()
	tu.WriteZip(t, root, "a.cbz", page())
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	sc.Run(context.Background(), "")
	os.RemoveAll(root)
	if err := sc.Run(context.Background(), ""); err == nil {
		t.Fatal("expected an error for a missing root")
	}
	if _, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10}); total != 1 {
		t.Fatal("items were marked missing because the root vanished")
	}
}

func TestBooksUseIncludeGlobsSourcesAndLabels(t *testing.T) {
	root := t.TempDir()
	tu.WriteEPUB(t, root, "oreilly_downloads/Learning Go.epub", tu.EPUBOpts{Title: "Learning Go", Chapters: []string{"<p>x</p>"}})
	tu.WriteEPUB(t, root, "horror-bookbundle_downloads/Dracula.epub", tu.EPUBOpts{Title: "Dracula", Chapters: []string{"<p>x</p>", "<p>y</p>"}})
	tu.WriteEPUB(t, root, "src/not-a-book.epub", tu.EPUBOpts{Title: "Nope"})
	labels := filepath.Join(t.TempDir(), "labels.json")
	os.WriteFile(labels, []byte(`{"horror-bookbundle": {"label": "Horror Bundle", "year": 2019}, "oreilly": "O'Reilly"}`), 0o644)
	tags := filepath.Join(t.TempDir(), "tags.json")
	os.WriteFile(tags, []byte(`{"oreilly_downloads/Learning Go.epub": ["Go", "Programming"], "gone.epub": ["X"]}`), 0o644)
	sc, st := newScanner(t, config.Library{Name: "Books", Root: root, Kind: "books", Include: []string{"*_downloads"}, Labels: labels, Tags: tags})
	ctx := context.Background()
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	items, total, _ := st.Items(ctx, store.ItemFilter{Sort: "title", Limit: 10})
	if total != 2 {
		t.Fatalf("include glob ignored: %d", total)
	}
	d := items[0]
	if d.Title != "Dracula" || d.Source != "horror-bookbundle" || d.SeriesName != "Horror Bundle (2019)" || d.Pages != 2 {
		t.Fatalf("dracula %+v", d)
	}
	if items[1].SeriesName != "O'Reilly" || len(items[1].Tags) != 2 {
		t.Fatalf("label/tags: %+v", items[1])
	}
	if _, total, _ := st.Items(ctx, store.ItemFilter{Tag: "Go", Limit: 10}); total != 1 {
		t.Fatal("tag filter")
	}
}

func TestSymlinkOutsideRootIsIgnored(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	secret := tu.WriteZip(t, outside, "secret.cbz", page())
	os.Symlink(secret, filepath.Join(root, "link.cbz"))
	inside := tu.WriteZip(t, root, "real.cbz", page())
	os.Symlink(inside, filepath.Join(root, "alias.cbz"))
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	sc.Run(context.Background(), "")
	_, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10})
	if total != 2 {
		t.Fatalf("want real + in-root alias only, got %d", total)
	}
}

func TestNumberFromName(t *testing.T) {
	cases := map[string]float64{
		"Saga #054 (2018) (Digital).cbz": 54,
		"Berserk v12.cbr":                12,
		"One Piece Chapter 1000.5.cbz":   1000.5,
		"Hellboy 03 (1994).cbz":          3,
	}
	for name, want := range cases {
		if got := numberFromName(name); got == nil || *got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	if numberFromName("Watchmen (1987).cbz") != nil {
		t.Error("year taken as number")
	}
}

func TestSymlinkedRootIsScanned(t *testing.T) {
	real, links := t.TempDir(), t.TempDir()
	tu.WriteZip(t, real, "S/a.cbz", page())
	root := filepath.Join(links, "comics")
	os.Symlink(real, root)
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	if err := sc.Run(context.Background(), ""); err != nil || sc.Status().Added != 1 {
		t.Fatalf("symlinked root: %+v %v", sc.Status(), err)
	}
	if _, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10}); total != 1 {
		t.Fatal("item not listed")
	}
}

func TestUnreadableDirectoryDoesNotMarkItsItemsMissing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	root := t.TempDir()
	tu.WriteZip(t, root, "Locked/a.cbz", page())
	tu.WriteZip(t, root, "Open/b.cbz", page())
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	sc.Run(context.Background(), "")
	locked := filepath.Join(root, "Locked")
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	sc.Run(context.Background(), "")
	if s := sc.Status(); s.Missing != 0 {
		t.Fatalf("unreadable dir marked missing: %+v", s)
	}
	if _, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10}); total != 2 {
		t.Fatal("items hidden")
	}
}

// A progress write must not wait on the scanner: rows are buffered, and no
// transaction is open between flushes.
func TestProgressWriteDuringScanIsNotBlocked(t *testing.T) {
	root := t.TempDir()
	tu.WriteZip(t, root, "a.cbz", page())
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	ctx := context.Background()
	sc.Run(ctx, "")
	items, _, _ := st.Items(ctx, store.ItemFilter{Limit: 1})
	libs, _ := st.Libraries(ctx)
	w := &writer{s: sc, ctx: ctx, lib: libs[0]}
	w.add(store.ScannedItem{SeriesPath: ".", SeriesName: "C", Path: "new.cbz", Format: "cbz", Title: "n"}, false)
	start := time.Now()
	if _, _, err := st.SaveProgress(ctx, items[0].ID, 1, store.ProgressUpdate{Page: 1}); err != nil || time.Since(start) > time.Second {
		t.Fatalf("progress blocked by a pending scan batch: %v after %v", err, time.Since(start))
	}
	w.close()
	if w.err != nil {
		t.Fatal(w.err)
	}
}

func TestNASSystemFoldersAreSkipped(t *testing.T) {
	root := t.TempDir()
	tu.WriteZip(t, root, "Conan/Conan 01.cbz", page())
	tu.WriteZip(t, root, "Conan/@eaDir/Conan 01.cbz", page())
	tu.WriteZip(t, root, "#recycle/Old 01.cbz", page())
	sc, st := newScanner(t, config.Library{Name: "C", Root: root, Kind: "comics"})
	sc.Run(context.Background(), "")
	if _, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10}); total != 1 {
		t.Fatalf("NAS system folders were scanned: %d items", total)
	}
}
