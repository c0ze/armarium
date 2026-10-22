package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/c0ze/armarium/internal/kosync"
)

// koHeaders are what CrossPoint sends next to its Basic credential.
var koHeaders = map[string]string{"X-Auth-User": "x4", "X-Auth-Key": "5f4dcc3b5aa765d61d8327deb882cf99",
	"Accept": "application/vnd.koreader.v1+json"}

func (e *env) kosync(method, path, body string) map[string]any {
	e.t.Helper()
	w := e.do(req{method: method, path: path, body: body, basic: e.token, headers: koHeaders})
	if w.Code != 200 {
		e.t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
	}
	return decode[map[string]any](e.t, w)
}

// downloadDigest fetches an item's file over OPDS, as the X4 does, and
// returns the KOReader document id of what it received.
func (e *env) downloadDigest(id int64) string {
	e.t.Helper()
	w := e.do(req{path: fmt.Sprintf("/opds/v1.2/items/%d/file", id), basic: e.token})
	if w.Code != 200 {
		e.t.Fatalf("download: %d", w.Code)
	}
	d, _ := kosync.Digest(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	return d
}

func TestKOSyncAuth(t *testing.T) {
	e := newEnv(t)
	if got := e.kosync("GET", "/kosync/users/auth", ""); got["authorized"] != "OK" {
		t.Errorf("auth = %v", got)
	}
	if w := e.do(req{path: "/kosync/users/auth", headers: koHeaders}); w.Code != 401 {
		t.Errorf("no credential: %d", w.Code)
	}
	if w := e.do(req{path: "/kosync/users/auth", basic: "wrong", headers: koHeaders}); w.Code != 401 {
		t.Errorf("wrong token: %d", w.Code)
	}
	if w := e.do(req{method: "POST", path: "/kosync/users/create", body: `{"username":"x4","password":"p"}`}); w.Code != 403 ||
		!strings.Contains(w.Body.String(), "no sign-up") {
		t.Errorf("sign-up: %d %s", w.Code, w.Body)
	}
	if w := e.do(req{path: "/kosync/healthcheck"}); w.Code != 200 {
		t.Errorf("healthcheck: %d", w.Code)
	}
}

func TestKOSyncUploadMovesProgress(t *testing.T) {
	e := newEnv(t)
	doc := e.downloadDigest(e.bookID)
	if w := e.do(req{path: "/kosync/syncs/progress/" + doc, basic: e.token, headers: koHeaders}); w.Code != 404 {
		t.Fatalf("before any upload: %d %s", w.Code, w.Body)
	}

	body := fmt.Sprintf(`{"document":%q,"progress":"/body/DocFragment[2]/body/p[1]/text().0","percentage":0.75,
		"device":"CrossPoint","device_id":"crosspoint-reader"}`, doc)
	if got := e.kosync("PUT", "/kosync/syncs/progress", body); got["document"] != doc || got["timestamp"] == nil {
		t.Fatalf("upload = %v", got)
	}
	it, _ := e.srv.Store.Item(context.Background(), e.bookID)
	// Weighed by the chapters' sizes, as CrossPoint does.
	sizes := e.srv.chapterSizes(context.Background(), it)
	if len(sizes) != 2 || sizes[0] == sizes[1] {
		t.Fatalf("fixture chapter sizes = %v, want two different", sizes)
	}
	_, scroll := kosync.ChapterOf("/body/DocFragment[2]/body", 0.75, sizes)
	wantLoc := fmt.Sprintf(`{"chapter":1,"scroll":%v}`, scroll)
	// On the last chapter at 75%: still reading.
	if it.Progress.Page != 2 || it.Progress.Status != "reading" || it.Progress.Locator != wantLoc {
		t.Fatalf("item progress = %+v, want locator %s", it.Progress, wantLoc)
	}

	// The upload comes back as uploaded, not echoed as newer Armarium progress.
	got := e.kosync("GET", "/kosync/syncs/progress/"+doc, "")
	if got["progress"] != "/body/DocFragment[2]/body/p[1]/text().0" || got["percentage"] != 0.75 ||
		got["device"] != "CrossPoint" || got["device_id"] != "crosspoint-reader" || got["document"] != doc {
		t.Fatalf("download = %v", got)
	}

	// Reading on in the web reader makes Armarium's position the newer one.
	w := e.do(req{method: "PUT", path: fmt.Sprintf("/api/items/%d/progress", e.bookID), token: e.token,
		body: `{"page":1,"locator":"{\"chapter\":0,\"scroll\":0.25}","reset":true}`})
	if w.Code != 200 {
		t.Fatalf("web progress: %d %s", w.Code, w.Body)
	}
	got = e.kosync("GET", "/kosync/syncs/progress/"+doc, "")
	if got["progress"] != "/body/DocFragment[1]/body" || got["percentage"] != kosync.Percentage(0, 0.25, sizes) ||
		got["device"] != "Armarium" {
		t.Fatalf("armarium position = %v", got)
	}
}

func TestKOSyncMatchesByMetadata(t *testing.T) {
	e := newEnv(t)
	cal := e.items["Infection"]
	// A Filename-method id the server never saw a download for.
	body := `{"document":"0123456789abcdef0123456789abcdef","progress":"/body/DocFragment[1]/body","percentage":0.4,
		"device":"CrossPoint","device_id":"crosspoint-reader",
		"metadata":{"filename":"John Betancourt - Infection.epub","title":"INFECTION","authors":"John Betancourt"}}`
	e.kosync("PUT", "/kosync/syncs/progress", body)
	it, _ := e.srv.Store.Item(context.Background(), cal.ID)
	if it.Progress.Page != 1 || it.Progress.Status != "reading" {
		t.Fatalf("calibre book progress = %+v", it.Progress)
	}
	// The end of the book marks it read; the id is linked now, no metadata needed.
	e.kosync("PUT", "/kosync/syncs/progress", `{"document":"0123456789abcdef0123456789abcdef",
		"progress":"/body/DocFragment[1]/body","percentage":1,"device":"CrossPoint","device_id":"crosspoint-reader"}`)
	if it, _ := e.srv.Store.Item(context.Background(), cal.ID); it.Progress.Status != "read" {
		t.Fatalf("finished book = %+v", it.Progress)
	}
}

func TestKOSyncUnknownDocumentRoundTrips(t *testing.T) {
	e := newEnv(t)
	body := `{"document":"feedface","progress":"/body/DocFragment[7]/body","percentage":0.9,"device":"KOReader","device_id":"k1"}`
	e.kosync("PUT", "/kosync/syncs/progress", body)
	if got := e.kosync("GET", "/kosync/syncs/progress/feedface", ""); got["progress"] != "/body/DocFragment[7]/body" {
		t.Fatalf("round trip = %v", got)
	}
}

func TestKOSyncRefusals(t *testing.T) {
	e := newEnv(t)
	good := `{"document":"feedface","progress":"p","percentage":0.5,"device":"d","device_id":"i"}`
	for _, c := range []struct {
		name string
		q    req
		code int
	}{
		{"anonymous", req{method: "PUT", path: "/kosync/syncs/progress", body: good, headers: koHeaders}, 401},
		// Basic alone is ambient (a browser may have cached it): needs the KOSync header.
		{"basic without x-auth-user", req{method: "PUT", path: "/kosync/syncs/progress", body: good, basic: e.token}, 403},
		{"not json", req{method: "PUT", path: "/kosync/syncs/progress", body: good, basic: e.token, headers: koHeaders, ctype: "text/plain"}, 415},
		{"bad document", req{method: "PUT", path: "/kosync/syncs/progress", basic: e.token, headers: koHeaders,
			body: `{"document":"../x","progress":"p","percentage":0.5,"device":"d"}`}, 400},
		{"bad percentage", req{method: "PUT", path: "/kosync/syncs/progress", basic: e.token, headers: koHeaders,
			body: `{"document":"feedface","progress":"p","percentage":2,"device":"d"}`}, 400},
		{"bad get document", req{path: "/kosync/syncs/progress/a.b", basic: e.token, headers: koHeaders}, 400},
	} {
		if w := e.do(c.q); w.Code != c.code {
			t.Errorf("%s: %d %s, want %d", c.name, w.Code, w.Body, c.code)
		}
	}
}
