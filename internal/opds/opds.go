// Package opds renders OPDS 1.2 Atom feeds with OPDS-PSE page streaming links.
// The XML is written by hand so the namespace prefixes are exactly the ones
// clients look for (pse:count, opensearch:totalResults).
package opds

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"time"
)

const (
	NavType  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	AcqType  = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	RelAcq   = "http://opds-spec.org/acquisition"
	RelImage = "http://opds-spec.org/image"
	RelThumb = "http://opds-spec.org/image/thumbnail"
	RelPSE   = "http://vaemendis.net/opds-pse/stream"
	RelSub   = "subsection"
)

type Link struct {
	Rel, Href, Type, Title string
	PSECount               int // pse:count, only on RelPSE links
	PSELastRead            int // pse:lastRead (0-based page), -1 for none
	PSELastReadDate        time.Time
}

type Entry struct {
	ID, Title string
	Author    string
	Updated   time.Time
	Content   string
	Links     []Link
}

type Feed struct {
	ID, Title string
	Updated   time.Time
	// Links go before the entries: some clients only read feed-level
	// next/search links that precede the first <entry>.
	Links   []Link
	Entries []Entry
	// Paging, written as OpenSearch elements when Total > 0.
	Total, Offset, PerPage int
}

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func writeLink(b *bytes.Buffer, l Link) {
	b.WriteString(`<link rel="` + esc(l.Rel) + `" href="` + esc(l.Href) + `"`)
	if l.Type != "" {
		b.WriteString(` type="` + esc(l.Type) + `"`)
	}
	if l.Title != "" {
		b.WriteString(` title="` + esc(l.Title) + `"`)
	}
	if l.Rel == RelPSE {
		b.WriteString(` pse:count="` + strconv.Itoa(l.PSECount) + `"`)
		if l.PSELastRead >= 0 && !l.PSELastReadDate.IsZero() {
			b.WriteString(` pse:lastRead="` + strconv.Itoa(l.PSELastRead) + `" pse:lastReadDate="` + stamp(l.PSELastReadDate) + `"`)
		}
	}
	b.WriteString("/>")
}

// WriteTo renders the feed.
func (f Feed) WriteTo(w io.Writer) (int64, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog"` +
		` xmlns:pse="http://vaemendis.net/opds-pse/ns" xmlns:opensearch="http://a9.com/-/spec/opensearch/1.1/"` +
		` xmlns:dc="http://purl.org/dc/terms/">`)
	b.WriteString("<id>" + esc(f.ID) + "</id><title>" + esc(f.Title) + "</title><updated>" + stamp(f.Updated) + "</updated>")
	b.WriteString("<author><name>Armarium</name></author>")
	for _, l := range f.Links {
		writeLink(&b, l)
	}
	if f.Total > 0 {
		b.WriteString("<opensearch:totalResults>" + strconv.Itoa(f.Total) + "</opensearch:totalResults>")
		b.WriteString("<opensearch:itemsPerPage>" + strconv.Itoa(f.PerPage) + "</opensearch:itemsPerPage>")
		b.WriteString("<opensearch:startIndex>" + strconv.Itoa(f.Offset+1) + "</opensearch:startIndex>")
	}
	for _, e := range f.Entries {
		b.WriteString("<entry><id>" + esc(e.ID) + "</id><title>" + esc(e.Title) + "</title><updated>" + stamp(e.Updated) + "</updated>")
		if e.Author != "" {
			b.WriteString("<author><name>" + esc(e.Author) + "</name></author>")
		}
		if e.Content != "" {
			b.WriteString(`<content type="text">` + esc(e.Content) + "</content>")
		}
		for _, l := range e.Links {
			writeLink(&b, l)
		}
		b.WriteString("</entry>")
	}
	b.WriteString("</feed>\n")
	return b.WriteTo(w)
}

// OpenSearch returns the description document for a search template URL.
func OpenSearch(template string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
<ShortName>Armarium</ShortName><Description>Search the Armarium library</Description>
<InputEncoding>UTF-8</InputEncoding><OutputEncoding>UTF-8</OutputEncoding>
<Url type="` + esc(AcqType) + `" template="` + esc(template) + `"/>
</OpenSearchDescription>
`)
}
