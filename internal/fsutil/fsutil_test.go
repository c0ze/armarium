package fsutil

import (
	"path/filepath"
	"testing"
)

func TestInside(t *testing.T) {
	for rel, want := range map[string]bool{
		".": true, "a": true, "a/b": true, "..a": true, "a/..b": true,
		"..": false, "../x": false, "../../etc/passwd": false,
	} {
		if got := Inside(filepath.FromSlash(rel)); got != want {
			t.Errorf("Inside(%q) = %v, want %v", filepath.FromSlash(rel), got, want)
		}
	}
	if filepath.Separator == '\\' {
		for _, rel := range []string{`..\x`, `C:\Windows`, `\\server\share`} {
			if Inside(rel) {
				t.Errorf("Inside(%q) on Windows", rel)
			}
		}
	}
}

func TestSafeRelSlash(t *testing.T) {
	for p, want := range map[string]bool{
		"Author/Title (1)": true, "a/b.epub": true,
		"": false, ".": false, "..": false, "../x": false, "/abs": false,
		`..\..\Windows`: false, `a\..\..\x`: false, `C:\Windows`: false, `C:/x`: false, `\\server\share`: false,
	} {
		if got := SafeRelSlash(p); got != want {
			t.Errorf("SafeRelSlash(%q) = %v, want %v", p, got, want)
		}
	}
}
