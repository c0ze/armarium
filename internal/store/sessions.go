package store

import "context"

// ResetSessionsIfPasswordChanged ends every owner session when password_hash
// differs from the one seen last time, so a reset password logs everyone out.
// The first run only records it. It reports whether sessions were cleared.
func (s *Store) ResetSessionsIfPasswordChanged(ctx context.Context, passwordHash string) (bool, error) {
	fingerprint := passwordFingerprint(passwordHash)
	seen, err := s.Meta(ctx, "password")
	if err != nil || seen == fingerprint {
		return false, err
	}
	if seen != "" {
		if _, err := s.DB.ExecContext(ctx, "DELETE FROM session"); err != nil {
			return false, err
		}
	}
	return seen != "", s.SetMeta(ctx, "password", fingerprint)
}
