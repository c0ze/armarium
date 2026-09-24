package sanitize

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html/charset"
)

var declaredCharset = regexp.MustCompile(`(?i)^\s*<\?xml[^>]*\bencoding\s*=\s*["']([\w.:-]+)["']|<meta[^>]*\bcharset\s*=\s*["']?([\w.:-]+)`)

// toUTF8 decodes a chapter that declares another encoding (Shift_JIS, cp1254…);
// the tokenizer reads bytes as UTF-8 and would turn it into mojibake. Only the
// document's first 1 KiB is searched, where the XML declaration or meta lives.
func toUTF8(src []byte) []byte {
	m := declaredCharset.FindSubmatch(src[:min(len(src), 1024)])
	if m == nil {
		return src
	}
	label := strings.ToLower(string(m[1]) + string(m[2]))
	if label == "utf-8" || label == "utf8" {
		return src
	}
	r, err := charset.NewReaderLabel(label, bytes.NewReader(src))
	if err != nil {
		return src // unknown label: UTF-8 is the EPUB default anyway
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return src
	}
	return out
}
