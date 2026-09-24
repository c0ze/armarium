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

// Elements dropped together with everything inside them. <head> is not one of
// them: its <link> and <style> are collected, <title> and <script> are dropped
// here, and <meta>/<base> are not on the allowlist.
var dropped = set("script", "style", "svg", "math", "iframe", "object", "embed", "form", "template",
	"noscript", "title", "button", "input", "select", "textarea", "audio", "video", "canvas",
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

// Page is a sanitized chapter, ready for Document.
type Page struct {
	Body            []byte
	Sheets          []string // served URLs of same-book stylesheets
	Styles          [][]byte // the chapter's own <style> blocks, url()s rewritten
	Root, BodyAttrs string   // class/lang/dir of <html> and <body>, written out
}

// Chapter sanitizes one XHTML document.
func Chapter(src []byte, o Options) Page {
	src = selfClosingRaw.ReplaceAll(toUTF8(src), []byte("<$1$2></$1>"))
	z := xhtml.NewTokenizer(bytes.NewReader(src))
	var out bytes.Buffer
	var p Page
	skip := 0 // depth inside a dropped element
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			p.Body = out.Bytes()
			return p
		}
		tok := z.Token()
		name := tok.Data
		switch tt {
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			if name == "link" && strings.EqualFold(attr(tok, "rel"), "stylesheet") {
				if u, ok := o.Resource(attr(tok, "href")); ok {
					p.Sheets = append(p.Sheets, u)
				}
				continue
			}
			// Vertical Japanese and similar layouts hang off classes on <html> or
			// <body> (html.vrtl { writing-mode: vertical-rl }), so those survive.
			if skip == 0 && (name == "html" || name == "body") {
				var b bytes.Buffer
				writeGlobalAttrs(&b, tok, o)
				if name == "html" {
					p.Root = b.String()
				} else {
					p.BodyAttrs = b.String()
				}
				continue
			}
			// A book's own <style> carries real formatting (Sigil's p.sgc-1 {bold});
			// it is raw text, so the next token is its whole content.
			if skip == 0 && name == "style" && tt == xhtml.StartTagToken {
				if z.Next() == xhtml.TextToken {
					p.Styles = append(p.Styles, safeCSS(z.Text(), o))
					skip++ // the end tag comes next and pops this
				}
				continue // an empty <style></style> already consumed its end tag
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
	writeGlobalAttrs(out, tok, o)
	for _, a := range tok.Attr {
		if tagAttrs[name][a.Key] {
			writeAttr(out, a.Key, a.Val)
		}
	}
	out.WriteString(">")
}

// writeGlobalAttrs keeps id, class, title, lang and dir, plus a style attribute
// with its url()s pinned to the book (code listings rely on inline
// white-space: pre and a monospace font).
func writeGlobalAttrs(out *bytes.Buffer, tok xhtml.Token, o Options) {
	for _, a := range tok.Attr {
		key := a.Key
		if key == "xml:lang" {
			key = "lang"
		}
		switch {
		case globalAttrs[key]:
			writeAttr(out, key, a.Val)
		case key == "style":
			writeAttr(out, key, string(CSS([]byte(a.Val), o.Resource)))
		}
	}
}

// safeCSS rewrites a <style> block's url()s and makes sure it cannot close its
// own element when written back out.
func safeCSS(src []byte, o Options) []byte {
	return bytes.ReplaceAll(CSS(src, o.Resource), []byte("</"), []byte(`<\/`))
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
