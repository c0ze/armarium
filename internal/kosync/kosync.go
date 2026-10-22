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
// from the percentage. sizes weighs the chapters the way CrossPoint does (their
// inflated sizes; all zero means equal weights); empty when unknown.
func ChapterOf(xpath string, pct float64, sizes []int64) (chapter int, scroll float64) {
	pct = min(max(pct, 0), 1)
	chapter = -1
	if m := docFragment.FindStringSubmatch(xpath); m != nil {
		var n int
		if _, err := fmt.Sscan(m[1], &n); err == nil && n >= 1 {
			chapter = n - 1
		}
	}
	if len(sizes) == 0 {
		return max(chapter, 0), 0
	}
	w, cum := weights(sizes)
	target := pct * cum[len(w)]
	if chapter < 0 {
		chapter = len(w) - 1
		for i := range w {
			if cum[i+1] > target {
				chapter = i
				break
			}
		}
	}
	chapter = min(chapter, len(w)-1)
	if w[chapter] > 0 {
		scroll = min(max((target-cum[chapter])/w[chapter], 0), 1)
	}
	return chapter, math.Round(scroll*1000) / 1000
}

// Percentage is the overall position (0..1) of a place in a chapter, weighed
// like ChapterOf; 0 when sizes is empty.
func Percentage(chapter int, scroll float64, sizes []int64) float64 {
	if len(sizes) == 0 {
		return 0
	}
	w, cum := weights(sizes)
	chapter = min(max(chapter, 0), len(w)-1)
	return min((cum[chapter]+min(max(scroll, 0), 1)*w[chapter])/cum[len(w)], 1)
}

// weights returns the chapter weights (equal when sizes are all zero) and
// their running totals, cum[i] being the weight before chapter i.
func weights(sizes []int64) (w, cum []float64) {
	var total int64
	for _, n := range sizes {
		total += max(n, 0)
	}
	w, cum = make([]float64, len(sizes)), make([]float64, len(sizes)+1)
	for i, n := range sizes {
		w[i] = 1
		if total > 0 {
			w[i] = float64(max(n, 0))
		}
		cum[i+1] = cum[i] + w[i]
	}
	return w, cum
}

// PageFor is Armarium's progress page for a place in a book, the web reader's
// rule (progressFor in web/src/lib/reading.ts): chapter+1, except that the last
// chapter is page chapters-1 until it is scrolled past 95%, when the book is
// read. chapters 0 (unknown) gives chapter+1, never read.
func PageFor(chapter, chapters int, scroll float64) (page int, read bool) {
	if chapters <= 0 || chapter < chapters-1 {
		return chapter + 1, false
	}
	if scroll > 0.95 {
		return chapters, true
	}
	return chapters - 1, false
}

// XPointer is the XPointer of the start of a 0-based chapter.
func XPointer(chapter int) string {
	return fmt.Sprintf("/body/DocFragment[%d]/body", chapter+1)
}
