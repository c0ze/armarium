package scan

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/c0ze/armarium/internal/formats"
)

// walk finds library files on disk. It walks the resolved root: WalkDir does not
// follow a symlinked root, which would otherwise find nothing and mark the whole
// library missing.
func (s *Scanner) walk(ctx context.Context, realRoot string, include []string, visit func(found)) ([]string, error) {
	var unreadable []string
	err := filepath.WalkDir(realRoot, func(p string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(realRoot, p)
		rel = filepath.ToSlash(rel)
		if err != nil {
			if rel == "." {
				return err
			}
			s.Log.Warn("scan: skipping unreadable path", "path", p, "err", err)
			unreadable = append(unreadable, rel)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if rel != "." && !included(include, rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || nasDirs[d.Name()]) {
				return fs.SkipDir
			}
			if s.DirSleep > 0 {
				time.Sleep(s.DirSleep)
			}
			return nil
		}
		if formats.FormatOf(d.Name()) == "" || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 && !insideRoot(realRoot, p) {
			return nil
		}
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			visit(found{rel: rel, size: info.Size(), mt: info.ModTime().Unix(), abs: p})
		}
		return nil
	})
	return unreadable, err
}

// nasDirs are thumbnail, recycle-bin and snapshot folders that NAS systems add
// inside shared folders (Synology, QNAP). Walking them costs I/O for nothing.
var nasDirs = map[string]bool{"@eaDir": true, "#recycle": true, "#snapshot": true, "@Recycle": true, ".@__thumb": true}

// under reports whether rel is one of dirs or inside one of them.
func under(dirs []string, rel string) bool {
	for _, d := range dirs {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// included applies the library's top-level include globs to a relative path.
func included(globs []string, rel string) bool {
	if len(globs) == 0 {
		return true
	}
	top, _, _ := strings.Cut(rel, "/")
	for _, g := range globs {
		if ok, _ := path.Match(g, top); ok {
			return true
		}
	}
	return false
}

// insideRoot reports whether a symlink's target stays under the library root.
func insideRoot(realRoot, p string) bool {
	target, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// retryable reports whether a file's format can yield pages when reopened.
func retryable(rel string) bool {
	switch formats.FormatOf(rel) {
	case "cbz", "cbr", "epub":
		return true
	}
	return false
}
