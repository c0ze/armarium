package httpapi

import (
	"net/http"

	"github.com/c0ze/armarium/internal/scan"
	"github.com/c0ze/armarium/internal/store"
)

type page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

func (s *Server) listLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if libs == nil {
		libs = []store.Library{}
	}
	writeJSON(w, http.StatusOK, libs)
}

func (s *Server) listSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.fail(w, r, errNotFound)
		return
	}
	if _, err := s.Store.Library(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	off, lim := queryInt(r, "offset", 0, 0, 1<<30), queryInt(r, "limit", 100, 1, 500)
	list, total, err := s.Store.SeriesList(r.Context(), id, r.URL.Query().Get("q"), off, lim)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if list == nil {
		list = []store.Series{}
	}
	writeJSON(w, http.StatusOK, page[store.Series]{list, total, off, lim})
}

// getSeries returns the series and all its items in reading order.
func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.fail(w, r, errNotFound)
		return
	}
	se, err := s.Store.Series(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, _, err := s.Store.Items(r.Context(), store.ItemFilter{SeriesID: id, Limit: 5000})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": se, "items": items})
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ItemFilter{
		LibraryID: queryID(r, "library"), SeriesID: queryID(r, "series"),
		Query: q.Get("q"), Status: q.Get("status"), Tag: q.Get("tag"), Source: q.Get("source"), Sort: q.Get("sort"),
		Offset: queryInt(r, "offset", 0, 0, 1<<30), Limit: queryInt(r, "limit", 60, 1, 500),
	}
	if !store.ValidSort(f.Sort) {
		writeError(w, http.StatusBadRequest, "sort must be one of: title, added, recent")
		return
	}
	switch f.Status {
	case "", "unread", "reading", "read":
	default:
		writeError(w, http.StatusBadRequest, "status must be unread, reading or read")
		return
	}
	items, total, err := s.Store.Items(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page[store.Item]{items, total, f.Offset, f.Limit})
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) (store.Item, bool) {
	id, ok := pathID(r, "id")
	if !ok {
		s.fail(w, r, errNotFound)
		return store.Item{}, false
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return it, false
	}
	return it, true
}

// getItem returns the item with everything a reader needs in one call: progress,
// series and library (PanelFlow needed three Kavita calls for this).
func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": it, "library": lib})
}

type facetOut struct {
	store.Facet
	Label string `json:"label"`
}

func (s *Server) getFacets(w http.ResponseWriter, r *http.Request) {
	lib := queryID(r, "library")
	sources, tags, err := s.Store.Facets(r.Context(), lib)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	labels := s.allLabels()
	out := make([]facetOut, len(sources))
	for i, f := range sources {
		out[i] = facetOut{f, labels.Label(f.Name)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out, "tags": tags})
}

// allLabels merges the label files of every library. Loaded per request: the
// files are tiny and this keeps edits live without a restart.
func (s *Server) allLabels() scan.Labels {
	out := scan.Labels{}
	for _, l := range s.Cfg.Libraries {
		if ls, err := scan.LoadLabels(l.Labels); err == nil {
			for k, v := range ls {
				out[k] = v
			}
		}
	}
	return out
}

func (s *Server) putProgress(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	var body struct {
		Page    *int   `json:"page"`
		Locator string `json:"locator"`
		Status  string `json:"status"`
		Reset   bool   `json:"reset"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Locator) > 1024 {
		writeError(w, http.StatusBadRequest, "body must be {page, locator?, status?, reset?}")
		return
	}
	u := store.ProgressUpdate{Locator: body.Locator, Status: body.Status, Reset: body.Reset}
	switch {
	case body.Status == "unread":
		u.Reset, u.Page = true, 0
	case body.Status == "read" && body.Page == nil:
		u.Page = max(it.Pages, it.Progress.Page)
	case body.Status != "" && body.Status != "reading" && body.Status != "read":
		writeError(w, http.StatusBadRequest, "status must be unread, reading or read")
		return
	case body.Page == nil:
		writeError(w, http.StatusBadRequest, "page is required")
		return
	}
	if body.Page != nil && body.Status != "unread" {
		u.Page = *body.Page
	}
	if u.Page < 0 || (it.Pages > 0 && u.Page > it.Pages) {
		writeError(w, http.StatusBadRequest, "page out of range")
		return
	}
	p, applied, err := s.Store.SaveProgress(r.Context(), it.ID, it.Pages, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": p, "applied": applied})
}
