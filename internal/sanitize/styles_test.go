package sanitize

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func TestKeepsBookStylingSafely(t *testing.T) {
	p := Chapter([]byte(`<html class="vrtl" xml:lang="ja" onload="x()"><head>
		<style>p.sgc-1 { font-weight: bold } .i { background: url(img/a.png) } .q::after { content: "</p>" }</style>
		<style></style></head><body class="main" style="margin: 0">
		<div style="white-space: pre-wrap; background: url(https://evil.example/t.png)">  code</div><p>after</p></body></html>`), resolver())
	doc := string(Document(p, "/read/base.css"))
	for _, want := range []string{
		`<html class="vrtl" lang="ja"><head>`,
		`<body class="main" style="margin: 0">`,
		`p.sgc-1 { font-weight: bold }`,
		`url("/read/1/res/3")`,
		`content: "<\/p>"`,
		`<div style="white-space: pre-wrap; background: url(&#34;&#34;)">  code</div>`,
		`<p>after</p>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q in\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "evil.example") || strings.Contains(doc, "onload") {
		t.Errorf("unsafe content leaked:\n%s", doc)
	}
}

func TestDecodesDeclaredCharset(t *testing.T) {
	sjis, _ := japanese.ShiftJIS.NewEncoder().String("<p>吾輩は猫である</p>")
	p := Chapter([]byte(`<?xml version="1.0" encoding="Shift_JIS"?><html><body>`+sjis+`</body></html>`), resolver())
	if !strings.Contains(string(p.Body), "吾輩は猫である") {
		t.Fatalf("got %q", p.Body)
	}
}
