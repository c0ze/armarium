// Package covers makes small JPEG thumbnails on first request and caches them on
// disk. Nothing is pre-generated, so a scan never decodes images.
package covers

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sync"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const width = 320

var ErrNoCover = errors.New("no cover")

type Service struct {
	Dir       string
	MaxPixels int64 // decompression-bomb guard, checked before decoding
	MaxBytes  int64 // largest source image read into memory

	gen   sync.Mutex // one decode at a time: a large page is tens of MiB decoded
	locks sync.Map
}

// Path returns the cached thumbnail for key (item id plus a version such as the
// file's mtime), generating it from src on a miss. src returns the source image.
func (s *Service) Path(id, version int64, src func() (io.ReadCloser, error)) (string, error) {
	p := filepath.Join(s.Dir, fmt.Sprintf("%d-%d.jpg", id, version))
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	l, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := l.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	rc, err := src()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, s.MaxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(b)) > s.MaxBytes {
		return "", ErrNoCover
	}
	thumb, err := s.thumbnail(b)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", err
	}
	old, _ := filepath.Glob(filepath.Join(s.Dir, fmt.Sprintf("%d-*.jpg", id)))
	for _, o := range old {
		os.Remove(o)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, thumb, 0o644); err != nil {
		return "", err
	}
	return p, os.Rename(tmp, p)
}

func (s *Service) thumbnail(b []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNoCover // AVIF, JXL and broken images get no thumbnail
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > s.MaxPixels {
		return nil, ErrNoCover
	}
	s.gen.Lock()
	defer s.gen.Unlock()
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNoCover
	}
	w, h := cfg.Width, cfg.Height
	if w > width {
		h = h * width / w
		w = width
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, max(h, 1)))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
