package store

import (
	"context"
	"database/sql"
)

// KOSyncRecord is a KOReader sync document: the item it names (0 if unknown)
// and the last position a device uploaded for it (UpdatedAt 0: none yet).
type KOSyncRecord struct {
	Document         string
	ItemID           int64
	Progress         string
	Percentage       float64
	Device, DeviceID string
	UpdatedAt        int64
}

func (s *Store) KOSyncDocument(ctx context.Context, doc string) (KOSyncRecord, error) {
	r := KOSyncRecord{Document: doc}
	var item sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT item_id, progress, percentage, device, device_id, updated_at
		FROM kosync_document WHERE document = ?`, doc).
		Scan(&item, &r.Progress, &r.Percentage, &r.Device, &r.DeviceID, &r.UpdatedAt)
	r.ItemID = item.Int64
	return r, notFound(err)
}

// LinkKOSyncDocument records that doc names itemID, keeping any uploaded position.
func (s *Store) LinkKOSyncDocument(ctx context.Context, doc string, itemID int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kosync_document (document, item_id) VALUES (?, ?)
		ON CONFLICT (document) DO UPDATE SET item_id = excluded.item_id`, doc, itemID)
	return err
}

// SaveKOSync stores an uploaded position. r.ItemID, when set, also links the
// document; 0 keeps the current link.
func (s *Store) SaveKOSync(ctx context.Context, r KOSyncRecord) error {
	var item any
	if r.ItemID != 0 {
		item = r.ItemID
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kosync_document
		  (document, item_id, progress, percentage, device, device_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (document) DO UPDATE SET item_id = COALESCE(excluded.item_id, item_id),
		  progress = excluded.progress, percentage = excluded.percentage, device = excluded.device,
		  device_id = excluded.device_id, updated_at = excluded.updated_at`,
		r.Document, item, r.Progress, r.Percentage, r.Device, r.DeviceID, r.UpdatedAt)
	return err
}

// EPUBFile is the EPUB of a book: the item's own file or a Calibre extra format.
type EPUBFile struct {
	ItemID, LibraryID int64
	Path, Author      string
}

// EPUBCandidates lists present items with an EPUB whose title folds to the
// same text as title, at most limit of them.
func (s *Store) EPUBCandidates(ctx context.Context, title string, limit int) ([]EPUBFile, error) {
	if title == "" {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, i.library_id,
		  CASE WHEN i.format = 'epub' THEN i.path ELSE f.path END, i.author
		FROM item i LEFT JOIN item_file f ON f.item_id = i.id AND f.format = 'epub'
		WHERE i.missing_at IS NULL AND (i.format = 'epub' OR f.path IS NOT NULL) AND fold(i.title) = fold(?)
		ORDER BY i.id LIMIT ?`, title, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EPUBFile
	for rows.Next() {
		var f EPUBFile
		if err := rows.Scan(&f.ItemID, &f.LibraryID, &f.Path, &f.Author); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
