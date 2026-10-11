package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

// ItemsWithPageState lists the comics whose stored page numbers matter: a
// reading position or panel corrections. Missing files are left out.
func (s *Store) ItemsWithPageState(ctx context.Context) ([]Item, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+itemCols+itemFrom+` WHERE i.format IN ('cbz', 'cbr') AND i.missing_at IS NULL
		AND (COALESCE(p.page, 0) > 0 OR EXISTS (SELECT 1 FROM panel_correction c WHERE c.item_id = i.id))
		ORDER BY i.id`)
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

// PageOrderDone reports whether RemapPages already ran for an item.
func (s *Store) PageOrderDone(ctx context.Context, itemID int64) (bool, error) {
	v, err := s.Meta(ctx, pageOrderKey(itemID))
	return v != "", err
}

func pageOrderKey(itemID int64) string { return "page_order:" + strconv.FormatInt(itemID, 10) }

// RemapPages moves an item's reading position and panel corrections from old
// page numbers to new ones (to: old → new; nil when nothing moves) and marks
// the item done, all in one transaction: a crash never maps an item twice.
// A finished ("read") position stays at the end of the book.
func (s *Store) RemapPages(ctx context.Context, itemID int64, to map[int]int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if to != nil {
		var page int
		var status string
		err := tx.QueryRowContext(ctx, "SELECT page, status FROM progress WHERE item_id = ?", itemID).Scan(&page, &status)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		case status != "read" && page > 0 && to[page] != 0 && to[page] != page:
			// updated_at stays: clients learn of the renumbering from Item.PageOrder.
			if _, err := tx.ExecContext(ctx, "UPDATE progress SET page = ? WHERE item_id = ?", to[page], itemID); err != nil {
				return err
			}
		}
		if err := remapCorrections(ctx, tx, itemID, to); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO meta (key, value) VALUES (?, '2') ON CONFLICT (key) DO NOTHING", pageOrderKey(itemID)); err != nil {
		return err
	}
	return tx.Commit()
}

// remapCorrections renumbers through a staging range above every page the item
// has, so no step can collide with a row not yet moved. A row whose page is not
// in the mapping (beyond the archive) goes back where it was, or is dropped if
// a moved row now holds that page.
func remapCorrections(ctx context.Context, tx *sql.Tx, itemID int64, to map[int]int) error {
	var top int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(page), 0) FROM panel_correction WHERE item_id = ?", itemID).Scan(&top); err != nil {
		return err
	}
	off := max(top, len(to)) + 1
	if _, err := tx.ExecContext(ctx, "UPDATE panel_correction SET page = page + ? WHERE item_id = ?", off, itemID); err != nil {
		return err
	}
	for old, page := range to {
		if _, err := tx.ExecContext(ctx, "UPDATE panel_correction SET page = ? WHERE item_id = ? AND page = ?", page, itemID, old+off); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT page FROM panel_correction WHERE item_id = ? AND page >= ?", itemID, off)
	if err != nil {
		return err
	}
	var left []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		left = append(left, p)
	}
	rows.Close()
	for _, p := range left {
		var taken int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM panel_correction WHERE item_id = ? AND page = ?", itemID, p-off).Scan(&taken); err != nil {
			return err
		}
		if taken > 0 {
			_, err = tx.ExecContext(ctx, "DELETE FROM panel_correction WHERE item_id = ? AND page = ?", itemID, p)
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE panel_correction SET page = ? WHERE item_id = ? AND page = ?", p-off, itemID, p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
