package store

import (
	"context"
	"database/sql"
)

// ProgressUpdate is one write from a reader. Page is 1-based (the page or chapter
// the reader is on); 0 means not started.
type ProgressUpdate struct {
	Page    int
	Locator string
	Status  string // "" derives it from Page; "unread" only makes sense with Reset
	Reset   bool   // allow moving backwards (mark unread, re-read)
}

// SaveProgress applies u to item and returns the stored progress. Without Reset the
// write is monotonic, enforced in SQL: a lower page than the stored one is ignored
// and a "read" status is never downgraded. applied reports whether the write landed.
func (s *Store) SaveProgress(ctx context.Context, itemID int64, pages int, u ProgressUpdate) (p Progress, applied bool, err error) {
	status := u.Status
	if status == "" {
		switch {
		case pages > 0 && u.Page >= pages:
			status = "read"
		case u.Page > 0:
			status = "reading"
		default:
			status = "unread"
		}
	}
	var res sql.Result
	if u.Reset {
		res, err = s.DB.ExecContext(ctx, `INSERT INTO progress (item_id, page, locator, status, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (item_id) DO UPDATE SET page = excluded.page, locator = excluded.locator,
			  status = excluded.status, updated_at = excluded.updated_at`,
			itemID, u.Page, u.Locator, status, s.now())
	} else {
		res, err = s.DB.ExecContext(ctx, `INSERT INTO progress (item_id, page, locator, status, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (item_id) DO UPDATE SET page = excluded.page, locator = excluded.locator,
			  status = CASE WHEN progress.status = 'read' THEN 'read' ELSE excluded.status END,
			  updated_at = excluded.updated_at
			WHERE excluded.page >= progress.page`,
			itemID, u.Page, u.Locator, status, s.now())
	}
	if err != nil {
		return p, false, err
	}
	n, _ := res.RowsAffected()
	p, err = s.ProgressOf(ctx, itemID)
	return p, n > 0, err
}

func (s *Store) ProgressOf(ctx context.Context, itemID int64) (Progress, error) {
	p := Progress{Status: "unread"}
	err := s.DB.QueryRowContext(ctx, "SELECT page, locator, status, updated_at FROM progress WHERE item_id = ?", itemID).
		Scan(&p.Page, &p.Locator, &p.Status, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		err = nil
	}
	return p, err
}

// ImportProgress writes progress from another app, keeping whichever side was
// updated more recently. Used by the Kavita and Skrivist importers.
func (s *Store) ImportProgress(ctx context.Context, itemID int64, p Progress) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO progress (item_id, page, locator, status, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (item_id) DO UPDATE SET page = excluded.page, locator = excluded.locator,
		  status = excluded.status, updated_at = excluded.updated_at
		WHERE excluded.updated_at > progress.updated_at`,
		itemID, p.Page, p.Locator, p.Status, p.UpdatedAt)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
