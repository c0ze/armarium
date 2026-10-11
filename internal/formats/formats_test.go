package formats

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nwaples/rardecode/v2"

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
func (f *fakeContainer) Name(int) string                         { return "" }

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

// A cover at the archive root comes before the chapter folders, in ZIP and RAR.
func TestRootPagesComeFirst(t *testing.T) {
	dir := t.TempDir()
	cover := tu.PNG(4, 4, 200)
	entries := []tu.Entry{{Name: "chapter1/01.png", Data: tu.PNG(4, 4, 10)}, {Name: "chapter1/02.png", Data: tu.PNG(4, 4, 20)}, {Name: "cover.png", Data: cover}}
	for _, p := range []string{tu.WriteZip(t, dir, "n.cbz", entries...), tu.WriteRAR(t, dir, "n.cbr", entries...)} {
		c, err := Open(p, strings.TrimPrefix(filepath.Ext(p), "."), lim)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		names := []string{c.Name(0), c.Name(1), c.Name(2)}
		if want := []string{"cover.png", "chapter1/01.png", "chapter1/02.png"}; !slices.Equal(names, want) {
			t.Fatalf("%s: %v, want %v", filepath.Base(p), names, want)
		}
		rc, _, err := c.Page(0)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(b, cover) {
			t.Fatalf("%s: page 1 is not the cover", filepath.Base(p))
		}
		c.Close()
	}
}

// Every entry of a solid archive after the first depends on those before it,
// so rardecode cannot open one on its own: such pages are read sequentially.
func TestSolidRARPages(t *testing.T) {
	p := tu.WriteSolidRAR(t, t.TempDir(), "solid.cbr", comicEntries()...)
	rfs, err := rardecode.OpenFS(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rfs.Open("page1.png"); !errors.Is(err, rardecode.ErrSolidOpen) {
		t.Fatalf("the fixture must be solid for rardecode: %v", err)
	}
	c, err := Open(p, "cbr", lim)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	want := map[int][]byte{0: tu.PNG(2, 2, 1), 1: []byte("jpeg-2"), 2: tu.PNG(2, 2, 10)}
	// Forwards, then a page behind the reader, which starts it over.
	for _, i := range []int{1, 0, 2, 1} {
		if b, _ := pageBytes(t, c, i); !bytes.Equal(b, want[i]) {
			t.Fatalf("page %d: %q", i+1, b)
		}
	}
}

// Each solid decoder holds a dictionary of up to maxRarDict, and the comics
// cache keeps several archives open: only the most recent few stay live.
func TestSolidDecodersStayFewAcrossComics(t *testing.T) {
	dir := t.TempDir()
	var open []*rarContainer
	for i := range maxLiveSolid + 1 {
		c, err := Open(tu.WriteSolidRAR(t, dir, fmt.Sprintf("solid%d.cbr", i), comicEntries()...), "cbr", lim)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		pageBytes(t, c, 1) // a solid entry: needs a sequential decoder
		open = append(open, c.(*rarContainer))
	}
	live := func(c *rarContainer) bool { c.mu.Lock(); defer c.mu.Unlock(); return c.seq != nil }
	if live(open[0]) {
		t.Fatal("the least recently used decoder must be closed")
	}
	for i, c := range open[1:] {
		if !live(c) {
			t.Fatalf("decoder %d closed too early", i+1)
		}
	}
	// The first comic read again starts over, and the next least recently used closes.
	// (Page 1 is the archive's third entry; its first, page10, needs no decoder.)
	if b, _ := pageBytes(t, open[0], 0); !bytes.Equal(b, tu.PNG(2, 2, 1)) {
		t.Fatalf("page 1 after a restart: %q", b)
	}
	if live(open[1]) || !live(open[0]) {
		t.Fatal("the cap must follow the most recent reads")
	}
}

// A solid entry over the size limit fails, but its decoder still counts toward the cap.
func TestOversizedSolidEntriesStayUnderTheDecoderCap(t *testing.T) {
	dir := t.TempDir()
	small := Limits{MaxEntryBytes: 4, MaxEntries: 100}
	var open []*rarContainer
	for i := range maxLiveSolid + 1 {
		c, err := Open(tu.WriteSolidRAR(t, dir, fmt.Sprintf("big%d.cbr", i), comicEntries()...), "cbr", small)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if _, _, err := c.Page(0); !errors.Is(err, ErrTooLarge) { // page1.png, a solid entry
			t.Fatalf("page 1: %v, want ErrTooLarge", err)
		}
		open = append(open, c.(*rarContainer))
	}
	live := 0
	for _, c := range open {
		c.mu.Lock()
		if c.seq != nil {
			live++
		}
		c.mu.Unlock()
	}
	if live > maxLiveSolid {
		t.Fatalf("%d solid decoders open, cap %d", live, maxLiveSolid)
	}
}

// Requests still reading cannot be evicted, so the request that finishes last trims
// the decoders back to the cap: none stays above it once reads are done.
func TestSolidDecoderCapHoldsAfterConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	var open []*rarContainer
	for i := range maxLiveSolid + 1 {
		c, err := Open(tu.WriteSolidRAR(t, dir, fmt.Sprintf("busy%d.cbr", i), comicEntries()...), "cbr", lim)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		open = append(open, c.(*rarContainer))
	}
	a, b, c := open[0], open[1], open[2]
	pageBytes(t, a, 1)
	pageBytes(t, b, 1)
	// a and b are still inside their reads (holding their locks) when c reads.
	a.mu.Lock()
	b.mu.Lock()
	pageBytes(t, c, 1)
	a.mu.Unlock()
	b.mu.Unlock()
	live := 0
	for _, x := range open {
		x.mu.Lock()
		if x.seq != nil {
			live++
		}
		x.mu.Unlock()
	}
	if live > maxLiveSolid {
		t.Fatalf("%d solid decoders open after the reads, cap %d", live, maxLiveSolid)
	}
}
