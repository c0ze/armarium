package calibre

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	tu "github.com/c0ze/armarium/internal/testutil"
)

func TestReadLibrary(t *testing.T) {
	root := t.TempDir()
	tu.WriteCalibre(t, root,
		tu.CalibreBook{Title: "Infection", Author: "John Betancourt", Series: "Double Helix", SeriesIndex: 1,
			Tags: []string{"Star Trek", "SF"}, Cover: tu.PNG(2, 3, 1),
			Files: map[string][]byte{"MOBI": []byte("mobi"), "EPUB": []byte("epub")}},
		tu.CalibreBook{Title: "Standalone", Author: "Ann Other", Files: map[string][]byte{"MOBI": []byte("m")}},
	)
	books, err := Read(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("books %d", len(books))
	}
	b := books[0]
	if b.Title != "Infection" || b.Authors[0] != "John Betancourt" || b.Series != "Double Helix" || b.SeriesIndex != 1 ||
		len(b.Tags) != 2 || !b.HasCover {
		t.Fatalf("book %+v", b)
	}
	if len(b.Files) != 2 || b.Files[0].Format != "epub" || b.Files[0].Path != "John Betancourt/Infection (1)/Infection - John Betancourt.epub" {
		t.Fatalf("files %+v (EPUB must rank first)", b.Files)
	}
	if books[1].Series != "" || books[1].Files[0].Format != "mobi" {
		t.Fatalf("standalone %+v", books[1])
	}
}

func TestReadRefusesEscapingPathsAndNonCalibreFolders(t *testing.T) {
	root := t.TempDir()
	tu.WriteCalibre(t, root, tu.CalibreBook{Title: "Evil", Author: "X", Files: map[string][]byte{"EPUB": []byte("e")}})
	db, _ := sql.Open("sqlite", filepath.Join(root, "metadata.db"))
	db.Exec(`UPDATE books SET path = '../../etc'`)
	db.Close()
	books, err := Read(context.Background(), root)
	if err != nil || len(books) != 0 {
		t.Fatalf("escaping book kept: %+v %v", books, err)
	}
	if _, err := Read(context.Background(), t.TempDir()); err == nil {
		t.Fatal("a folder without metadata.db must be an error")
	}
}
