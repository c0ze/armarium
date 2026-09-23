package httpapi

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

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

func TestUIFallbackWithoutBuild(t *testing.T) {
	e := newEnv(t)
	if w := e.do(req{path: "/library/1"}); w.Code != 503 {
		t.Fatalf("unbuilt UI: %d", w.Code)
	}
	if w := e.do(req{path: "/api/nope", token: e.token}); w.Code != 404 || !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("unknown api path: %d", w.Code)
	}
}
