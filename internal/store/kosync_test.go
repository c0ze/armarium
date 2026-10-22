package store

import (
	"context"
	"errors"
	"testing"
)

func TestKOSyncRecords(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, ids := seed(t, s, "a.cbz", "b.cbz")

	if _, err := s.KOSyncDocument(ctx, "d1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown document: %v", err)
	}
	if err := s.LinkKOSyncDocument(ctx, "d1", ids[0]); err != nil {
		t.Fatal(err)
	}
	r, err := s.KOSyncDocument(ctx, "d1")
	if err != nil || r.ItemID != ids[0] || r.UpdatedAt != 0 {
		t.Fatalf("linked: %+v, %v", r, err)
	}

	up := KOSyncRecord{Document: "d1", Progress: "/body/DocFragment[2]/body", Percentage: 0.5,
		Device: "CrossPoint", DeviceID: "crosspoint-reader", UpdatedAt: 100}
	if err := s.SaveKOSync(ctx, up); err != nil {
		t.Fatal(err)
	}
	r, _ = s.KOSyncDocument(ctx, "d1")
	up.ItemID = ids[0] // an upload keeps the link
	if r != up {
		t.Fatalf("saved: %+v, want %+v", r, up)
	}

	// Linking again (another download) keeps the position.
	if err := s.LinkKOSyncDocument(ctx, "d1", ids[1]); err != nil {
		t.Fatal(err)
	}
	r, _ = s.KOSyncDocument(ctx, "d1")
	if r.ItemID != ids[1] || r.Progress != up.Progress || r.UpdatedAt != 100 {
		t.Fatalf("relinked: %+v", r)
	}

	// A document nobody linked still keeps what a device uploaded.
	if err := s.SaveKOSync(ctx, KOSyncRecord{Document: "d2", Progress: "x", UpdatedAt: 5}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.KOSyncDocument(ctx, "d2"); r.ItemID != 0 || r.Progress != "x" {
		t.Fatalf("unlinked: %+v", r)
	}

	// Removing the library unlinks the document but keeps the record.
	if _, err := s.RemoveLibrary(ctx, "Comics"); err != nil {
		t.Fatal(err)
	}
	if r, err := s.KOSyncDocument(ctx, "d1"); err != nil || r.ItemID != 0 || r.Progress != up.Progress {
		t.Fatalf("after removal: %+v, %v", r, err)
	}
}

func TestEPUBCandidates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SyncLibraries(ctx, []Library{{Name: "Books", Root: "/b", Kind: "books"}}); err != nil {
		t.Fatal(err)
	}
	libs, _ := s.Libraries(ctx)
	b, _ := s.Begin(ctx, libs[0].ID)
	for _, it := range []ScannedItem{
		{Path: "les.epub", Format: "epub", Title: "Les Misérables", Author: "Victor Hugo"},
		{Path: "les-cal/les.pdf", Format: "pdf", Title: "Les Miserables", Author: "Someone Else",
			Extra: []ExtraFile{{Format: "epub", Path: "les-cal/les.epub", Size: 1}}},
		{Path: "les.pdf", Format: "pdf", Title: "Les Misérables", Author: "Victor Hugo"}, // no EPUB
		{Path: "other.epub", Format: "epub", Title: "Other", Author: "Victor Hugo"},
	} {
		it.SeriesPath, it.SeriesName, it.SeriesSort, it.SortTitle, it.Size, it.MTime = "S", "S", "s", it.Title, 1, 1
		it.Calibre = len(it.Extra) > 0
		if err := b.Upsert(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}

	got, err := s.EPUBCandidates(ctx, "LES MISERABLES", 10)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, c := range got {
		if c.LibraryID != libs[0].ID || c.ItemID == 0 {
			t.Errorf("candidate %+v", c)
		}
		paths[c.Path] = c.Author
	}
	if len(paths) != 2 || paths["les.epub"] != "Victor Hugo" || paths["les-cal/les.epub"] != "Someone Else" {
		t.Fatalf("candidates = %+v", got)
	}
	if got, _ := s.EPUBCandidates(ctx, "les miserables", 1); len(got) != 1 {
		t.Fatalf("limit: %+v", got)
	}
	if got, _ := s.EPUBCandidates(ctx, "", 10); len(got) != 0 {
		t.Fatalf("empty title: %+v", got)
	}
}
