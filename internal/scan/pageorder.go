package scan

import (
	"context"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/c0ze/armarium/internal/formats"
)

// RemapPageOrder moves reading positions and panel corrections stored in the
// numbering before v0.5.0 to the root-first page numbers, so comics whose pages
// were numbered differently keep each reader's place and every correction on
// its own image. Every row records the numbering it was written in, so only
// rows from before the upgrade move, each once, however often this runs. A
// comic that cannot be opened or is missing keeps its rows for a later start
// or scan. It runs at every start and after every scan; with nothing left in
// the old numbering it is one query.
func (s *Scanner) RemapPageOrder(ctx context.Context) error {
	items, err := s.Store.ItemsWithOldPageNumbers(ctx)
	if err != nil || len(items) == 0 {
		return err
	}
	s.Log.Info("page order: renumbering stored pages", "comics", len(items))
	failed := 0
	for _, it := range items {
		lib, err := s.Store.Library(ctx, it.LibraryID)
		if err != nil {
			return err
		}
		c, err := formats.Open(filepath.Join(lib.Root, filepath.FromSlash(it.Path)), it.Format, s.Limits)
		if err != nil {
			s.Log.Warn("page order: cannot open; will retry", "item", it.ID, "err", err)
			failed++
			continue
		}
		names := make([]string, c.Pages())
		for i := range names {
			names[i] = c.Name(i)
		}
		c.Close()
		if err := s.Store.RemapPages(ctx, it.ID, pageMapping(names)); err != nil {
			return err
		}
	}
	s.Log.Info("page order: stored pages renumbered", "comics", len(items)-failed, "retry_later", failed)
	return nil
}

// pageMapping maps each old page number (NaturalLess order, before v0.5.0) to
// its number in the current order, given the current order's names; nil when
// nothing moves. Both sorts are stable over the archive's own order, so the
// k-th copy of a repeated name is the same entry in both: repeats are matched
// by occurrence.
func pageMapping(names []string) map[int]int {
	key := func(list []string) []string {
		seen := map[string]int{}
		out := make([]string, len(list))
		for i, n := range list {
			out[i] = n + "\x00" + strconv.Itoa(seen[n])
			seen[n]++
		}
		return out
	}
	old := slices.Clone(names)
	sort.SliceStable(old, func(i, j int) bool { return formats.NaturalLess(old[i], old[j]) })
	index := map[string]int{}
	for i, k := range key(names) {
		index[k] = i + 1
	}
	to, moved := make(map[int]int, len(old)), false
	for i, k := range key(old) {
		to[i+1] = index[k]
		moved = moved || index[k] != i+1
	}
	if !moved {
		return nil
	}
	return to
}
