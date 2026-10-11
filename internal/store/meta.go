package store

import (
	"context"
	"database/sql"
	"errors"
)

// Meta returns a stored value: one-time job markers and fingerprints. A
// missing key is "".
func (s *Store) Meta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetMeta stores a value, replacing any previous one.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value", key, value)
	return err
}
