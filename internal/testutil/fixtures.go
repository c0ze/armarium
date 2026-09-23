// Package testutil builds tiny comic and book fixtures for tests.
package testutil

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// PNG returns a w×h PNG filled with one gray level.
func PNG(w, h int, gray uint8) []byte {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = gray
	}
	img.Set(0, 0, color.Gray{Y: gray})
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// Entry is one file inside a fixture archive.
type Entry struct {
	Name string
	Data []byte
}

// WriteZip writes entries to dir/name and returns the full path.
func WriteZip(t testing.TB, dir, name string, entries ...Entry) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range entries {
		w, err := zw.Create(e.Name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(e.Data)
	}
	zw.Close()
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// WriteRAR writes a RAR 4 archive with stored (uncompressed) entries. No rar
// tool is needed: the format for stored files is small and fully specified.
func WriteRAR(t testing.TB, dir, name string, entries ...Entry) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	var b bytes.Buffer
	b.Write([]byte{0x52, 0x61, 0x72, 0x21, 0x1a, 0x07, 0x00}) // marker
	writeRARBlock(&b, 0x73, 0, make([]byte, 6))               // archive header
	for _, e := range entries {
		var h bytes.Buffer
		le := func(v any) { binary.Write(&h, binary.LittleEndian, v) }
		le(uint32(len(e.Data)))        // packed size
		le(uint32(len(e.Data)))        // unpacked size
		h.WriteByte(3)                 // host OS: unix
		le(crc32.ChecksumIEEE(e.Data)) // file CRC
		le(uint32(0x5a210000))         // DOS time
		h.WriteByte(20)                // version needed
		h.WriteByte(0x30)              // method: store
		le(uint16(len(e.Name)))        // name size
		le(uint32(0o100644))           // attributes
		h.WriteString(e.Name)
		writeRARBlock(&b, 0x74, 0x8000, h.Bytes())
		b.Write(e.Data)
	}
	writeRARBlock(&b, 0x7b, 0x4000, nil) // end of archive
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeRARBlock(b *bytes.Buffer, typ byte, flags uint16, body []byte) {
	var h bytes.Buffer
	h.WriteByte(typ)
	binary.Write(&h, binary.LittleEndian, flags)
	binary.Write(&h, binary.LittleEndian, uint16(7+len(body)))
	h.Write(body)
	crc := uint16(crc32.ChecksumIEEE(h.Bytes()))
	binary.Write(b, binary.LittleEndian, crc)
	b.Write(h.Bytes())
}

// EPUBOpts customises WriteEPUB.
type EPUBOpts struct {
	Title    string
	Chapters []string // XHTML body contents, one per chapter
	Extra    []Entry  // additional files under OEBPS/
	ExtraOPF string   // extra manifest <item> elements
}

// WriteEPUB writes a minimal EPUB 3 with a nav document and a cover image.
func WriteEPUB(t testing.TB, dir, name string, o EPUBOpts) string {
	t.Helper()
	manifest, spine, nav := "", "", ""
	entries := []Entry{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte(`<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{"OEBPS/images/cover.png", PNG(40, 60, 128)},
	}
	for i, body := range o.Chapters {
		id := "c" + string(rune('0'+i))
		file := "text/ch" + string(rune('0'+i)) + ".xhtml"
		manifest += `<item id="` + id + `" href="` + file + `" media-type="application/xhtml+xml"/>`
		spine += `<itemref idref="` + id + `"/>`
		nav += `<li><a href="` + file + `">Chapter ` + string(rune('1'+i)) + `</a></li>`
		entries = append(entries, Entry{"OEBPS/" + file, []byte(`<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>x</title><link rel="stylesheet" href="../style.css"/></head><body>` + body + `</body></html>`)})
	}
	entries = append(entries,
		Entry{"OEBPS/nav.xhtml", []byte(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol>` + nav + `</ol></nav></body></html>`)},
		Entry{"OEBPS/style.css", []byte(`p { margin: 0 } .x { background: url(images/cover.png) } .y { background: url(https://evil.example/t.png) }`)},
		Entry{"OEBPS/content.opf", []byte(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + o.Title + `</dc:title><dc:creator>Test Author</dc:creator><meta name="cover" content="cover"/></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="cover" href="images/cover.png" media-type="image/png"/><item id="css" href="style.css" media-type="text/css"/>` + manifest + o.ExtraOPF + `</manifest><spine>` + spine + `</spine></package>`)},
	)
	for _, e := range o.Extra {
		entries = append(entries, Entry{"OEBPS/" + e.Name, e.Data})
	}
	return WriteZip(t, dir, name, entries...)
}
