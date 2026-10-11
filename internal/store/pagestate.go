package store

import (
	"context"
	"database/sql"
	"errors"
)

// ItemsWithOldPageNumbers lists the comics that still hold a reading position
// or panel corrections in the page numbering before v0.5.0 (page_order 1).
// Missing files are left out until a scan finds them again.
func (s *Store) ItemsWithOldPageNumbers(ctx context.Context) ([]Item, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+itemCols+itemFrom+` WHERE i.format IN ('cbz', 'cbr') AND i.missing_at IS NULL
		AND (COALESCE(p.page_order, ?) < ? OR EXISTS (SELECT 1 FROM panel_correction c WHERE c.item_id = i.id AND c.page_order < ?))
		ORDER BY i.id`, PageOrder, PageOrder, PageOrder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// staging lifts an item's old corrections clear of every real page while they
// are renumbered, so no step collides with a row not yet moved.
const staging = 1 << 40

// RemapPages moves an item's rows still in the old numbering to the current one
// (to: old page → new page; nil when nothing moves), in one transaction. Rows
// written since the upgrade stay where they are: an old correction whose new
// page already holds a newer one is dropped. A finished ("read") position
// stays at the end of the book, and a correction beyond the archive keeps its
// number. updated_at stays: clients learn of the renumbering from Item.PageOrder.
func (s *Store) RemapPages(ctx context.Context, itemID int64, to map[int]int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The first statement writes, so the transaction holds the write lock before
	// it reads anything: no API write can land between a read and its update.
	if _, err := tx.ExecContext(ctx, "UPDATE panel_correction SET page = page + ? WHERE item_id = ? AND page_order < ?", staging, itemID, PageOrder); err != nil {
		return err
	}
	var page int
	var status string
	err = tx.QueryRowContext(ctx, "SELECT page, status FROM progress WHERE item_id = ? AND page_order < ?", itemID, PageOrder).Scan(&page, &status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		if status != "read" && page > 0 && to[page] != 0 {
			page = to[page]
		}
		if _, err := tx.ExecContext(ctx, "UPDATE progress SET page = ?, page_order = ? WHERE item_id = ?", page, PageOrder, itemID); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT page FROM panel_correction WHERE item_id = ? AND page_order < ?", itemID, PageOrder)
	if err != nil {
		return err
	}
	var staged []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		staged = append(staged, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range staged {
		page := p - staging
		if n := to[page]; n != 0 {
			page = n
		}
		res, err := tx.ExecContext(ctx, "UPDATE OR IGNORE panel_correction SET page = ?, page_order = ? WHERE item_id = ? AND page = ?", page, PageOrder, itemID, p)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 { // a correction written since the upgrade holds that page
			if _, err := tx.ExecContext(ctx, "DELETE FROM panel_correction WHERE item_id = ? AND page = ?", itemID, p); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
