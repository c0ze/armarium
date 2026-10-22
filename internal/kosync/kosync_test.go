package kosync

import (
	"bytes"
	"testing"
)

func TestDigest(t *testing.T) {
	big := make([]byte, 3_000_000)
	for i := range big {
		big[i] = byte(i % 251)
	}
	// Vectors from an independent implementation of KOReader's partialMD5.
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "d41d8cd98f00b204e9800998ecf8427e"},
		{"short", []byte("hello"), "5d41402abc4b2a76b9719d911017c592"},
		{"3 MB", big, "da96d9df9e3ed2a4e922f3641ba40600"},
	} {
		got, err := Digest(bytes.NewReader(c.data), int64(len(c.data)))
		if err != nil || got != c.want {
			t.Errorf("%s: Digest = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestChapterOf(t *testing.T) {
	for _, c := range []struct {
		xpath    string
		pct      float64
		chapters int
		chapter  int
		scroll   float64
	}{
		{"/body/DocFragment[3]/body/div/p[2]/text().14", 0.5, 4, 2, 0},
		{"/body/DocFragment[3]/body", 0.6, 4, 2, 0.4},
		{"/body/DocFragment[1]/body", 0.1, 4, 0, 0.4},
		{"", 0.6, 4, 2, 0.4}, // no XPointer: from the percentage
		{"", 1, 4, 3, 1},     // the very end stays in the last chapter
		{"/body/DocFragment[9]/body", 0.9, 4, 3, 0.6}, // past the spine: clamped
		{"/body/DocFragment[0]/body", 0, 4, 0, 0},
		{"/body/DocFragment[2]/body", 0.3, 0, 1, 0}, // chapter count unknown
		{"garbage", 0.3, 0, 0, 0},
	} {
		ch, sc := ChapterOf(c.xpath, c.pct, c.chapters)
		if ch != c.chapter || sc != c.scroll {
			t.Errorf("ChapterOf(%q, %v, %d) = %d, %v; want %d, %v", c.xpath, c.pct, c.chapters, ch, sc, c.chapter, c.scroll)
		}
	}
}

func TestXPointer(t *testing.T) {
	if got := XPointer(2); got != "/body/DocFragment[3]/body" {
		t.Errorf("XPointer(2) = %q", got)
	}
}

func TestValidDocument(t *testing.T) {
	for doc, want := range map[string]bool{
		"5d41402abc4b2a76b9719d911017c592": true,
		"ABCdef0123":                       true,
		"":                                 false,
		"../etc":                           false,
		string(make([]byte, 65)):           false,
	} {
		if got := ValidDocument(doc); got != want {
			t.Errorf("ValidDocument(%q) = %v", doc, got)
		}
	}
}
