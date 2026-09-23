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

// Document wraps a sanitized body fragment into a complete HTML page with the
// book's stylesheets after the reader's base stylesheet.
func Document(body []byte, sheets []string, base string) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(base) + `">`)
	for _, s := range sheets {
		b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(s) + `">`)
	}
	b.WriteString(`</head><body>`)
	b.Write(body)
	b.WriteString(`</body></html>`)
	return b.Bytes()
}
