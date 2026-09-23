package formats

import (
	"bytes"
	"io"
	"io/fs"
	"sort"
	"sync"

	"github.com/nwaples/rardecode/v2"
)

// maxRarDict caps the decoder dictionary. RAR5 lets an archive ask for up to
// 64 GiB; comics use 4–32 MiB (WinRAR's RAR5 default is 32 MiB), and the NAS
// container has 256 MiB in total.
const maxRarDict = 32 << 20

// rarContainer uses rardecode's RarFS, which indexes file header offsets once so
// page N seeks straight to its entry in non-solid archives.
type rarContainer struct {
	mu    sync.Mutex // RarFS shares volume state between opens
	rfs   *rardecode.RarFS
	pages []string
	lim   Limits
}

func openRar(p string, lim Limits) (Container, error) {
	rfs, err := rardecode.OpenFS(p, rardecode.MaxDictionarySize(maxRarDict))
	if err != nil {
		return nil, err
	}
	c := &rarContainer{rfs: rfs, lim: lim}
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
	sort.SliceStable(c.pages, func(i, j int) bool { return NaturalLess(c.pages[i], c.pages[j]) })
	return c, nil
}

func (c *rarContainer) Pages() int { return len(c.pages) }

// Page reads the whole entry under the lock (pages are a few MiB, capped by
// MaxEntryBytes) so concurrent requests never interleave on the shared decoder.
func (c *rarContainer) Page(i int) (io.ReadCloser, string, error) {
	if i < 0 || i >= len(c.pages) {
		return nil, "", ErrNoPage
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.rfs.Open(c.pages[i])
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

func (c *rarContainer) Close() error { return nil }
