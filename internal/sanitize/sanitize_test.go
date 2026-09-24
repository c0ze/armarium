package sanitize

import (
	"bytes"
	"strings"
	"testing"
)

func resolver() Options {
	return Options{
		Resource: func(ref string) (string, bool) {
			switch ref {
			case "img/a.png":
				return "/read/1/res/3", true
			case "style.css":
				return "/read/1/res/4", true
			}
			return "", false
		},
		Link: func(ref string) (string, bool) {
			if strings.HasPrefix(ref, "ch2.xhtml") {
				return "/read/1/chapter/2" + strings.TrimPrefix(ref, "ch2.xhtml"), true
			}
			return "", false
		},
	}
}

func clean(t *testing.T, in string) string {
	t.Helper()
	p := Chapter([]byte(in), resolver())
	return p.Root + p.BodyAttrs + string(p.Body) + string(bytes.Join(p.Styles, nil))
}

// The payloads below come from the audit of the Python reader (2026-09-23).
func TestDropsScriptVectors(t *testing.T) {
	bad := []string{
		`<svg><style><img src=x onerror=alert(1)></style></svg>`,
		`<math><mi xlink:href="javascript:alert(1)">x</mi></math>`,
		`<script>alert(1)</script>`,
		`<img src="img/a.png" onerror="alert(1)">`,
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<iframe src="https://evil.example"></iframe>`,
		`<object data="x.swf"></object><embed src="x">`,
		`<form action="https://evil.example"><input name=a></form>`,
		`<p style="background:url(javascript:alert(1))">x</p>`,
		`<style>body{background:url(https://evil.example/t)}</style>`,
		`<img srcset="https://evil.example/a.png 1x" src="img/a.png">`,
		`<base href="https://evil.example/">`,
		`<meta http-equiv="refresh" content="0;url=https://evil.example">`,
		`<link rel="stylesheet" href="https://evil.example/x.css">`,
		`<div onclick="alert(1)" onmouseover=alert(1)>x</div>`,
		`<template><script>alert(1)</script></template>`,
		`<noscript><img src=x onerror=alert(1)></noscript>`,
		`<img src="https://evil.example/pixel.png">`,
		`<svg/onload=alert(1)>`,
	}
	for _, in := range bad {
		out := strings.ToLower(clean(t, in))
		for _, needle := range []string{"<script", "onerror", "onload", "onclick", "onmouseover", "javascript:",
			"evil.example", "<svg", "<math", "<style", "<iframe", "<object", "<embed", "<form", "<input",
			"srcset", "<base", "<meta", "<link", "<template", "<noscript"} {
			if strings.Contains(out, needle) {
				t.Errorf("input %q\n  output %q\n  contains %q", in, out, needle)
			}
		}
	}
}

func TestKeepsContentAndRewritesBookURLs(t *testing.T) {
	out := clean(t, `<html><head><title/><link rel="stylesheet" href="style.css"/></head><body>
		<h1 id="top" class="t" epub:type="title">Hello &amp; <em>world</em></h1>
		<p xml:lang="tr">Merhaba<br/>dünya</p>
		<img src="img/a.png" alt="A" width="10"/>
		<a href="ch2.xhtml#s1">next</a> <a href="#top">top</a> <a href="https://example.com">ext</a>
		<table><tr><td colspan="2">x</td></tr></table>
		<div/><custom-tag>kept text</custom-tag>
	</body></html>`)
	for _, want := range []string{
		`<h1 id="top" class="t">Hello &amp; <em>world</em></h1>`,
		`<p lang="tr">Merhaba<br>dünya</p>`,
		`<img src="/read/1/res/3" alt="A" width="10">`,
		`<a href="/read/1/chapter/2#s1">next</a>`,
		`<a href="#top">top</a>`,
		`<a>ext</a>`,
		`<td colspan="2">x</td>`,
		`<div></div>`,
		`kept text`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "custom-tag") || strings.Contains(out, "epub:type") {
		t.Errorf("unknown tag or attribute leaked:\n%s", out)
	}
}

func TestSVGCoverImageBecomesImg(t *testing.T) {
	out := clean(t, `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1 1"><image xlink:href="img/a.png"/></svg>`)
	if !strings.Contains(out, `<img src="/read/1/res/3" alt="">`) || strings.Contains(out, "<svg") {
		t.Fatalf("got %q", out)
	}
}

func TestStylesheetsAreCollected(t *testing.T) {
	sheets := Chapter([]byte(`<head><link rel="stylesheet" href="style.css"/><link rel="stylesheet" href="https://evil.example/x.css"/></head><p>x</p>`), resolver()).Sheets
	if len(sheets) != 1 || sheets[0] != "/read/1/res/4" {
		t.Fatalf("sheets = %v", sheets)
	}
}

func TestCSSRewritesURLs(t *testing.T) {
	css := CSS([]byte(`.a{background:url(img/a.png)} .b{background:url("https://evil.example/t.png")} @import "https://evil.example/x.css"; .c{x:url('img/a.png')}`),
		resolver().Resource)
	s := string(css)
	if strings.Contains(s, "evil.example") || strings.Count(s, "/read/1/res/3") != 2 {
		t.Fatalf("got %s", s)
	}
}

func TestDocumentWrapsBody(t *testing.T) {
	doc := string(Document(Page{Body: []byte("<p>x</p>"), Sheets: []string{"/read/1/res/4"}}, "/read/base.css"))
	if !strings.HasPrefix(doc, "<!doctype html><html><head>") || !strings.Contains(doc, `<link rel="stylesheet" href="/read/1/res/4">`) ||
		!strings.Contains(doc, "<body><p>x</p></body>") {
		t.Fatalf("got %s", doc)
	}
}
