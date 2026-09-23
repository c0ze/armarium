package formats

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	tu "github.com/c0ze/armarium/internal/testutil"
)

var lim = Limits{MaxEntryBytes: 1 << 20, MaxEntries: 100}

func pageBytes(t *testing.T, c Container, i int) ([]byte, string) {
	t.Helper()
	rc, typ, err := c.Page(i)
	if err != nil {
		t.Fatalf("page %d: %v", i, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("page %d read: %v", i, err)
	}
	return b, typ
}

func comicEntries() []tu.Entry {
	return []tu.Entry{
		{Name: "page10.png", Data: tu.PNG(2, 2, 10)},
		{Name: "page2.jpg", Data: []byte("jpeg-2")},
		{Name: "page1.png", Data: tu.PNG(2, 2, 1)},
		{Name: "__MACOSX/page1.png", Data: []byte("junk")},
		{Name: ".hidden.png", Data: []byte("junk")},
		{Name: "ComicInfo.xml", Data: []byte("<x/>")},
	}
}

func TestComicContainersOrderAndFilterPages(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{
		tu.WriteZip(t, dir, "a.cbz", comicEntries()...),
		tu.WriteRAR(t, dir, "a.cbr", comicEntries()...),
	} {
		c, err := Open(p, FormatOf(p), lim)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if c.Pages() != 3 {
			t.Fatalf("%s: pages = %d, want 3", p, c.Pages())
		}
		if b, typ := pageBytes(t, c, 1); string(b) != "jpeg-2" || typ != "image/jpeg" {
			t.Fatalf("%s: natural order broken: %q %s", p, b, typ)
		}
		if b, _ := pageBytes(t, c, 2); !bytes.Equal(b, tu.PNG(2, 2, 10)) {
			t.Fatalf("%s: last page wrong", p)
		}
		if _, _, err := c.Page(3); !errors.Is(err, ErrNoPage) {
			t.Fatalf("%s: out of range: %v", p, err)
		}
		if _, _, err := c.Page(-1); !errors.Is(err, ErrNoPage) {
			t.Fatalf("%s: negative: %v", p, err)
		}
		c.Close()
	}
}

func TestEntrySizeAndCountCaps(t *testing.T) {
	dir := t.TempDir()
	big := bytes.Repeat([]byte{0}, 2<<20) // compresses to almost nothing: a tiny bomb
	for _, p := range []string{
		tu.WriteZip(t, dir, "big.cbz", tu.Entry{Name: "1.png", Data: big}),
		tu.WriteRAR(t, dir, "big.cbr", tu.Entry{Name: "1.png", Data: big}),
	} {
		c, err := Open(p, FormatOf(p), lim)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.Page(0); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("%s: want ErrTooLarge, got %v", p, err)
		}
	}
	var many []tu.Entry
	for i := 0; i < 101; i++ {
		many = append(many, tu.Entry{Name: filepath.Join("p", string(rune('a'+i%26))+string(rune('a'+i/26))+".png"), Data: []byte("x")})
	}
	for _, p := range []string{tu.WriteZip(t, dir, "many.cbz", many...), tu.WriteRAR(t, dir, "many.cbr", many...)} {
		if _, err := Open(p, FormatOf(p), lim); !errors.Is(err, ErrTooMany) {
			t.Fatalf("%s: want ErrTooMany, got %v", p, err)
		}
	}
}

func TestCorruptArchiveIsAnErrorNotAPanic(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"x.cbz", "x.cbr", "x.epub"} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("not an archive at all"), 0o644)
		var err error
		if name == "x.epub" {
			_, err = OpenEPUB(p, lim)
		} else {
			_, err = Open(p, FormatOf(p), lim)
		}
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestNaturalSort(t *testing.T) {
	in := []string{"Vol 10", "vol 9", "page002", "page1", "Page10", "a", "page01"}
	sort.Slice(in, func(i, j int) bool { return NaturalLess(in[i], in[j]) })
	want := []string{"a", "page01", "page1", "page002", "Page10", "vol 9", "Vol 10"}
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("got %v", in)
		}
	}
	keys := []string{SortKey("Issue 9"), SortKey("issue 10"), SortKey("Issue 100")}
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("SortKey order: %v", keys)
	}
}

func TestPDFPageCount(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.pdf")
	os.WriteFile(p, []byte("%PDF-1.4\n1 0 obj << /Type /Pages /Kids [2 0 R] /Count 42 >> endobj\n3 0 obj << /Count 3 /Type /Pages >> endobj\n5 0 obj << /Type /Outlines /Count 7 >>"), 0o644)
	if n := PDFPageCount(p); n != 42 {
		t.Fatalf("pages = %d", n)
	}
}

func TestCacheReusesAndClosesEvicted(t *testing.T) {
	c := NewCache[Container](1)
	opened := 0
	open := func() (Container, error) { opened++; return &fakeContainer{}, nil }
	a, relA, _ := c.Get(1, open)
	_, relA2, _ := c.Get(1, open)
	if opened != 1 {
		t.Fatal("second Get should hit the cache")
	}
	c.Get(2, open) // evicts 1 while still referenced
	if a.(*fakeContainer).closed {
		t.Fatal("closed while in use")
	}
	relA()
	relA() // double release is harmless
	relA2()
	if !a.(*fakeContainer).closed {
		t.Fatal("evicted container not closed after release")
	}
}

type fakeContainer struct{ closed bool }

func (f *fakeContainer) Pages() int                              { return 0 }
func (f *fakeContainer) Page(int) (io.ReadCloser, string, error) { return nil, "", ErrNoPage }
func (f *fakeContainer) Close() error                            { f.closed = true; return nil }

// Comics are often saved with the wrong extension; the signature decides.
func TestMisnamedArchivesOpenByContent(t *testing.T) {
	dir := t.TempDir()
	zipAsCBR := tu.WriteZip(t, dir, "really-zip.cbr", comicEntries()...)
	rarAsCBZ := tu.WriteRAR(t, dir, "really-rar.cbz", comicEntries()...)
	for p, format := range map[string]string{zipAsCBR: "cbr", rarAsCBZ: "cbz"} {
		c, err := Open(p, format, lim)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(p), err)
		}
		if c.Pages() != 3 {
			t.Fatalf("%s: pages %d", filepath.Base(p), c.Pages())
		}
		c.Close()
	}
}
