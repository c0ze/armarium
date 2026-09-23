package formats

import (
	"archive/zip"
	"io"
	"os"
	"sort"
)

type zipContainer struct {
	f     *os.File
	pages []*zip.File
	lim   Limits
}

func openZip(p string, lim Limits) (Container, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	zr, err := newZipReader(f, lim)
	if err != nil {
		f.Close()
		return nil, err
	}
	c := &zipContainer{f: f, lim: lim}
	for _, zf := range zr.File {
		if !zf.FileInfo().IsDir() && isPage(zf.Name) {
			c.pages = append(c.pages, zf)
		}
	}
	sort.SliceStable(c.pages, func(i, j int) bool { return NaturalLess(c.pages[i].Name, c.pages[j].Name) })
	return c, nil
}

func newZipReader(f *os.File, lim Limits) (*zip.Reader, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, err
	}
	if lim.MaxEntries > 0 && len(zr.File) > lim.MaxEntries {
		return nil, ErrTooMany
	}
	return zr, nil
}

func (c *zipContainer) Pages() int { return len(c.pages) }

func (c *zipContainer) Page(i int) (io.ReadCloser, string, error) {
	if i < 0 || i >= len(c.pages) {
		return nil, "", ErrNoPage
	}
	return openZipEntry(c.pages[i], c.lim)
}

func openZipEntry(zf *zip.File, lim Limits) (io.ReadCloser, string, error) {
	if lim.MaxEntryBytes > 0 && zf.UncompressedSize64 > uint64(lim.MaxEntryBytes) {
		return nil, "", ErrTooLarge
	}
	rc, err := zf.Open()
	if err != nil {
		return nil, "", err
	}
	if lim.MaxEntryBytes > 0 {
		rc = &capped{r: rc, n: lim.MaxEntryBytes}
	}
	return rc, ImageType(zf.Name), nil
}

func (c *zipContainer) Close() error { return c.f.Close() }
