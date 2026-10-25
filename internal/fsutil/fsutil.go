// Package fsutil holds the path checks that keep reads inside a library, on
// every platform: Windows paths escape with `..\` and drive letters, not just
// `../`.
package fsutil

import (
	"path"
	"path/filepath"
	"strings"
)

// Inside reports whether rel, a result of filepath.Rel(root, p), stays under
// root.
func Inside(rel string) bool {
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel) && filepath.VolumeName(rel) == ""
}

// SafeRelSlash reports whether p, a relative path from a library's metadata
// (Calibre's metadata.db), names something under the library on any
// platform: no "..", no root, no drive letter or UNC share, with either slash.
func SafeRelSlash(p string) bool {
	if p == "" || strings.Contains(p, ":") || strings.HasPrefix(p, `\\`) {
		return false
	}
	c := path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return c != "." && c != ".." && !strings.HasPrefix(c, "../") && !path.IsAbs(c)
}
