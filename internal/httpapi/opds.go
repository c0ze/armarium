package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/opds"
	"github.com/c0ze/armarium/internal/store"
)

const opdsPerPage = 50

// opdsRoutes mounts the catalog under prefix. Every link a feed emits stays under
// the same prefix, so clients that carry the token in the path (/opds/t/{token})
// never lose it, and Basic-auth clients keep using /opds/v1.2.
func (s *Server) opdsRoutes(prefix string, route func(string, Access, http.HandlerFunc)) {
	route("GET "+strings.TrimSuffix(prefix, "/v1.2"), Reader, s.opdsCatalog)
	route("GET "+prefix, Reader, s.opdsCatalog)
	route("GET "+prefix+"/catalog", Reader, s.opdsCatalog)
	route("GET "+prefix+"/libraries/{id}", Reader, s.opdsLibrary)
	route("GET "+prefix+"/series/{id}", Reader, s.opdsSeries)
	route("GET "+prefix+"/recent", Reader, s.opdsItemFeed("recent", "Recently added", store.ItemFilter{Sort: "added"}))
	route("GET "+prefix+"/reading", Reader, s.opdsItemFeed("reading", "Continue reading", store.ItemFilter{Status: "reading", Sort: "recent"}))
	route("GET "+prefix+"/search", Reader, s.opdsSearch)
	route("GET "+prefix+"/opensearch.xml", Reader, s.opdsOpenSearch)
	route("GET "+prefix+"/items/{id}/file", Reader, s.getFile)
	route("GET "+prefix+"/items/{id}/cover", Reader, s.getCover)
	route("GET "+prefix+"/items/{id}/pse/{n}", Reader, s.opdsPage)
}

// opdsBase is the link prefix for this request.
func opdsBase(r *http.Request) string {
	if t := r.PathValue("token"); t != "" {
		return "/opds/t/" + url.PathEscape(t) + "/v1.2"
	}
	return "/opds/v1.2"
}

func writeFeed(w http.ResponseWriter, f opds.Feed, kind string) {
	w.Header().Set("Content-Type", kind+"; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	f.WriteTo(w)
}

// commonLinks are the feed-level links every feed carries. The Atom search
// template comes first: skrivist.app uses the first rel="search" link as-is.
func commonLinks(base, self, selfType string) []opds.Link {
	return []opds.Link{
		{Rel: "self", Href: self, Type: selfType},
		{Rel: "start", Href: base + "/catalog", Type: opds.NavType},
		{Rel: "search", Href: base + "/search?q={searchTerms}", Type: "application/atom+xml", Title: "Search"},
		{Rel: "search", Href: base + "/opensearch.xml", Type: "application/opensearchdescription+xml"},
	}
}

func (s *Server) opdsCatalog(w http.ResponseWriter, r *http.Request) {
	base := opdsBase(r)
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now()
	f := opds.Feed{ID: "urn:armarium:catalog", Title: "Armarium", Updated: now, Links: commonLinks(base, base+"/catalog", opds.NavType)}
	nav := func(id, title, href, typ, content string) {
		f.Entries = append(f.Entries, opds.Entry{ID: id, Title: title, Updated: now, Content: content,
			Links: []opds.Link{{Rel: opds.RelSub, Href: href, Type: typ}}})
	}
	nav("urn:armarium:reading", "Continue reading", base+"/reading", opds.AcqType, "Items in progress")
	nav("urn:armarium:recent", "Recently added", base+"/recent", opds.AcqType, "Newest items first")
	for _, l := range libs {
		nav(fmt.Sprintf("urn:armarium:library:%d", l.ID), l.Name, fmt.Sprintf("%s/libraries/%d", base, l.ID), opds.NavType, l.Kind)
	}
	writeFeed(w, f, opds.NavType)
}

// paging adds next/previous links and the OpenSearch counters.
func paging(f *opds.Feed, self string, off, total int, typ string) {
	f.Total, f.Offset, f.PerPage = total, off, opdsPerPage
	sep := "?"
	if strings.Contains(self, "?") {
		sep = "&"
	}
	if off+opdsPerPage < total {
		f.Links = append(f.Links, opds.Link{Rel: "next", Href: self + sep + "offset=" + strconv.Itoa(off+opdsPerPage), Type: typ})
	}
	if off > 0 {
		f.Links = append(f.Links, opds.Link{Rel: "previous", Href: self + sep + "offset=" + strconv.Itoa(max(off-opdsPerPage, 0)), Type: typ})
	}
}

func (s *Server) opdsLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		s.fail(w, r, errNotFound)
		return
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	base, off := opdsBase(r), queryInt(r, "offset", 0, 0, 1<<30)
	list, total, err := s.Store.SeriesList(r.Context(), id, "", off, opdsPerPage)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	self := fmt.Sprintf("%s/libraries/%d", base, id)
	now := time.Now()
	f := opds.Feed{ID: fmt.Sprintf("urn:armarium:library:%d", id), Title: lib.Name, Updated: now, Links: commonLinks(base, self, opds.NavType)}
	f.Links = append(f.Links, opds.Link{Rel: "up", Href: base + "/catalog", Type: opds.NavType})
	paging(&f, self, off, total, opds.NavType)
	for _, se := range list {
		f.Entries = append(f.Entries, opds.Entry{
			ID: fmt.Sprintf("urn:armarium:series:%d", se.ID), Title: se.Name, Updated: now,
			Content: fmt.Sprintf("%d items, %d unread", se.Items, se.Unread),
			Links:   []opds.Link{{Rel: opds.RelSub, Href: fmt.Sprintf("%s/series/%d", base, se.ID), Type: opds.AcqType}},
		})
	}
	writeFeed(w, f, opds.NavType)
}

func (s *Server) opdsSeries(w http.ResponseWriter, r *http.Request) {
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
	base := opdsBase(r)
	s.acquisition(w, r, fmt.Sprintf("urn:armarium:series:%d", id), se.Name, fmt.Sprintf("%s/series/%d", base, id),
		store.ItemFilter{SeriesID: id},
		opds.Link{Rel: "up", Href: fmt.Sprintf("%s/libraries/%d", base, se.LibraryID), Type: opds.NavType})
}

func (s *Server) opdsItemFeed(key, title string, f store.ItemFilter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := opdsBase(r)
		s.acquisition(w, r, "urn:armarium:"+key, title, base+"/"+key, f, opds.Link{Rel: "up", Href: base + "/catalog", Type: opds.NavType})
	}
}

func (s *Server) opdsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	base := opdsBase(r)
	s.acquisition(w, r, "urn:armarium:search", "Search: "+q, base+"/search?q="+url.QueryEscape(q),
		store.ItemFilter{Query: q, Sort: "title"}, opds.Link{Rel: "up", Href: base + "/catalog", Type: opds.NavType})
}

func (s *Server) opdsOpenSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/opensearchdescription+xml; charset=utf-8")
	w.Write(opds.OpenSearch(opdsBase(r) + "/search?q={searchTerms}"))
}

func (s *Server) acquisition(w http.ResponseWriter, r *http.Request, id, title, self string, filter store.ItemFilter, up opds.Link) {
	base, off := opdsBase(r), queryInt(r, "offset", 0, 0, 1<<30)
	filter.Offset, filter.Limit = off, opdsPerPage
	items, total, err := s.Store.Items(r.Context(), filter)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := opds.Feed{ID: id, Title: title, Updated: time.Now(), Links: append(commonLinks(base, self, opds.AcqType), up)}
	paging(&f, self, off, total, opds.AcqType)
	for _, it := range items {
		f.Entries = append(f.Entries, itemEntry(base, it))
	}
	writeFeed(w, f, opds.AcqType)
}

func itemEntry(base string, it store.Item) opds.Entry {
	href := fmt.Sprintf("%s/items/%d", base, it.ID)
	updated := time.Unix(max(it.AddedAt, it.Progress.UpdatedAt), 0)
	e := opds.Entry{
		ID: fmt.Sprintf("urn:armarium:item:%d", it.ID), Title: it.Title, Author: it.Author, Updated: updated,
		Content: entrySummary(it),
		Links:   []opds.Link{{Rel: opds.RelAcq, Href: href + "/file", Type: formats.MediaTypeOf(it.Format)}},
	}
	// Other formats of a Calibre book: one acquisition link each (KOReader, Kindle).
	for _, f := range it.Formats {
		e.Links = append(e.Links, opds.Link{Rel: opds.RelAcq, Href: href + "/file?format=" + url.QueryEscape(f), Type: formats.MediaTypeOf(f)})
	}
	if it.HasCover {
		e.Links = append(e.Links,
			opds.Link{Rel: opds.RelImage, Href: href + "/cover", Type: "image/jpeg"},
			opds.Link{Rel: opds.RelThumb, Href: href + "/cover", Type: "image/jpeg"})
	}
	if (it.Format == "cbz" || it.Format == "cbr") && it.Pages > 0 {
		pse := opds.Link{Rel: opds.RelPSE, Href: href + "/pse/{pageNumber}", Type: "image/jpeg", PSECount: it.Pages, PSELastRead: -1}
		if it.Progress.Page > 0 {
			pse.PSELastRead = min(it.Progress.Page, it.Pages) - 1
			pse.PSELastReadDate = time.Unix(it.Progress.UpdatedAt, 0)
		}
		e.Links = append(e.Links, pse)
	}
	return e
}

// opdsPage is the PSE stream endpoint: {pageNumber} is 0-based per the spec.
func (s *Server) opdsPage(w http.ResponseWriter, r *http.Request) {
	it, ok := s.item(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 {
		s.fail(w, r, errNotFound)
		return
	}
	s.servePage(w, r, it, n)
}

// entrySummary is the one-line OPDS content: series, format, length, size. EPUB
// lengths are chapters, and an unknown length is left out.
func entrySummary(it store.Item) string {
	parts := []string{it.SeriesName, strings.ToUpper(it.Format)}
	unit := "pages"
	if it.Format == "epub" {
		unit = "chapters"
	}
	if it.Pages > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", it.Pages, unit))
	}
	parts = append(parts, fmt.Sprintf("%.1f MB", float64(it.Size)/(1<<20)))
	return strings.Join(parts, " · ")
}
