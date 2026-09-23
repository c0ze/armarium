package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/c0ze/armarium/internal/calibre"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
)

// calibreFiles lists a Calibre library from its metadata.db instead of walking
// it. Each book becomes one item: its preferred format that exists on disk
// (EPUB first), with the other formats as extra downloads. Books in a Calibre
// series are grouped by series, the rest by their first author, the way
// Calibre-Web's shelves read.
func (s *Scanner) calibreFiles(ctx context.Context, realRoot string, visit func(found)) error {
	books, err := calibre.Read(ctx, realRoot)
	if err != nil {
		return err
	}
	for _, b := range books {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var primary *found
		var extra []store.ExtraFile
		for _, f := range b.Files {
			abs := filepath.Join(realRoot, filepath.FromSlash(f.Path))
			info, err := os.Stat(abs)
			if err != nil || !info.Mode().IsRegular() || !insideRoot(realRoot, abs) {
				continue
			}
			if primary == nil {
				primary = &found{rel: f.Path, size: info.Size(), mt: info.ModTime().Unix(), abs: abs}
				continue
			}
			extra = append(extra, store.ExtraFile{Format: f.Format, Path: f.Path, Size: info.Size()})
		}
		if primary == nil {
			continue // no file on disk: the book's item (if any) is marked missing
		}
		primary.meta = calibreItem(b, *primary, extra)
		visit(*primary)
	}
	return nil
}

func calibreItem(b calibre.Book, f found, extra []store.ExtraFile) *store.ScannedItem {
	format := strings.TrimPrefix(filepath.Ext(f.rel), ".")
	author := "Unknown author"
	if len(b.Authors) > 0 {
		author = b.Authors[0]
	}
	it := &store.ScannedItem{
		Path: f.rel, Format: format, Size: f.size, MTime: f.mt,
		Title: b.Title, SortTitle: formats.SortKey(b.Sort), Author: strings.Join(b.Authors, ", "),
		HasCover: b.HasCover || format == "epub" || format == "cbz" || format == "cbr",
		Calibre:  true, Tags: b.Tags, Extra: extra,
		SeriesPath: "author/" + author, SeriesName: author,
	}
	if it.Tags == nil {
		it.Tags = []string{}
	}
	if b.Series != "" {
		it.SeriesPath, it.SeriesName = "series/"+b.Series, b.Series
		n := b.SeriesIndex
		it.Number = &n
	}
	it.SeriesSort = formats.SortKey(it.SeriesName)
	return it
}
