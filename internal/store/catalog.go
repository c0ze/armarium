package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

type Library struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Root string `json:"-"`
	Kind string `json:"kind"`
}

type Series struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"libraryId"`
	Path      string `json:"-"`
	Name      string `json:"name"`
	Items     int    `json:"items"`
	Unread    int    `json:"unread"`
	// CoverItemID is the first item in reading order, whose cover stands in for the series.
	CoverItemID int64 `json:"coverItemId"`
}

// UnconfiguredError lists libraries the database holds but the config no
// longer names.
type UnconfiguredError struct{ Names []string }

func (e *UnconfiguredError) Error() string {
	quoted := make([]string, len(e.Names))
	for i, n := range e.Names {
		quoted[i] = strconv.Quote(n)
	}
	return "library " + strings.Join(quoted, ", ") + " is in the database but not in the config"
}

// SyncLibraries makes every configured library exist. It never deletes one: a
// library missing from the config (a rename, a typo, one commented out) keeps
// its items, progress and panel corrections, and SyncLibraries reports it and
// writes nothing. RenameLibrary and RemoveLibrary are the explicit ways out.
func (s *Store) SyncLibraries(ctx context.Context, libs []Library) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	names := make([]any, 0, len(libs))
	for _, l := range libs {
		names = append(names, l.Name)
		if _, err := tx.ExecContext(ctx, `INSERT INTO library (name, root, kind) VALUES (?, ?, ?)
			ON CONFLICT (name) DO UPDATE SET root = excluded.root, kind = excluded.kind`,
			l.Name, l.Root, l.Kind); err != nil {
			return err
		}
	}
	q := "SELECT name FROM library"
	if len(names) > 0 {
		q += " WHERE name NOT IN (?" + strings.Repeat(",?", len(names)-1) + ")"
	}
	rows, err := tx.QueryContext(ctx, q+" ORDER BY name", names...)
	if err != nil {
		return err
	}
	var unconfigured []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		unconfigured = append(unconfigured, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(unconfigured) > 0 {
		return &UnconfiguredError{Names: unconfigured}
	}
	return tx.Commit()
}

// RenameLibrary gives a library a new name and keeps everything in it.
func (s *Store) RenameLibrary(ctx context.Context, from, to string) error {
	res, err := s.DB.ExecContext(ctx, "UPDATE library SET name = ? WHERE name = ?", to, from)
	if err != nil {
		return err // including a UNIQUE violation when `to` exists
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveLibrary deletes a library with its items, progress and panel
// corrections, and reports how many items went.
func (s *Store) RemoveLibrary(ctx context.Context, name string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM library WHERE name = ?", name).Scan(&id); err != nil {
		return 0, notFound(err)
	}
	var items int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM item WHERE library_id = ?", id).Scan(&items); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM library WHERE id = ?", id); err != nil {
		return 0, err
	}
	return items, tx.Commit()
}

func (s *Store) Libraries(ctx context.Context) ([]Library, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, name, root, kind FROM library ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Library
	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Name, &l.Root, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) Library(ctx context.Context, id int64) (Library, error) {
	var l Library
	err := s.DB.QueryRowContext(ctx, "SELECT id, name, root, kind FROM library WHERE id = ?", id).
		Scan(&l.ID, &l.Name, &l.Root, &l.Kind)
	return l, notFound(err)
}

const seriesCols = `s.id, s.library_id, s.path, s.name,
	(SELECT COUNT(*) FROM item i WHERE i.series_id = s.id AND i.missing_at IS NULL),
	(SELECT COUNT(*) FROM item i LEFT JOIN progress p ON p.item_id = i.id
	  WHERE i.series_id = s.id AND i.missing_at IS NULL AND COALESCE(p.status, 'unread') != 'read'),
	COALESCE((SELECT i.id FROM item i WHERE i.series_id = s.id AND i.missing_at IS NULL
	  ORDER BY i.number IS NULL, i.number, i.sort_title LIMIT 1), 0)`

func scanSeries(sc interface{ Scan(...any) error }) (Series, error) {
	var x Series
	err := sc.Scan(&x.ID, &x.LibraryID, &x.Path, &x.Name, &x.Items, &x.Unread, &x.CoverItemID)
	return x, err
}

// SeriesList returns the non-empty series of a library, optionally filtered by name.
func (s *Store) SeriesList(ctx context.Context, libraryID int64, q string, offset, limit int) ([]Series, int, error) {
	where := `s.library_id = ? AND EXISTS (SELECT 1 FROM item i WHERE i.series_id = s.id AND i.missing_at IS NULL)`
	args := []any{libraryID}
	if q != "" {
		where += " AND s.search LIKE ? ESCAPE '\\'"
		args = append(args, likePattern(q))
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM series s WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+seriesCols+" FROM series s WHERE "+where+
		" ORDER BY s.sort_name LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Series
	for rows.Next() {
		x, err := scanSeries(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

func (s *Store) Series(ctx context.Context, id int64) (Series, error) {
	x, err := scanSeries(s.DB.QueryRowContext(ctx, "SELECT "+seriesCols+" FROM series s WHERE s.id = ?", id))
	return x, notFound(err)
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(fold(q)) + "%"
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
