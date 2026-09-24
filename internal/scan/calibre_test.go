package scan

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

func TestCalibreLibraryUsesMetadataDB(t *testing.T) {
	root := t.TempDir()
	epub := tu.WriteEPUB(t, t.TempDir(), "x.epub", tu.EPUBOpts{Title: "Ignored", Chapters: []string{"<p>1</p>", "<p>2</p>"}})
	epubBytes, _ := os.ReadFile(epub)
	tu.WriteCalibre(t, root,
		tu.CalibreBook{Title: "Infection", Author: "John Betancourt", Series: "Double Helix", SeriesIndex: 1,
			Tags: []string{"Star Trek"}, Cover: tu.PNG(4, 6, 9),
			Files: map[string][]byte{"EPUB": epubBytes, "MOBI": []byte("mobi")}},
		tu.CalibreBook{Title: "Standalone", Author: "Ann Other", Files: map[string][]byte{"MOBI": []byte("m")}},
		tu.CalibreBook{Title: "Gone", Author: "Ann Other", Files: map[string][]byte{"EPUB": []byte("e")}},
	)
	os.RemoveAll(filepath.Join(root, "Ann Other", "Gone (3)"))
	sc, st := newScanner(t, config.Library{Name: "Calibre", Root: root, Kind: "books", Calibre: true})
	ctx := context.Background()
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	items, total, _ := st.Items(ctx, store.ItemFilter{Sort: "title", Limit: 10})
	if total != 2 {
		t.Fatalf("want 2 books (one per book, missing files skipped), got %d", total)
	}
	inf, alone := items[0], items[1]
	if inf.Title != "Infection" || inf.Author != "John Betancourt" || inf.SeriesName != "Double Helix" ||
		inf.Number == nil || *inf.Number != 1 || inf.Format != "epub" || inf.Pages != 2 || !inf.HasCover ||
		len(inf.Formats) != 1 || inf.Formats[0] != "mobi" || len(inf.Tags) != 1 {
		t.Fatalf("series book %+v", inf)
	}
	if alone.Format != "mobi" || alone.SeriesName != "Ann Other" || alone.HasCover || alone.Pages != 0 {
		t.Fatalf("standalone %+v", alone)
	}
	if p, err := st.ItemFile(ctx, inf.ID, "mobi"); err != nil || filepath.Base(p) != "Infection - John Betancourt.mobi" {
		t.Fatalf("extra format: %q %v", p, err)
	}

	// Edits in Calibre reach Armarium on the next scan without reopening files.
	db, _ := sql.Open("sqlite", filepath.Join(root, "metadata.db"))
	db.Exec(`UPDATE books SET title = 'Infection (Revised)' WHERE id = 1`)
	db.Close()
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if s := sc.Status(); s.Added+s.Updated != 0 {
		t.Fatalf("metadata refresh counted as file changes: %+v", s)
	}
	if it, _ := st.Item(ctx, inf.ID); it.Title != "Infection (Revised)" || it.Pages != 2 {
		t.Fatalf("after edit %+v", it)
	}
}

func TestCalibreLibraryWithoutMetadataDBFailsSafely(t *testing.T) {
	root := t.TempDir()
	sc, _ := newScanner(t, config.Library{Name: "C", Root: root, Kind: "books", Calibre: true})
	if err := sc.Run(context.Background(), ""); err == nil {
		t.Fatal("expected an error without metadata.db")
	}
}

func TestFolderScanListsDownloadOnlyFormats(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "a_downloads"), 0o755)
	os.WriteFile(filepath.Join(root, "a_downloads", "Book.mobi"), []byte("m"), 0o644)
	os.WriteFile(filepath.Join(root, "a_downloads", "README.txt"), []byte("r"), 0o644)
	sc, st := newScanner(t, config.Library{Name: "B", Root: root, Kind: "books"})
	sc.Run(context.Background(), "")
	items, total, _ := st.Items(context.Background(), store.ItemFilter{Limit: 10})
	if total != 1 || items[0].Format != "mobi" || items[0].HasCover {
		t.Fatalf("items %+v", items)
	}
}

func TestCalibreSkipsEmptyFilesAndReportsBrokenOnes(t *testing.T) {
	root := t.TempDir()
	tu.WriteCalibre(t, root,
		tu.CalibreBook{Title: "Empty Epub", Author: "A", Files: map[string][]byte{"EPUB": {}, "PDF": []byte("%PDF-1.4")}},
		tu.CalibreBook{Title: "Only Empty", Author: "A", Files: map[string][]byte{"EPUB": {}}},
		tu.CalibreBook{Title: "Broken", Author: "A", Files: map[string][]byte{"EPUB": []byte("not a zip")}},
	)
	sc, st := newScanner(t, config.Library{Name: "Calibre", Root: root, Kind: "books", Calibre: true})
	ctx := context.Background()
	if err := sc.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if s := sc.Status(); s.Skipped != 1 || s.Errors != 1 {
		t.Fatalf("status %+v, want 1 skipped (Only Empty) and 1 error (Broken)", s)
	}
	items, _, _ := st.Items(ctx, store.ItemFilter{Sort: "title", Limit: 10})
	if len(items) != 2 || items[1].Title != "Empty Epub" || items[1].Format != "pdf" {
		t.Fatalf("empty EPUB should fall back to the PDF: %+v", items)
	}
}
