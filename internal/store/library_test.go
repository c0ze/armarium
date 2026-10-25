package store

import (
	"context"
	"errors"
	"testing"
)

var comics, books = Library{Name: "Comics", Root: "/c", Kind: "comics"}, Library{Name: "Books", Root: "/b", Kind: "books"}

func TestSyncLibrariesNeverDeletes(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, ids := seed(t, s, "a.cbz")
	if err := s.SyncLibraries(ctx, []Library{comics, books}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SaveProgress(ctx, ids[0], 10, ProgressUpdate{Page: 4}); err != nil {
		t.Fatal(err)
	}
	// A rename (or a typo) in the config: the old library is not configured any more.
	err := s.SyncLibraries(ctx, []Library{{Name: "Comics2", Root: "/c", Kind: "comics"}, books})
	var unconfigured *UnconfiguredError
	if !errors.As(err, &unconfigured) || len(unconfigured.Names) != 1 || unconfigured.Names[0] != "Comics" {
		t.Fatalf("want UnconfiguredError{Comics}, got %v", err)
	}
	if p, _ := s.ProgressOf(ctx, ids[0]); p.Page != 4 {
		t.Fatalf("progress lost: %+v", p)
	}
	libs, _ := s.Libraries(ctx)
	for _, l := range libs {
		if l.Name == "Comics2" {
			t.Fatal("a refused sync must not add libraries")
		}
	}
}

func TestRenameLibraryKeepsItems(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, ids := seed(t, s, "a.cbz")
	s.SaveProgress(ctx, ids[0], 10, ProgressUpdate{Page: 4})
	if err := s.RenameLibrary(ctx, "Comics", "Comics2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncLibraries(ctx, []Library{{Name: "Comics2", Root: "/c", Kind: "comics"}, books}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.ProgressOf(ctx, ids[0]); p.Page != 4 {
		t.Fatalf("progress lost: %+v", p)
	}
	if err := s.RenameLibrary(ctx, "Missing", "X"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename of a missing library: %v", err)
	}
	if err := s.RenameLibrary(ctx, "Comics2", "Books"); err == nil {
		t.Fatal("rename onto an existing library must fail")
	}
}

func TestRemoveLibraryCascades(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	_, ids := seed(t, s, "a.cbz", "b.cbz")
	n, err := s.RemoveLibrary(ctx, "Comics")
	if err != nil || n != 2 {
		t.Fatalf("removed %d items: %v", n, err)
	}
	if _, err := s.Item(ctx, ids[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("item survived: %v", err)
	}
	if _, err := s.RemoveLibrary(ctx, "Comics"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestLibraryItemCounts(t *testing.T) {
	s := openTest(t)
	lib, _ := seed(t, s, "A/1.cbz", "A/2.cbz", "B/3.cbz")
	counts, err := s.LibraryItemCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts[lib.Name] != 3 {
		t.Fatalf("counts %v", counts)
	}
}
