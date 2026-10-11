package scan

import (
	"context"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/c0ze/armarium/internal/formats"
)

// RemapPageOrder moves reading positions and panel corrections to the page
// numbers of the root-first order (v0.5.0), once. Comics whose pages were
// numbered differently before keep each reader's place and every correction
// on its own image. Each comic is marked as it is done; the global marker is
// set only when none failed, so an unreadable archive is retried next start.
func (s *Scanner) RemapPageOrder(ctx context.Context) error {
	if v, err := s.Store.Meta(ctx, "page_order"); err != nil || v == "2" {
		return err
	}
	items, err := s.Store.ItemsWithPageState(ctx)
	if err != nil {
		return err
	}
	failed := 0
	for _, it := range items {
		if done, err := s.Store.PageOrderDone(ctx, it.ID); err != nil {
			return err
		} else if done {
			continue
		}
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
	if failed > 0 {
		return nil
	}
	if err := s.Store.SetMeta(ctx, "page_order", "2"); err != nil {
		return err
	}
	_, err = s.Store.DB.ExecContext(ctx, "DELETE FROM meta WHERE key LIKE 'page_order:%'")
	return err
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
