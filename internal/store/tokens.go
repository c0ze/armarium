package store

import (
	"context"
	"database/sql"
	"time"
)

type Token struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt *int64 `json:"lastUsedAt"`
}

// AddToken stores the hash of a new API token. The plaintext never touches the DB.
func (s *Store) AddToken(ctx context.Context, name, hash string) (Token, error) {
	t := Token{Name: name, CreatedAt: s.now()}
	res, err := s.DB.ExecContext(ctx, "INSERT INTO token (name, hash, created_at) VALUES (?, ?, ?)", name, hash, t.CreatedAt)
	if err != nil {
		return t, err
	}
	t.ID, err = res.LastInsertId()
	return t, err
}

func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, name, created_at, last_used_at FROM token ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		var last sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			t.LastUsedAt = &last.Int64
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM token WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TokenValid reports whether hash belongs to a token, and records the use at most
// once a minute so every page request doesn't turn into a write.
func (s *Store) TokenValid(ctx context.Context, hash string) (bool, error) {
	var id int64
	var last sql.NullInt64
	err := s.DB.QueryRowContext(ctx, "SELECT id, last_used_at FROM token WHERE hash = ?", hash).Scan(&id, &last)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if now := s.now(); !last.Valid || now-last.Int64 > 60 {
		s.DB.ExecContext(ctx, "UPDATE token SET last_used_at = ? WHERE id = ?", now, id)
	}
	return true, nil
}

func (s *Store) AddSession(ctx context.Context, hash string, ttl time.Duration) error {
	s.DB.ExecContext(ctx, "DELETE FROM session WHERE expires_at < ?", s.now())
	_, err := s.DB.ExecContext(ctx, "INSERT INTO session (hash, expires_at) VALUES (?, ?)", hash, s.Now().Add(ttl).Unix())
	return err
}

func (s *Store) SessionValid(ctx context.Context, hash string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM session WHERE hash = ? AND expires_at >= ?", hash, s.now()).Scan(&n)
	return n > 0, err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM session WHERE hash = ?", hash)
	return err
}
