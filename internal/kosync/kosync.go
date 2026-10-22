// Package kosync holds the pure parts of KOReader progress sync: KOReader's
// document id for a file and the mapping between its XPointers and chapters.
package kosync

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"regexp"
)

// Digest is KOReader's partial MD5 of a file ("Binary" matching in CrossPoint):
// 1 KiB at offset 0 and at 1024 << 2i for i = 0..10, skipping offsets past the end.
func Digest(r io.ReaderAt, size int64) (string, error) {
	h := md5.New()
	buf := make([]byte, 1024)
	for i := -1; i <= 10; i++ {
		var off int64
		if i >= 0 {
			off = 1024 << (2 * i)
		}
		if off >= size {
			continue
		}
		n, err := r.ReadAt(buf, off)
		h.Write(buf[:n])
		if err != nil && err != io.EOF {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var docID = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)

// ValidDocument reports whether s can be a document id (KOReader's are 32 hex digits).
func ValidDocument(s string) bool { return docID.MatchString(s) }

var docFragment = regexp.MustCompile(`^/body/DocFragment\[(\d+)\]`)

// ChapterOf turns a KOReader XPointer and overall percentage (0..1) into a
// 0-based chapter and the position within it (0..1, three decimals). The
// chapter comes from the XPointer's DocFragment (1-based spine index), else
// from the percentage; chapters is the spine length, 0 when unknown.
func ChapterOf(xpath string, pct float64, chapters int) (chapter int, scroll float64) {
	pct = min(max(pct, 0), 1)
	chapter = -1
	if m := docFragment.FindStringSubmatch(xpath); m != nil {
		var n int
		if _, err := fmt.Sscan(m[1], &n); err == nil && n >= 1 {
			chapter = n - 1
		}
	}
	if chapters <= 0 {
		return max(chapter, 0), 0
	}
	if chapter < 0 {
		chapter = int(pct * float64(chapters))
	}
	chapter = min(chapter, chapters-1)
	scroll = min(max(pct*float64(chapters)-float64(chapter), 0), 1)
	return chapter, math.Round(scroll*1000) / 1000
}

// XPointer is the XPointer of the start of a 0-based chapter.
func XPointer(chapter int) string {
	return fmt.Sprintf("/body/DocFragment[%d]/body", chapter+1)
}
