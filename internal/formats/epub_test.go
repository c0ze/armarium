package formats

import (
	"testing"

	tu "github.com/c0ze/armarium/internal/testutil"
)

func TestEPUBParsesMetadataSpineTOCAndCover(t *testing.T) {
	p := tu.WriteEPUB(t, t.TempDir(), "b.epub", tu.EPUBOpts{
		Title:    "A Book",
		Chapters: []string{"<p>one</p>", "<p>two</p>"},
	})
	e, err := OpenEPUB(p, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Title != "A Book" || e.Author != "Test Author" {
		t.Fatalf("metadata: %q %q", e.Title, e.Author)
	}
	if len(e.Spine) != 2 {
		t.Fatalf("spine: %v", e.Spine)
	}
	if e.Cover < 0 || e.Manifest[e.Cover].Href != "OEBPS/images/cover.png" {
		t.Fatalf("cover: %d", e.Cover)
	}
	toc := e.TOC()
	if len(toc) != 2 || toc[1].Title != "Chapter 2" || toc[1].Chapter != 1 {
		t.Fatalf("toc: %+v", toc)
	}
	b, err := e.ReadEntry(e.Spine[0], 1<<20)
	if err != nil || len(b) == 0 {
		t.Fatalf("chapter: %v", err)
	}
}

func TestResolveHrefRefusesEscapes(t *testing.T) {
	cases := map[string]bool{
		"text/ch1.xhtml":         true,
		"../images/a.png":        true, // relative to OEBPS/text: stays inside
		"../../../etc/passwd":    false,
		"/abs/path":              false,
		"https://evil.example/x": false,
		"data:image/png;base64,": false,
		"javascript:alert(1)":    false,
		"":                       false,
		"a%20b.xhtml#frag":       true,
	}
	for href, ok := range cases {
		p, got := ResolveHref("OEBPS/text", href)
		if got != ok {
			t.Errorf("%q: ok=%v (%q), want %v", href, got, p, ok)
		}
	}
	if p, _ := ResolveHref("OEBPS/text", "a%20b.xhtml#frag"); p != "OEBPS/text/a b.xhtml" {
		t.Errorf("decoded path = %q", p)
	}
}

func TestEPUBWithXML11Declaration(t *testing.T) {
	dir := t.TempDir()
	p := tu.WriteZip(t, dir, "x11.epub",
		tu.Entry{Name: "META-INF/container.xml", Data: []byte(`<?xml version="1.0"?><container><rootfiles><rootfile full-path="o.opf"/></rootfiles></container>`)},
		tu.Entry{Name: "o.opf", Data: []byte(`<?xml version="1.1"?><package><metadata><title>Eleven</title></metadata><manifest><item id="a" href="a.html" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="a"/></spine></package>`)},
		tu.Entry{Name: "a.html", Data: []byte("<p>x</p>")},
	)
	e, err := OpenEPUB(p, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Title != "Eleven" || len(e.Spine) != 1 {
		t.Fatalf("got %q %v", e.Title, e.Spine)
	}
}

func TestEPUBWithoutContainerFallsBackToOPF(t *testing.T) {
	p := tu.WriteZip(t, t.TempDir(), "nocontainer.epub",
		tu.Entry{Name: "mimetype", Data: []byte("application/epub+zip")},
		tu.Entry{Name: "OEBPS/content.opf", Data: []byte(`<package><metadata><title>Bilim</title></metadata><manifest><item id="a" href="a.html" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="a"/></spine></package>`)},
		tu.Entry{Name: "OEBPS/a.html", Data: []byte("<p>x</p>")},
	)
	e, err := OpenEPUB(p, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Title != "Bilim" || len(e.Spine) != 1 || e.Manifest[e.Spine[0]].Href != "OEBPS/a.html" {
		t.Fatalf("got %q %v", e.Title, e.Spine)
	}
}
