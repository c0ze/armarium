package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// The owner password set from the browser or `armarium password`. A
// password_hash in the config wins over it.
const ownerPasswordKey = "owner_password_hash"

// OwnerPassword returns the stored argon2id hash, "" when none is set.
func (s *Store) OwnerPassword(ctx context.Context) (string, error) {
	return s.Meta(ctx, ownerPasswordKey)
}

// SetOwnerPassword stores a new hash and ends every owner session, recording
// its fingerprint so the start-up check does not reset them a second time.
func (s *Store) SetOwnerPassword(ctx context.Context, hash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	set := "INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value"
	if _, err := tx.ExecContext(ctx, set, ownerPasswordKey, hash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM session"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, set, "password", passwordFingerprint(hash)); err != nil {
		return err
	}
	return tx.Commit()
}

func passwordFingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:])
}
