package store

import (
	"context"
	"database/sql"
	"strings"
)

type Progress struct {
	Page      int    `json:"page"`
	Locator   string `json:"locator"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"`
}

type Item struct {
	ID         int64    `json:"id"`
	LibraryID  int64    `json:"libraryId"`
	SeriesID   int64    `json:"seriesId"`
	SeriesName string   `json:"seriesName"`
	Path       string   `json:"-"`
	Format     string   `json:"format"`
	Size       int64    `json:"size"`
	MTime      int64    `json:"-"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	Number     *float64 `json:"number"`
	Pages      int      `json:"pages"`
	Source     string   `json:"source"`
	AddedAt    int64    `json:"addedAt"`
	Tags       []string `json:"tags"`
	HasCover   bool     `json:"hasCover"`
	// Formats lists extra formats of the same book (Calibre libraries), each
	// downloadable with /file?format=; the item's own Format is not repeated.
	Formats  []string `json:"formats"`
	Progress Progress `json:"progress"`
}

type ItemFilter struct {
	LibraryID, SeriesID int64
	Query, Status, Tag  string
	Source, Sort        string
	Offset, Limit       int
}

const itemCols = `i.id, i.library_id, i.series_id, s.name, i.path, i.format, i.size, i.mtime, i.title,
	i.number, i.pages, i.source, i.added_at, i.author, i.has_cover,
	COALESCE((SELECT group_concat(f.format, ',') FROM item_file f WHERE f.item_id = i.id), ''),
	COALESCE(p.page, 0), COALESCE(p.locator, ''),
	COALESCE(p.status, 'unread'), COALESCE(p.updated_at, 0),
	COALESCE((SELECT group_concat(t.name, char(31)) FROM item_tag it JOIN tag t ON t.id = it.tag_id
	  WHERE it.item_id = i.id), '')`

const itemFrom = ` FROM item i JOIN series s ON s.id = i.series_id LEFT JOIN progress p ON p.item_id = i.id`

func scanItem(sc interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var num sql.NullFloat64
	var tags, formats string
	err := sc.Scan(&it.ID, &it.LibraryID, &it.SeriesID, &it.SeriesName, &it.Path, &it.Format, &it.Size, &it.MTime,
		&it.Title, &num, &it.Pages, &it.Source, &it.AddedAt, &it.Author, &it.HasCover, &formats,
		&it.Progress.Page, &it.Progress.Locator, &it.Progress.Status, &it.Progress.UpdatedAt, &tags)
	if num.Valid {
		it.Number = &num.Float64
	}
	it.Tags = []string{}
	if tags != "" {
		it.Tags = strings.Split(tags, "\x1f")
	}
	it.Formats = []string{}
	if formats != "" {
		it.Formats = strings.Split(formats, ",")
	}
	return it, err
}

var itemSorts = map[string]string{
	"":       "s.sort_name, i.number IS NULL, i.number, i.sort_title",
	"title":  "i.sort_title",
	"added":  "i.added_at DESC, i.id DESC",
	"recent": "COALESCE(p.updated_at, 0) DESC, i.id DESC",
}

// ValidSort reports whether sort is an accepted ItemFilter.Sort value.
func ValidSort(sort string) bool { _, ok := itemSorts[sort]; return ok }

// Items lists present (not missing) items matching f, plus the total match count.
func (s *Store) Items(ctx context.Context, f ItemFilter) ([]Item, int, error) {
	where := []string{"i.missing_at IS NULL"}
	var args []any
	add := func(cond string, v any) { where = append(where, cond); args = append(args, v) }
	if f.LibraryID != 0 {
		add("i.library_id = ?", f.LibraryID)
	}
	if f.SeriesID != 0 {
		add("i.series_id = ?", f.SeriesID)
	}
	if f.Query != "" {
		where = append(where, "(i.title LIKE ? ESCAPE '\\' OR s.name LIKE ? ESCAPE '\\' OR i.author LIKE ? ESCAPE '\\')")
		args = append(args, likePattern(f.Query), likePattern(f.Query), likePattern(f.Query))
	}
	if f.Status != "" {
		add("COALESCE(p.status, 'unread') = ?", f.Status)
	}
	if f.Source != "" {
		add("i.source = ?", f.Source)
	}
	if f.Tag != "" {
		add("EXISTS (SELECT 1 FROM item_tag it JOIN tag t ON t.id = it.tag_id WHERE it.item_id = i.id AND t.name = ?)", f.Tag)
	}
	w := " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+itemFrom+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order, ok := itemSorts[f.Sort]
	if !ok {
		order = itemSorts[""]
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+itemCols+itemFrom+w+" ORDER BY "+order+" LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// Item returns one present item by ID.
func (s *Store) Item(ctx context.Context, id int64) (Item, error) {
	it, err := scanItem(s.DB.QueryRowContext(ctx, "SELECT "+itemCols+itemFrom+" WHERE i.id = ? AND i.missing_at IS NULL", id))
	return it, notFound(err)
}

// Facet is a filter value with the number of present items carrying it.
type Facet struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Facets returns the sources and tags in use, for the UI filters.
func (s *Store) Facets(ctx context.Context, libraryID int64) (sources, tags []Facet, err error) {
	lib := ""
	args := []any{}
	if libraryID != 0 {
		lib = " AND i.library_id = ?"
		args = append(args, libraryID)
	}
	sources, err = s.facetQuery(ctx, `SELECT i.source, COUNT(*) FROM item i WHERE i.missing_at IS NULL AND i.source != ''`+lib+
		` GROUP BY i.source ORDER BY i.source`, args)
	if err != nil {
		return nil, nil, err
	}
	tags, err = s.facetQuery(ctx, `SELECT t.name, COUNT(*) FROM tag t JOIN item_tag it ON it.tag_id = t.id
		JOIN item i ON i.id = it.item_id WHERE i.missing_at IS NULL`+lib+` GROUP BY t.name ORDER BY t.name`, args)
	return sources, tags, err
}

func (s *Store) facetQuery(ctx context.Context, q string, args []any) ([]Facet, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Facet{}
	for rows.Next() {
		var f Facet
		if err := rows.Scan(&f.Name, &f.Count); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ItemFile returns the library-relative path of another format of an item.
func (s *Store) ItemFile(ctx context.Context, itemID int64, format string) (string, error) {
	var p string
	err := s.DB.QueryRowContext(ctx, "SELECT path FROM item_file WHERE item_id = ? AND format = ?", itemID, format).Scan(&p)
	return p, notFound(err)
}
