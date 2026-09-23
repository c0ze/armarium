package formats

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

var xml11 = regexp.MustCompile(`^(\s*<\?xml\s+version\s*=\s*["'])1\.1`)

// maxMetaBytes caps container.xml, the OPF and the nav/NCX documents.
const maxMetaBytes = 4 << 20

var ErrBadEPUB = errors.New("invalid epub")

type ManifestItem struct {
	ID, Href, MediaType, Properties string // Href is the resolved path inside the zip
}

// EPUB is an opened book. Resources are addressed by manifest index, chapters by
// spine index; names from requests are never used to look anything up.
type EPUB struct {
	f        *os.File
	files    map[string]*zip.File
	lim      Limits
	OPFPath  string
	Title    string
	Author   string
	Manifest []ManifestItem
	Spine    []int // manifest indices in reading order
	Cover    int   // manifest index of the cover image, or -1
	byPath   map[string]int
}

type opfDoc struct {
	Metadata struct {
		Titles   []string `xml:"title"`
		Creators []string `xml:"creator"`
		Metas    []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Items []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef string `xml:"idref,attr"`
	} `xml:"spine>itemref"`
}

func OpenEPUB(p string, lim Limits) (*EPUB, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	e, err := parseEPUB(f, lim)
	if err != nil {
		f.Close()
		return nil, err
	}
	return e, nil
}

func parseEPUB(f *os.File, lim Limits) (*EPUB, error) {
	zr, err := newZipReader(f, lim)
	if err != nil {
		return nil, err
	}
	e := &EPUB{f: f, lim: lim, files: map[string]*zip.File{}, byPath: map[string]int{}, Cover: -1}
	for _, zf := range zr.File {
		e.files[zf.Name] = zf
	}
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := e.decodeXML("META-INF/container.xml", &container); err == nil && len(container.Rootfiles) > 0 {
		e.OPFPath = path.Clean(container.Rootfiles[0].FullPath)
	} else if e.OPFPath = firstOPF(zr.File); e.OPFPath == "" {
		return nil, ErrBadEPUB
	}
	var opf opfDoc
	if err := e.decodeXML(e.OPFPath, &opf); err != nil {
		return nil, ErrBadEPUB
	}
	e.fill(&opf)
	return e, nil
}

// firstOPF is the fallback for books without META-INF/container.xml (some
// converters omit it): the first package document in name order.
func firstOPF(files []*zip.File) string {
	best := ""
	for _, f := range files {
		if strings.EqualFold(path.Ext(f.Name), ".opf") && (best == "" || f.Name < best) {
			best = f.Name
		}
	}
	return best
}

func (e *EPUB) fill(opf *opfDoc) {
	if len(opf.Metadata.Titles) > 0 {
		e.Title = strings.TrimSpace(opf.Metadata.Titles[0])
	}
	if len(opf.Metadata.Creators) > 0 {
		e.Author = strings.TrimSpace(opf.Metadata.Creators[0])
	}
	coverID := ""
	for _, m := range opf.Metadata.Metas {
		if m.Name == "cover" {
			coverID = m.Content
		}
	}
	ids := map[string]int{}
	dir := path.Dir(e.OPFPath)
	for _, it := range opf.Items {
		p, ok := ResolveHref(dir, it.Href)
		if !ok {
			continue
		}
		idx := len(e.Manifest)
		e.Manifest = append(e.Manifest, ManifestItem{ID: it.ID, Href: p, MediaType: it.MediaType, Properties: it.Properties})
		ids[it.ID] = idx
		e.byPath[p] = idx
		if (it.ID == coverID || hasProp(it.Properties, "cover-image")) && strings.HasPrefix(it.MediaType, "image/") {
			e.Cover = idx
		}
	}
	for _, ref := range opf.Spine {
		if idx, ok := ids[ref.IDRef]; ok && e.files[e.Manifest[idx].Href] != nil {
			e.Spine = append(e.Spine, idx)
		}
	}
}

func hasProp(props, want string) bool {
	for _, p := range strings.Fields(props) {
		if p == want {
			return true
		}
	}
	return false
}

// ResolveHref resolves a (URL-encoded, possibly fragment-bearing) href relative to
// dir into a clean zip path. It refuses anything that escapes the archive root or
// carries a scheme.
func ResolveHref(dir, href string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	p := path.Clean(path.Join(dir, u.Path))
	if strings.HasPrefix(p, "../") || p == ".." || strings.HasPrefix(p, "/") {
		return "", false
	}
	return p, true
}

// IndexOf returns the manifest index for a resolved zip path.
func (e *EPUB) IndexOf(p string) (int, bool) { i, ok := e.byPath[p]; return i, ok }

// SpineIndexOf returns the chapter number for a manifest index.
func (e *EPUB) SpineIndexOf(manifestIdx int) (int, bool) {
	for i, m := range e.Spine {
		if m == manifestIdx {
			return i, true
		}
	}
	return 0, false
}

// Entry opens manifest item idx, bounded by MaxEntryBytes.
func (e *EPUB) Entry(idx int) (io.ReadCloser, error) {
	if idx < 0 || idx >= len(e.Manifest) {
		return nil, ErrNoPage
	}
	zf := e.files[e.Manifest[idx].Href]
	if zf == nil {
		return nil, ErrNoPage
	}
	rc, _, err := openZipEntry(zf, e.lim)
	return rc, err
}

// ReadEntry reads manifest item idx fully, capped at max bytes.
func (e *EPUB) ReadEntry(idx int, max int64) ([]byte, error) {
	rc, err := e.Entry(idx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err == nil && int64(len(b)) > max {
		err = ErrTooLarge
	}
	return b, err
}

func (e *EPUB) decodeXML(name string, v any) error {
	zf := e.files[name]
	if zf == nil {
		return ErrBadEPUB
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxMetaBytes))
	if err != nil {
		return err
	}
	// encoding/xml only accepts version 1.0; some publishers declare 1.1.
	b = xml11.ReplaceAll(b, []byte(`${1}1.0`))
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d.Decode(v)
}

func (e *EPUB) Close() error { return e.f.Close() }
