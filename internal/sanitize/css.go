package sanitize

import (
	"bytes"
	"html"
	"regexp"
)

var (
	cssURL    = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)'"\s]*))\s*\)`)
	cssImport = regexp.MustCompile(`(?i)@import\s+(?:"[^"]*"|'[^']*')[^;]*;?`)
)

// CSS rewrites url() references to served same-book URLs and drops @import of
// bare strings. Anything unresolvable becomes url("") so the browser loads nothing;
// the CSP on /read/* is the second fence.
func CSS(src []byte, resolve func(ref string) (string, bool)) []byte {
	src = cssImport.ReplaceAll(src, nil)
	return cssURL.ReplaceAllFunc(src, func(m []byte) []byte {
		g := cssURL.FindSubmatch(m)
		ref := string(bytes.Join([][]byte{g[1], g[2], g[3]}, nil))
		if u, ok := resolve(ref); ok {
			return []byte(`url("` + u + `")`)
		}
		return []byte(`url("")`)
	})
}

// Document wraps a sanitized chapter into a complete HTML page: the reader's base
// stylesheet first, then the book's stylesheets and its own <style> blocks.
func Document(p Page, base string) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html` + p.Root + `><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(base) + `">`)
	for _, s := range p.Sheets {
		b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(s) + `">`)
	}
	for _, s := range p.Styles {
		b.WriteString(`<style>`)
		b.Write(s)
		b.WriteString(`</style>`)
	}
	b.WriteString(`</head><body` + p.BodyAttrs + `>`)
	b.Write(p.Body)
	b.WriteString(`</body></html>`)
	return b.Bytes()
}
