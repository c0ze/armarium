package desktop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// logFile is armarium.log, rotated to armarium.1.log … armarium.<keep>.log
// when it would grow past max bytes.
type logFile struct {
	mu   sync.Mutex
	dir  string
	max  int64
	keep int
	f    *os.File
	size int64
}

// NewLogFile opens the log in dir (5 MiB and 3 old files suit the app).
func NewLogFile(dir string, maxBytes int64, keep int) (io.WriteCloser, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &logFile{dir: dir, max: maxBytes, keep: keep}
	return l, l.open()
}

func (l *logFile) name(i int) string {
	if i == 0 {
		return filepath.Join(l.dir, "armarium.log")
	}
	return filepath.Join(l.dir, fmt.Sprintf("armarium.%d.log", i))
}

func (l *logFile) open() error {
	f, err := os.OpenFile(l.name(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, fi.Size()
	return nil
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > 0 && l.size+int64(len(p)) > l.max {
		l.f.Close()
		os.Remove(l.name(l.keep))
		for i := l.keep - 1; i >= 0; i-- {
			os.Rename(l.name(i), l.name(i+1))
		}
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

func (l *logFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
