package covers

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"os"
	"testing"

	tu "github.com/c0ze/armarium/internal/testutil"
)

func src(b []byte) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
}

func TestThumbnailIsCachedAndScaled(t *testing.T) {
	s := &Service{Dir: t.TempDir(), MaxPixels: 1 << 24, MaxBytes: 1 << 20}
	p, err := s.Path(7, 100, src(tu.PNG(640, 960, 50)))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width != 320 || cfg.Height != 480 {
		t.Fatalf("thumb %dx%d %v", cfg.Width, cfg.Height, err)
	}
	calls := 0
	again, _ := s.Path(7, 100, func() (io.ReadCloser, error) { calls++; return nil, io.EOF })
	if again != p || calls != 0 {
		t.Fatal("second request should hit the disk cache")
	}
	p2, _ := s.Path(7, 101, src(tu.PNG(10, 10, 1)))
	if _, err := os.Stat(p); !os.IsNotExist(err) || p2 == p {
		t.Fatal("new version should replace the old thumbnail")
	}
}

func TestDecompressionBombIsRejectedBeforeDecode(t *testing.T) {
	// A PNG header claiming 100000x100000 pixels; the body is never decoded.
	png := tu.PNG(1, 1, 0)
	binary.BigEndian.PutUint32(png[16:], 100000)
	binary.BigEndian.PutUint32(png[20:], 100000)
	binary.BigEndian.PutUint32(png[29:], crc32.ChecksumIEEE(png[12:29])) // keep IHDR valid
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(png)); err != nil || cfg.Width != 100000 {
		t.Fatalf("fixture header not accepted: %v", err)
	}
	s := &Service{Dir: t.TempDir(), MaxPixels: 1 << 24, MaxBytes: 1 << 20}
	if _, err := s.Path(1, 1, src(png)); err != ErrNoCover {
		t.Fatalf("want ErrNoCover, got %v", err)
	}
	if _, err := s.Path(2, 1, src([]byte("not an image"))); err != ErrNoCover {
		t.Fatalf("garbage: %v", err)
	}
}
