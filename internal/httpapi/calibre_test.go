package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestCalibreBooksServeFormatsCoversAndOPDS(t *testing.T) {
	e := newEnv(t)
	inf, mobi := e.items["Infection"], e.items["Mobi Only"]
	if inf.ID == 0 || mobi.ID == 0 {
		t.Fatalf("calibre items not scanned: %v", e.items)
	}
	w := e.do(req{path: fmt.Sprintf("/api/items/%d/file?format=mobi", inf.ID), token: e.token})
	if w.Code != 200 || w.Body.String() != "MOBI-BYTES" || w.Header().Get("Content-Type") != "application/x-mobipocket-ebook" {
		t.Fatalf("extra format: %d %q %s", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	if w := e.do(req{path: fmt.Sprintf("/api/items/%d/file?format=azw3", inf.ID), token: e.token}); w.Code != 404 {
		t.Fatalf("unknown format: %d", w.Code)
	}
	// The cover comes from Calibre's cover.jpg (a 40x60 PNG here), not the EPUB's.
	if w := e.do(req{path: fmt.Sprintf("/api/items/%d/cover", inf.ID), token: e.token}); w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("calibre cover: %d", w.Code)
	}
	if w := e.do(req{path: fmt.Sprintf("/api/items/%d/cover", mobi.ID), token: e.token}); w.Code != 404 {
		t.Fatalf("book without cover: %d", w.Code)
	}
	feed := e.do(req{path: "/opds/v1.2/search?q=infection", basic: e.token}).Body.String()
	for _, want := range []string{"<author><name>John Betancourt</name></author>", `type="application/epub+zip"`,
		`/file?format=mobi" type="application/x-mobipocket-ebook"`, `rel="http://opds-spec.org/image"`} {
		if !strings.Contains(feed, want) {
			t.Errorf("feed missing %s", want)
		}
	}
	if feed := e.do(req{path: "/opds/v1.2/search?q=mobi", basic: e.token}).Body.String(); strings.Contains(feed, "opds-spec.org/image") {
		t.Error("coverless book advertises an image")
	}
}
