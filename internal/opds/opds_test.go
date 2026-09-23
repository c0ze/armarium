package opds

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func TestFeedIsValidAtomWithLinksBeforeEntries(t *testing.T) {
	f := Feed{
		ID: "urn:armarium:x", Title: `Tom & Jerry <1>`, Updated: time.Unix(0, 0),
		Links: []Link{{Rel: "next", Href: "/opds/v1.2/x?offset=50", Type: AcqType}},
		Entries: []Entry{{ID: "urn:armarium:item:1", Title: "Issue #1", Links: []Link{
			{Rel: RelAcq, Href: "/f", Type: "application/vnd.comicbook+zip"},
			{Rel: RelPSE, Href: "/p/{pageNumber}", Type: "image/jpeg", PSECount: 24, PSELastRead: 4, PSELastReadDate: time.Unix(10, 0)},
		}}},
		Total: 120, Offset: 50, PerPage: 50,
	}
	var b bytes.Buffer
	f.WriteTo(&b)
	out := b.String()

	var parsed struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
		Title   string   `xml:"title"`
		Entries []struct {
			Links []struct {
				Rel   string `xml:"rel,attr"`
				Count string `xml:"http://vaemendis.net/opds-pse/ns count,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(b.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid XML: %v\n%s", err, out)
	}
	if parsed.Title != "Tom & Jerry <1>" || parsed.Entries[0].Links[1].Count != "24" {
		t.Fatalf("parsed %+v", parsed)
	}
	if strings.Index(out, `rel="next"`) > strings.Index(out, "<entry>") {
		t.Fatal("feed links must precede entries")
	}
	for _, want := range []string{`pse:count="24"`, `pse:lastRead="4"`, "<opensearch:startIndex>51<", `{pageNumber}`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestOpenSearchEscapesTemplate(t *testing.T) {
	out := string(OpenSearch("/opds/v1.2/search?q={searchTerms}&x=1"))
	if !strings.Contains(out, `template="/opds/v1.2/search?q={searchTerms}&amp;x=1"`) {
		t.Fatal(out)
	}
}
