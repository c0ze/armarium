package store

import (
	"context"
	"database/sql"
	"time"
)

// Known is what the scanner needs to decide whether a file changed.
type Known struct {
	ID      int64
	Size    int64
	MTime   int64
	Pages   int
	Missing bool
}

// KnownItems maps library-relative path to the stored state, for change detection.
func (s *Store) KnownItems(ctx context.Context, libraryID int64) (map[string]Known, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, path, size, mtime, pages, missing_at IS NOT NULL FROM item WHERE library_id = ?", libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Known{}
	for rows.Next() {
		var k Known
		var p string
		if err := rows.Scan(&k.ID, &p, &k.Size, &k.MTime, &k.Pages, &k.Missing); err != nil {
			return nil, err
		}
		out[p] = k
	}
	return out, rows.Err()
}

// ScannedItem is one file the scanner opened (new or changed).
type ScannedItem struct {
	SeriesPath, SeriesName, SeriesSort string
	Path, Format, Title, SortTitle     string
	Number                             *float64
	Size, MTime                        int64
	Pages                              int
	Source, Author                     string
	HasCover                           bool
	// Calibre libraries also replace tags and the extra formats on every scan.
	Tags    []string
	Extra   []ExtraFile
	Calibre bool
}

// ExtraFile is another format of the same book (Calibre keeps one per format).
type ExtraFile struct {
	Format, Path string
	Size         int64
}

// Batch is one scanner transaction. The scanner commits every few dozen files so
// API writes are never blocked for long.
type Batch struct {
	s         *Store
	tx        *sql.Tx
	libraryID int64
	series    map[string]int64
}

func (s *Store) Begin(ctx context.Context, libraryID int64) (*Batch, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Batch{s: s, tx: tx, libraryID: libraryID, series: map[string]int64{}}, nil
}

func (b *Batch) Commit() error   { return b.tx.Commit() }
func (b *Batch) Rollback() error { return b.tx.Rollback() }

func (b *Batch) seriesID(ctx context.Context, path, name, sort string) (int64, error) {
	if id, ok := b.series[path]; ok {
		return id, nil
	}
	var id int64
	err := b.tx.QueryRowContext(ctx, `INSERT INTO series (library_id, path, name, sort_name) VALUES (?, ?, ?, ?)
		ON CONFLICT (library_id, path) DO UPDATE SET name = excluded.name, sort_name = excluded.sort_name
		RETURNING id`, b.libraryID, path, name, sort).Scan(&id)
	if err == nil {
		b.series[path] = id
	}
	return id, err
}

// Upsert inserts or refreshes an item; progress rows survive because the ID is kept.
func (b *Batch) Upsert(ctx context.Context, it ScannedItem) error {
	sid, err := b.seriesID(ctx, it.SeriesPath, it.SeriesName, it.SeriesSort)
	if err != nil {
		return err
	}
	var id int64
	err = b.tx.QueryRowContext(ctx, `INSERT INTO item (library_id, series_id, path, format, size, mtime, title,
		  sort_title, number, pages, source, added_at, author, has_cover)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, path) DO UPDATE SET series_id = excluded.series_id, format = excluded.format,
		  size = excluded.size, mtime = excluded.mtime, title = excluded.title, sort_title = excluded.sort_title,
		  number = excluded.number, pages = excluded.pages, source = excluded.source, author = excluded.author,
		  has_cover = excluded.has_cover, missing_at = NULL
		RETURNING id`,
		b.libraryID, sid, it.Path, it.Format, it.Size, it.MTime, it.Title, it.SortTitle, it.Number, it.Pages,
		it.Source, b.s.now(), it.Author, it.HasCover).Scan(&id)
	if err != nil || !it.Calibre {
		return err
	}
	if err := b.SetTags(ctx, id, it.Tags); err != nil {
		return err
	}
	if _, err := b.tx.ExecContext(ctx, "DELETE FROM item_file WHERE item_id = ?", id); err != nil {
		return err
	}
	for _, f := range it.Extra {
		if _, err := b.tx.ExecContext(ctx, "INSERT OR REPLACE INTO item_file (item_id, format, path, size) VALUES (?, ?, ?, ?)",
			id, f.Format, f.Path, f.Size); err != nil {
			return err
		}
	}
	return nil
}

// SetTags replaces an item's tags.
func (b *Batch) SetTags(ctx context.Context, itemID int64, tags []string) error {
	if _, err := b.tx.ExecContext(ctx, "DELETE FROM item_tag WHERE item_id = ?", itemID); err != nil {
		return err
	}
	for _, t := range tags {
		var tid int64
		if err := b.tx.QueryRowContext(ctx, `INSERT INTO tag (name) VALUES (?)
			ON CONFLICT (name) DO UPDATE SET name = excluded.name RETURNING id`, t).Scan(&tid); err != nil {
			return err
		}
		if _, err := b.tx.ExecContext(ctx, "INSERT OR IGNORE INTO item_tag (item_id, tag_id) VALUES (?, ?)", itemID, tid); err != nil {
			return err
		}
	}
	return nil
}

// Seen clears missing_at on an unchanged item that had gone missing.
func (b *Batch) Seen(ctx context.Context, id int64) error {
	_, err := b.tx.ExecContext(ctx, "UPDATE item SET missing_at = NULL WHERE id = ?", id)
	return err
}

// MarkMissing flags items that the scan did not find. Rows are kept (with progress)
// so a temporary NFS hiccup loses nothing.
func (b *Batch) MarkMissing(ctx context.Context, id int64) error {
	_, err := b.tx.ExecContext(ctx, "UPDATE item SET missing_at = ? WHERE id = ? AND missing_at IS NULL", b.s.now(), id)
	return err
}

// Prune deletes items missing for longer than keep, then empty series and unused tags.
func (s *Store) Prune(ctx context.Context, keep time.Duration) (int64, error) {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM item WHERE missing_at IS NOT NULL AND missing_at < ?", s.Now().Add(-keep).Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM series WHERE NOT EXISTS (SELECT 1 FROM item WHERE item.series_id = series.id)"); err != nil {
		return n, err
	}
	_, err = s.DB.ExecContext(ctx, "DELETE FROM tag WHERE NOT EXISTS (SELECT 1 FROM item_tag WHERE item_tag.tag_id = tag.id)")
	return n, err
}
