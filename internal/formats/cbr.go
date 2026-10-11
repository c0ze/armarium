package formats

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/nwaples/rardecode/v2"
)

// maxRarDict caps the decoder dictionary. RAR5 lets an archive ask for up to
// 64 GiB; comics use 4–32 MiB (WinRAR's RAR5 default is 32 MiB), and the NAS
// container has 256 MiB in total.
const maxRarDict = 32 << 20

// rarContainer uses rardecode's RarFS, which indexes file header offsets once so
// page N seeks straight to its entry in non-solid archives. Entries of a solid
// archive (all but the first) are read with a sequential reader instead.
type rarContainer struct {
	mu    sync.Mutex // RarFS shares volume state between opens
	path  string
	rfs   *rardecode.RarFS
	seq   *rardecode.ReadCloser // solid archives: kept after the last entry read
	pages []string
	lim   Limits
}

func openRar(p string, lim Limits) (Container, error) {
	rfs, err := rardecode.OpenFS(p, rardecode.MaxDictionarySize(maxRarDict))
	if err != nil {
		return nil, err
	}
	c := &rarContainer{path: p, rfs: rfs, lim: lim}
	n := 0
	err = fs.WalkDir(rfs, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if n++; lim.MaxEntries > 0 && n > lim.MaxEntries {
			return ErrTooMany
		}
		if !d.IsDir() && isPage(name) {
			c.pages = append(c.pages, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(c.pages, func(i, j int) bool { return PageLess(c.pages[i], c.pages[j]) })
	return c, nil
}

func (c *rarContainer) Pages() int { return len(c.pages) }

func (c *rarContainer) Name(i int) string { return c.pages[i] }

// Page reads the whole entry under the lock (pages are a few MiB, capped by
// MaxEntryBytes) so concurrent requests never interleave on the shared decoder.
func (c *rarContainer) Page(i int) (io.ReadCloser, string, error) {
	if i < 0 || i >= len(c.pages) {
		return nil, "", ErrNoPage
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.rfs.Open(c.pages[i])
	if errors.Is(err, rardecode.ErrSolidOpen) {
		b, err := c.solidEntry(c.pages[i])
		if err != nil {
			return nil, "", err
		}
		return io.NopCloser(bytes.NewReader(b)), ImageType(c.pages[i]), nil
	}
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if c.lim.MaxEntryBytes > 0 && st.Size() > c.lim.MaxEntryBytes {
		return nil, "", ErrTooLarge
	}
	var r io.Reader = f
	if c.lim.MaxEntryBytes > 0 {
		r = io.LimitReader(f, c.lim.MaxEntryBytes+1)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	if c.lim.MaxEntryBytes > 0 && int64(len(b)) > c.lim.MaxEntryBytes {
		return nil, "", ErrTooLarge
	}
	return io.NopCloser(bytes.NewReader(b)), ImageType(c.pages[i]), nil
}

// solidEntry reads one entry of a solid archive. Its data depends on every
// entry before it, so it comes from a sequential reader kept open after the
// last entry read: turning pages forwards decodes each entry once, and a page
// behind the reader starts it over. Next decodes and discards the entries in
// between (rardecode keeps the dictionary).
func (c *rarContainer) solidEntry(name string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if c.seq == nil {
			rc, err := rardecode.OpenReader(c.path, rardecode.MaxDictionarySize(maxRarDict))
			if err != nil {
				return nil, err
			}
			c.seq = rc
		}
		for {
			h, err := c.seq.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				c.closeSeq()
				return nil, err
			}
			if strings.TrimPrefix(path.Clean(h.Name), "/") != name {
				continue
			}
			limit := c.lim.MaxEntryBytes
			if limit <= 0 {
				limit = math.MaxInt64 - 1
			}
			if h.UnPackedSize > limit {
				return nil, ErrTooLarge
			}
			b, err := io.ReadAll(io.LimitReader(c.seq, limit+1))
			if err != nil {
				c.closeSeq()
				return nil, err
			}
			if int64(len(b)) > limit {
				return nil, ErrTooLarge
			}
			return b, nil
		}
		c.closeSeq() // the page is behind the reader: start over once
	}
	return nil, ErrNoPage
}

func (c *rarContainer) closeSeq() error {
	if c.seq == nil {
		return nil
	}
	err := c.seq.Close()
	c.seq = nil
	return err
}

func (c *rarContainer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeSeq()
}
