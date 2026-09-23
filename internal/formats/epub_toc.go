package formats

import (
	"bytes"
	"path"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// TOCEntry points at a chapter (spine index) plus an optional fragment.
type TOCEntry struct {
	Title    string `json:"title"`
	Chapter  int    `json:"chapter"` // 0-based spine index
	Fragment string `json:"fragment,omitempty"`
	Depth    int    `json:"depth"`
}

// TOC returns the table of contents from the EPUB 3 nav document, falling back
// to the NCX, then to one entry per spine item.
func (e *EPUB) TOC() []TOCEntry {
	for i, m := range e.Manifest {
		if hasProp(m.Properties, "nav") {
			if toc := e.navTOC(i); len(toc) > 0 {
				return toc
			}
		}
	}
	for i, m := range e.Manifest {
		if m.MediaType == "application/x-dtbncx+xml" {
			if toc := e.ncxTOC(i); len(toc) > 0 {
				return toc
			}
		}
	}
	toc := make([]TOCEntry, len(e.Spine))
	for i := range e.Spine {
		toc[i] = TOCEntry{Title: "Section " + strconv.Itoa(i+1), Chapter: i}
	}
	return toc
}

// target maps an href inside the document at docPath to a chapter and fragment.
func (e *EPUB) target(docPath, href string) (int, string, bool) {
	p, ok := ResolveHref(path.Dir(docPath), href)
	if !ok {
		return 0, "", false
	}
	idx, ok := e.byPath[p]
	if !ok {
		return 0, "", false
	}
	ch, ok := e.SpineIndexOf(idx)
	frag := ""
	if i := strings.IndexByte(href, '#'); i >= 0 {
		frag = href[i+1:]
	}
	return ch, frag, ok
}

func (e *EPUB) navTOC(idx int) []TOCEntry {
	b, err := e.ReadEntry(idx, maxMetaBytes)
	if err != nil {
		return nil
	}
	doc := e.Manifest[idx].Href
	z := html.NewTokenizer(bytes.NewReader(b))
	var out []TOCEntry
	inTOC, navDepth, olDepth := false, 0, 0
	var cur *TOCEntry
	var text strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		tok := z.Token()
		switch {
		case tt == html.StartTagToken && tok.Data == "nav":
			if inTOC {
				navDepth++
			} else if attr(tok, "epub:type") == "toc" || attr(tok, "role") == "doc-toc" {
				inTOC, navDepth = true, 1
			}
		case tt == html.EndTagToken && tok.Data == "nav" && inTOC:
			if navDepth--; navDepth == 0 {
				return out
			}
		case !inTOC:
		case tt == html.StartTagToken && tok.Data == "ol":
			olDepth++
		case tt == html.EndTagToken && tok.Data == "ol":
			olDepth--
		case tt == html.StartTagToken && tok.Data == "a":
			if ch, frag, ok := e.target(doc, attr(tok, "href")); ok {
				cur = &TOCEntry{Chapter: ch, Fragment: frag, Depth: max(olDepth-1, 0)}
				text.Reset()
			}
		case tt == html.TextToken && cur != nil:
			text.WriteString(tok.Data)
		case tt == html.EndTagToken && tok.Data == "a" && cur != nil:
			cur.Title = strings.Join(strings.Fields(text.String()), " ")
			out = append(out, *cur)
			cur = nil
		}
	}
}

type navPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []navPoint `xml:"navPoint"`
}

func (e *EPUB) ncxTOC(idx int) []TOCEntry {
	var ncx struct {
		Points []navPoint `xml:"navMap>navPoint"`
	}
	if err := e.decodeXML(e.Manifest[idx].Href, &ncx); err != nil {
		return nil
	}
	var out []TOCEntry
	var walk func([]navPoint, int)
	walk = func(ps []navPoint, depth int) {
		for _, p := range ps {
			if ch, frag, ok := e.target(e.Manifest[idx].Href, p.Content.Src); ok {
				out = append(out, TOCEntry{Title: strings.TrimSpace(p.Label), Chapter: ch, Fragment: frag, Depth: depth})
			}
			walk(p.Children, depth+1)
		}
	}
	walk(ncx.Points, 0)
	return out
}

func attr(t html.Token, key string) string {
	for _, a := range t.Attr {
		k := a.Key
		if a.Namespace != "" {
			k = a.Namespace + ":" + a.Key
		}
		if k == key {
			return a.Val
		}
	}
	return ""
}
