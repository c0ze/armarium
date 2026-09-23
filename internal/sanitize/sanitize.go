// Package sanitize turns untrusted EPUB XHTML into a safe HTML fragment using an
// allowlist of elements and attributes. Everything else is dropped or unwrapped;
// every URL must resolve to a resource of the same book.
package sanitize

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Options resolve book-relative references (already relative to the chapter's own
// location is the caller's job) into served URLs.
type Options struct {
	Resource func(ref string) (string, bool) // images and stylesheets
	Link     func(ref string) (string, bool) // links to other chapters
}

// Elements dropped together with everything inside them.
var dropped = set("script", "style", "svg", "math", "iframe", "object", "embed", "form", "template",
	"noscript", "head", "title", "button", "input", "select", "textarea", "audio", "video", "canvas",
	"applet", "frame", "frameset", "noembed", "noframes", "xmp", "plaintext", "portal", "dialog")

var allowed = set("a", "abbr", "address", "article", "aside", "b", "bdi", "bdo", "big", "blockquote", "br",
	"caption", "center", "cite", "code", "col", "colgroup", "dd", "del", "dfn", "div", "dl", "dt", "em",
	"figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr", "i", "img",
	"ins", "kbd", "li", "main", "mark", "nav", "ol", "p", "pre", "q", "rb", "rp", "rt", "ruby", "s", "samp",
	"section", "small", "span", "strike", "strong", "sub", "sup", "table", "tbody", "td", "tfoot", "th",
	"thead", "time", "tr", "tt", "u", "ul", "var", "wbr")

// XHTML writes <title/> and <script src=".."/>; an HTML tokenizer still switches to
// raw-text mode on those and swallows the rest of the document, so expand them.
var selfClosingRaw = regexp.MustCompile(`(?i)<(title|script|style|textarea|xmp|iframe|noembed|noframes|noscript|plaintext)(\s[^>]*?)?\s*/>`)

var void = set("br", "col", "hr", "img", "wbr")

var globalAttrs = set("id", "class", "title", "lang", "dir")

var tagAttrs = map[string]map[string]bool{
	"img":      set("alt", "width", "height"),
	"td":       set("colspan", "rowspan", "headers"),
	"th":       set("colspan", "rowspan", "headers", "scope"),
	"ol":       set("start", "reversed", "type"),
	"li":       set("value"),
	"col":      set("span"),
	"colgroup": set("span"),
	"time":     set("datetime"),
}

// Chapter sanitizes one XHTML document and returns the body fragment plus the
// served URLs of its same-book stylesheets.
func Chapter(src []byte, o Options) ([]byte, []string) {
	src = selfClosingRaw.ReplaceAll(src, []byte("<$1$2></$1>"))
	z := xhtml.NewTokenizer(bytes.NewReader(src))
	var out bytes.Buffer
	var sheets []string
	skip := 0 // depth inside a dropped element
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return out.Bytes(), sheets
		}
		tok := z.Token()
		name := tok.Data
		switch tt {
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			if name == "link" && strings.EqualFold(attr(tok, "rel"), "stylesheet") {
				if u, ok := o.Resource(attr(tok, "href")); ok {
					sheets = append(sheets, u)
				}
				continue
			}
			if skip > 0 || dropped[name] {
				if skip > 0 && name == "image" { // SVG-wrapped cover pages
					writeSVGImage(&out, tok, o)
				}
				if tt == xhtml.StartTagToken && !void[name] {
					skip++
				}
				continue
			}
			if !allowed[name] {
				continue // unknown element: unwrap, keep its text
			}
			if name == "img" && !imgSrcOK(tok, o) {
				continue
			}
			writeStart(&out, tok, o)
			if tt == xhtml.SelfClosingTagToken && !void[name] {
				out.WriteString("</" + name + ">")
			}
		case xhtml.EndTagToken:
			if skip > 0 {
				if !void[name] {
					skip--
				}
				continue
			}
			if allowed[name] && !void[name] {
				out.WriteString("</" + name + ">")
			}
		case xhtml.TextToken:
			if skip == 0 {
				out.WriteString(html.EscapeString(tok.Data))
			}
		}
	}
}

func imgSrcOK(tok xhtml.Token, o Options) bool {
	_, ok := o.Resource(attr(tok, "src"))
	return ok
}

func writeStart(out *bytes.Buffer, tok xhtml.Token, o Options) {
	name := tok.Data
	out.WriteString("<" + name)
	switch name {
	case "img":
		u, _ := o.Resource(attr(tok, "src"))
		writeAttr(out, "src", u)
	case "a":
		if href := attr(tok, "href"); strings.HasPrefix(href, "#") {
			writeAttr(out, "href", href)
		} else if u, ok := o.Link(href); ok {
			writeAttr(out, "href", u)
		}
	}
	for _, a := range tok.Attr {
		key := a.Key
		if key == "xml:lang" {
			key = "lang"
		}
		if globalAttrs[key] || tagAttrs[name][key] {
			writeAttr(out, key, a.Val)
		}
	}
	out.WriteString(">")
}

func writeSVGImage(out *bytes.Buffer, tok xhtml.Token, o Options) {
	href := attr(tok, "xlink:href")
	if href == "" {
		href = attr(tok, "href")
	}
	if u, ok := o.Resource(href); ok {
		out.WriteString(`<img src="` + html.EscapeString(u) + `" alt="">`)
	}
}

func writeAttr(out *bytes.Buffer, k, v string) {
	out.WriteString(" " + k + `="` + html.EscapeString(v) + `"`)
}

func attr(t xhtml.Token, key string) string {
	for _, a := range t.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
