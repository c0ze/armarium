package httpapi

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c0ze/armarium/internal/store"
	tu "github.com/c0ze/armarium/internal/testutil"
)

func TestBrowseLibrariesSeriesAndItem(t *testing.T) {
	e := newEnv(t)
	libs := decode[[]store.Library](t, e.do(req{path: "/api/libraries", token: e.token}))
	var comics store.Library
	for _, l := range libs {
		if l.Name == "Comics" {
			comics = l
		}
	}
	if len(libs) != 3 || comics.ID == 0 {
		t.Fatalf("libraries %+v", libs)
	}
	series := decode[page[store.Series]](t, e.do(req{path: fmt.Sprintf("/api/libraries/%d/series", comics.ID), token: e.token}))
	if series.Total != 1 || series.Items[0].Name != "Saga" || series.Items[0].Items != 2 {
		t.Fatalf("series %+v", series)
	}
	detail := decode[struct {
		Items []store.Item `json:"items"`
	}](t, e.do(req{path: fmt.Sprintf("/api/series/%d", series.Items[0].ID), token: e.token}))
	if len(detail.Items) != 2 || detail.Items[0].Title != "Saga #01" || detail.Items[1].Format != "cbr" {
		t.Fatalf("series detail %+v", detail.Items)
	}
	one := decode[struct {
		Item    store.Item    `json:"item"`
		Library store.Library `json:"library"`
	}](t, e.do(req{path: fmt.Sprintf("/api/items/%d", e.comicID), token: e.token}))
	if one.Item.Pages != 3 || one.Library.Kind != "comics" || one.Item.SeriesID == 0 {
		t.Fatalf("item %+v", one)
	}
	list := decode[page[store.Item]](t, e.do(req{path: "/api/items?q=book&sort=title", token: e.token}))
	if list.Total != 1 || list.Items[0].Source != "oreilly" {
		t.Fatalf("search %+v", list)
	}
	if w := e.do(req{path: "/api/items?sort=evil", token: e.token}); w.Code != 400 {
		t.Fatalf("bad sort: %d", w.Code)
	}
}

func TestPagesAreOneBasedAndPSEZeroBased(t *testing.T) {
	e := newEnv(t)
	for _, id := range []int64{e.comicID, e.items["Saga #02"].ID} { // cbz and cbr
		w := e.do(req{path: fmt.Sprintf("/api/items/%d/pages/2", id), token: e.token})
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), tu.PNG(8, 12, 20)) {
			t.Fatalf("item %d page 2: %d %s", id, w.Code, w.Header().Get("Content-Type"))
		}
		pse := e.do(req{path: fmt.Sprintf("/opds/v1.2/items/%d/pse/1", id), basic: e.token})
		if !bytes.Equal(pse.Body.Bytes(), w.Body.Bytes()) {
			t.Fatal("PSE page 1 (0-based) must equal API page 2 (1-based)")
		}
		etag := w.Header().Get("ETag")
		if w := e.do(req{path: fmt.Sprintf("/api/items/%d/pages/2", id), token: e.token, headers: map[string]string{"If-None-Match": etag}}); w.Code != 304 {
			t.Fatalf("etag revalidation: %d", w.Code)
		}
	}
}

func TestCoverFileAndRange(t *testing.T) {
	e := newEnv(t)
	for _, id := range []int64{e.comicID, e.bookID} {
		w := e.do(req{path: fmt.Sprintf("/api/items/%d/cover", id), token: e.token})
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || w.Body.Len() == 0 {
			t.Fatalf("cover %d: %d", id, w.Code)
		}
	}
	w := e.do(req{path: fmt.Sprintf("/api/items/%d/file", e.bookID), token: e.token, headers: map[string]string{"Range": "bytes=0-3"}})
	if w.Code != 206 || w.Body.String() != "PK\x03\x04" || w.Header().Get("Content-Type") != "application/epub+zip" {
		t.Fatalf("range: %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), `attachment; filename=Book.epub`) {
		t.Fatalf("disposition %q", w.Header().Get("Content-Disposition"))
	}
}

func TestProgressThroughTheAPI(t *testing.T) {
	e := newEnv(t)
	path := fmt.Sprintf("/api/items/%d/progress", e.comicID)
	put := func(body string) (int, store.Progress, bool) {
		w := e.do(req{method: "PUT", path: path, body: body, token: e.token})
		out := decode[struct {
			Progress store.Progress `json:"progress"`
			Applied  bool           `json:"applied"`
		}](t, w)
		return w.Code, out.Progress, out.Applied
	}
	if _, p, _ := put(`{"page":2}`); p.Page != 2 || p.Status != "reading" {
		t.Fatalf("forward: %+v", p)
	}
	if _, p, ok := put(`{"page":1}`); ok || p.Page != 2 {
		t.Fatalf("backward write applied: %+v", p)
	}
	if _, p, _ := put(`{"status":"read"}`); p.Page != 3 || p.Status != "read" {
		t.Fatalf("mark read: %+v", p)
	}
	if _, p, _ := put(`{"status":"unread"}`); p.Page != 0 || p.Status != "unread" {
		t.Fatalf("mark unread: %+v", p)
	}
	if _, p, ok := put(`{"page":1,"reset":true}`); !ok || p.Page != 1 {
		t.Fatalf("reset: %+v", p)
	}
	for _, bad := range []string{`{"page":4}`, `{"page":-1}`, `{}`, `{"status":"done"}`, `{"pageNum":1}`, `not json`} {
		if w := e.do(req{method: "PUT", path: path, body: bad, token: e.token}); w.Code != 400 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
}

func TestOPDSFeedsKeepTheTokenPrefixAndNeverRedirect(t *testing.T) {
	e := newEnv(t)
	base := "/opds/t/" + e.token + "/v1.2"
	cat := e.do(req{path: base + "/catalog"})
	if cat.Code != 200 || !strings.Contains(cat.Header().Get("Content-Type"), "kind=navigation") {
		t.Fatalf("catalog: %d", cat.Code)
	}
	body := cat.Body.String()
	if !strings.Contains(body, base+"/libraries/") || strings.Contains(body, `href="/opds/v1.2`) {
		t.Fatalf("links lost the token prefix:\n%s", body)
	}
	first := strings.Index(body, `rel="search"`)
	if first < 0 || !strings.Contains(body[first:first+200], "{searchTerms}") {
		t.Fatal("first search link must be the Atom template")
	}
	for _, p := range []string{"/opds", "/opds/v1.2", "/opds/v1.2/catalog", "/opds/v1.2/recent", "/opds/v1.2/search?q=saga", "/opds/v1.2/opensearch.xml"} {
		if w := e.do(req{path: p, basic: e.token}); w.Code != 200 {
			t.Errorf("%s: %d (redirects are refused by comics.skriv.ist)", p, w.Code)
		}
	}
	search := e.do(req{path: base + "/search?q=saga"}).Body.String()
	for _, want := range []string{`pse:count="3"`, `type="application/vnd.comicbook+zip"`, `type="application/vnd.comicbook-rar"`,
		`rel="http://opds-spec.org/image/thumbnail"`, base + "/items/"} {
		if !strings.Contains(search, want) {
			t.Errorf("search feed missing %s", want)
		}
	}
	books := e.do(req{path: base + "/search?q=book"}).Body.String()
	if !strings.Contains(books, `type="application/epub+zip"`) || strings.Contains(books, "opds-pse/stream") {
		t.Fatal("epub entry wrong")
	}
}

// atomFeed is the part of an OPDS feed the catalogue tests look at.
type atomFeed struct {
	Total   int        `xml:"http://a9.com/-/spec/opensearch/1.1/ totalResults"`
	Start   int        `xml:"http://a9.com/-/spec/opensearch/1.1/ startIndex"`
	Links   []atomLink `xml:"link"`
	Entries []struct {
		Title string     `xml:"title"`
		Links []atomLink `xml:"link"`
	} `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

func (e *env) feed(path string) atomFeed {
	e.t.Helper()
	w := e.do(req{path: path})
	var f atomFeed
	if w.Code != 200 {
		e.t.Fatalf("%s: %d", path, w.Code)
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &f); err != nil {
		e.t.Fatalf("%s: %v", path, err)
	}
	return f
}

func (f atomFeed) link(rel string) string {
	for _, l := range f.Links {
		if l.Rel == rel {
			return l.Href
		}
	}
	return ""
}

func (f atomFeed) titles() []string {
	out := []string{}
	for _, en := range f.Entries {
		out = append(out, en.Title)
	}
	return out
}

func TestOPDSLibraryListsAllTitles(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	comics := e.srv.Cfg.Libraries[0].Root
	want := []string{}
	for n := 1; n <= 60; n++ {
		name := fmt.Sprintf("Issue %d", n)
		tu.WriteZip(t, comics, fmt.Sprintf("Extra %d/%s.cbz", n, name), tu.Entry{Name: "01.png", Data: tu.PNG(8, 12, 10)})
		want = append(want, name)
	}
	want = append(want, "Saga #01", "Saga #02") // sort_title order: "issue 2" before "issue 10"
	if err := e.srv.Scanner.Run(ctx, ""); err != nil {
		t.Fatal(err)
	}
	libs := map[string]int64{}
	all, _ := e.srv.Store.Libraries(ctx)
	for _, l := range all {
		libs[l.Name] = l.ID
	}
	base := "/opds/t/" + e.token + "/v1.2"
	lib := fmt.Sprintf("%s/libraries/%d", base, libs["Calibre"])

	nav := e.feed(lib)
	if got := nav.titles(); len(got) < 2 || got[0] != "All titles" || got[1] == "All titles" {
		t.Fatalf("library entries: %v", got)
	}
	if l := nav.Entries[0].Links; len(l) != 1 || l[0].Href != lib+"/titles" || l[0].Rel != "subsection" {
		t.Fatalf("All titles link: %+v", l)
	}
	if got := e.feed(lib + "?offset=1").titles(); strings.Join(got, "|") != strings.Join(nav.titles()[1:], "|") {
		t.Fatalf("later page: %v", got) // All titles is entry 0, so offset 1 starts at the first series
	}

	books := e.feed(lib + "/titles")
	if got := books.titles(); strings.Join(got, "|") != "Infection|Mobi Only|No Cover File" {
		t.Fatalf("calibre titles: %v", got)
	}
	if books.link("up") != lib || books.link("self") != lib+"/titles" || books.link("start") != base+"/catalog" ||
		books.link("search") != base+"/search?q={searchTerms}" || books.link("next") != "" {
		t.Fatalf("calibre title links: %+v", books.Links)
	}

	// 61 series: All titles plus 49 series on page one, the other 12 on page two,
	// and the OpenSearch counters agree with what the pages hold.
	lib = fmt.Sprintf("%s/libraries/%d", base, libs["Comics"])
	nav, nav2 := e.feed(lib), e.feed(lib+"?offset=50")
	if len(nav.Entries) != opdsPerPage || nav.Total != 62 || nav.link("next") != lib+"?offset=50" ||
		len(nav2.Entries) != 12 || nav2.Start != 51 || nav2.Total != 62 || nav2.link("next") != "" {
		t.Fatalf("library pages: %d+%d entries, total %d, start %d", len(nav.Entries), len(nav2.Entries), nav.Total, nav2.Start)
	}
	seen := map[string]bool{}
	for _, title := range append(nav.titles()[1:], nav2.titles()...) {
		if title == "All titles" || seen[title] {
			t.Fatalf("library pages repeat %q", title)
		}
		seen[title] = true
	}
	first := e.feed(lib + "/titles")
	next := first.link("next")
	if len(first.Entries) != opdsPerPage || next != lib+"/titles?offset=50" || first.link("previous") != "" {
		t.Fatalf("first page: %d entries, next %q", len(first.Entries), next)
	}
	second := e.feed(next)
	if second.link("next") != "" || second.link("previous") != lib+"/titles?offset=0" {
		t.Fatalf("second page links: %+v", second.Links)
	}
	if got := append(first.titles(), second.titles()...); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("comic titles:\n got %v\nwant %v", got, want)
	}
	for _, en := range append(first.Entries, second.Entries...) {
		for _, l := range en.Links {
			if !strings.HasPrefix(l.Href, base+"/items/") {
				t.Fatalf("%s: link left the token prefix: %s", en.Title, l.Href)
			}
		}
	}

	for _, p := range []string{"/libraries/99/titles", "/libraries/0/titles", "/libraries/x/titles"} {
		if w := e.do(req{path: "/opds/v1.2" + p, basic: e.token}); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
}

func TestUIFallbackWithoutBuild(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/library/1"}); w.Code != 503 {
		t.Fatalf("unbuilt UI: %d", w.Code)
	}
	if w := e.do(req{path: "/api/nope", token: e.token}); w.Code != 404 || !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("unknown api path: %d", w.Code)
	}
}

func TestPageNamesAndNumbering(t *testing.T) {
	e := newEnv(t)
	w := e.do(req{path: fmt.Sprintf("/api/items/%d/pages", e.comicID), token: e.token})
	got := decode[struct {
		Pages []struct {
			Name string `json:"name"`
		} `json:"pages"`
	}](t, w)
	var names []string
	for _, p := range got.Pages {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "01.png,02.png,03.jpg" {
		t.Fatalf("names %v", names)
	}
	if w := e.do(req{path: fmt.Sprintf("/api/items/%d/pages", e.bookID), token: e.token}); w.Code != http.StatusNotFound {
		t.Fatalf("EPUB page list: %d", w.Code)
	}
	if w := e.do(req{path: fmt.Sprintf("/api/items/%d/pages", e.comicID)}); w.Code != http.StatusUnauthorized {
		t.Fatalf("page list without a token: %d", w.Code)
	}
	item := func(id int64) map[string]any {
		return decode[map[string]map[string]any](t, e.do(req{path: fmt.Sprintf("/api/items/%d", id), token: e.token}))["item"]
	}
	if item(e.comicID)["pageOrder"] != float64(2) {
		t.Fatalf("comic pageOrder: %v", item(e.comicID)["pageOrder"])
	}
	if _, ok := item(e.bookID)["pageOrder"]; ok {
		t.Fatal("an EPUB has no page numbering to report")
	}
	// A cached page of another numbering must not be revalidated as current.
	if etag := e.do(req{path: fmt.Sprintf("/api/items/%d/pages/1", e.comicID), token: e.token}).Header().Get("ETag"); !strings.HasSuffix(etag, `-p2"`) {
		t.Fatalf("page ETag %q lacks the numbering generation", etag)
	}
}

func calibreCover(t *testing.T, e *env) (store.Item, string) {
	t.Helper()
	it := e.items["Infection"]
	root := e.srv.Cfg.Libraries[2].Root
	return it, filepath.Join(filepath.Dir(filepath.Join(root, filepath.FromSlash(it.Path))), "cover.jpg")
}

func TestCalibreCoverMustStayInTheLibrary(t *testing.T) {
	e := newEnv(t)
	it, cover := calibreCover(t, e)
	outside := filepath.Join(t.TempDir(), "elsewhere.png")
	os.WriteFile(outside, tu.PNG(40, 60, 90), 0o644)
	os.Remove(cover)
	if err := os.Symlink(outside, cover); err != nil {
		t.Fatal(err)
	}
	if _, err := e.srv.inLibrary(context.Background(), it.LibraryID, cover); err != errNotFound {
		t.Fatalf("a cover.jpg pointing outside the library: %v", err)
	}
}

func TestCalibreCoverEditsRefreshTheThumbnail(t *testing.T) {
	e := newEnv(t)
	it, cover := calibreCover(t, e)
	path := fmt.Sprintf("/api/items/%d/cover", it.ID)
	before := e.do(req{path: path, token: e.token}).Body.Bytes()
	os.WriteFile(cover, tu.PNG(40, 60, 30), 0o644)
	later := time.Now().Add(time.Hour)
	os.Chtimes(cover, later, later)
	if after := e.do(req{path: path, token: e.token}).Body.Bytes(); bytes.Equal(before, after) {
		t.Fatal("a cover edited in Calibre still shows the old thumbnail")
	}
}
