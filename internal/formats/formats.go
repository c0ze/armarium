// Package formats reads comic archives (CBZ, CBR), EPUBs and PDFs. Every reader
// enforces the configured limits because library files are untrusted input.
package formats

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// Limits caps what a single archive may make us do.
type Limits struct {
	MaxEntryBytes int64 // largest decompressed entry we will read or stream
	MaxEntries    int   // entries per archive
}

var (
	ErrTooLarge   = errors.New("archive entry exceeds size limit")
	ErrTooMany    = errors.New("archive has too many entries")
	ErrNoPage     = errors.New("page out of range")
	ErrNotArchive = errors.New("format has no pages")
)

// Container is a comic: an ordered list of page images.
type Container interface {
	Pages() int
	// Page streams page i (0-based) and returns its media type.
	Page(i int) (io.ReadCloser, string, error)
	Close() error
}

// Open opens a comic container for format "cbz" or "cbr". The archive type is
// taken from the file's first bytes when they are recognisable: comics named
// .cbr that are really ZIP files (and the reverse) are common.
func Open(p, format string, lim Limits) (Container, error) {
	if format != "cbz" && format != "cbr" {
		return nil, fmt.Errorf("%w: %s", ErrNotArchive, format)
	}
	switch sniff(p) {
	case "zip":
		return openZip(p, lim)
	case "rar":
		return openRar(p, lim)
	}
	if format == "cbz" {
		return openZip(p, lim)
	}
	return openRar(p, lim)
}

// sniff reads an archive's signature: "zip", "rar" or "" when unknown.
func sniff(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 7)
	n, _ := io.ReadFull(f, b)
	switch {
	case n >= 4 && string(b[:4]) == "PK\x03\x04":
		return "zip"
	case n >= 7 && string(b[:6]) == "Rar!\x1a\x07":
		return "rar"
	}
	return ""
}

// mediaTypes lists every format Armarium lists. CBZ, CBR, EPUB and PDF are read in
// the browser; the rest are download-only (KOReader and Kindle-style readers).
var mediaTypes = map[string]string{
	"cbz":  "application/vnd.comicbook+zip",
	"cbr":  "application/vnd.comicbook-rar",
	"epub": "application/epub+zip",
	"pdf":  "application/pdf",
	"mobi": "application/x-mobipocket-ebook",
	"azw3": "application/vnd.amazon.ebook",
	"azw":  "application/vnd.amazon.ebook",
	"fb2":  "application/x-fictionbook+xml",
	"djvu": "image/vnd.djvu",
	"txt":  "text/plain; charset=utf-8",
	"rtf":  "application/rtf",
}

// FormatOf maps a file name found by a folder scan to a format, or "". Loose
// text formats are left out here: a folder scan would pick up every README.
func FormatOf(name string) string {
	f := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if _, ok := mediaTypes[f]; !ok || f == "txt" || f == "rtf" {
		return ""
	}
	return f
}

// Readable reports whether Armarium can show the format in the browser.
func Readable(format string) bool {
	switch format {
	case "cbz", "cbr", "epub", "pdf":
		return true
	}
	return false
}

// MediaTypeOf returns the media type of a whole file in the given format.
func MediaTypeOf(format string) string {
	if t, ok := mediaTypes[format]; ok {
		return t
	}
	return "application/octet-stream"
}

var imageTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif",
	".webp": "image/webp", ".avif": "image/avif", ".bmp": "image/bmp", ".jxl": "image/jxl",
}

// ImageType returns the media type for an image entry name, or "" if it isn't a page.
func ImageType(name string) string { return imageTypes[strings.ToLower(path.Ext(name))] }

// isPage reports whether an archive entry is a page image (not junk metadata).
func isPage(name string) bool {
	if ImageType(name) == "" {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return false
		}
	}
	return true
}

// capped fails the read once more than n bytes come out, in case headers lie.
type capped struct {
	r io.ReadCloser
	n int64
}

func (c *capped) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n -= int64(n)
	if c.n < 0 {
		return n, ErrTooLarge
	}
	return n, err
}

func (c *capped) Close() error { return c.r.Close() }
