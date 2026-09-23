// Package scan walks library roots and records comics and books in the store.
// Scans are incremental (size + mtime), throttled, and never delete on the spot:
// a file that disappears is only marked missing.
package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/c0ze/armarium/internal/config"
	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
)

const (
	batchSize   = 50
	missingKeep = 30 * 24 * time.Hour
)

type Status struct {
	Running    bool   `json:"running"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
	Added      int    `json:"added"`
	Updated    int    `json:"updated"`
	Missing    int    `json:"missing"`
	Errors     int    `json:"errors"`
	LastError  string `json:"lastError"`
}

type Scanner struct {
	Store     *store.Store
	Libraries []config.Library
	Limits    formats.Limits
	Workers   int
	DirSleep  time.Duration
	Log       *slog.Logger
	OnChange  func(itemID int64) // called for changed or vanished items (cache invalidation)

	mu     sync.Mutex
	status Status
}

var ErrRunning = errors.New("a scan is already running")

func (s *Scanner) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Run scans every library, or only the one named. It refuses to run twice at once.
func (s *Scanner) Run(ctx context.Context, only string) error {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return ErrRunning
	}
	s.status = Status{Running: true, StartedAt: time.Now().Unix()}
	s.mu.Unlock()

	var firstErr error
	libs, err := s.Store.Libraries(ctx)
	if err == nil {
		for _, lib := range libs {
			if only != "" && lib.Name != only {
				continue
			}
			if err := s.library(ctx, lib); err != nil {
				s.Log.Error("scan failed", "library", lib.Name, "err", err)
				s.bump(func(st *Status) { st.Errors++; st.LastError = err.Error() })
				firstErr = cmpOr(firstErr, err)
			}
		}
		if _, err := s.Store.Prune(ctx, missingKeep); err != nil {
			firstErr = cmpOr(firstErr, err)
		}
	} else {
		firstErr = err
	}
	s.bump(func(st *Status) { st.Running = false; st.FinishedAt = time.Now().Unix() })
	st := s.Status()
	s.Log.Info("scan finished", "added", st.Added, "updated", st.Updated, "missing", st.Missing, "errors", st.Errors)
	return firstErr
}

func cmpOr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func (s *Scanner) bump(f func(*Status)) {
	s.mu.Lock()
	f(&s.status)
	s.mu.Unlock()
}

func (s *Scanner) config(name string) config.Library {
	for _, l := range s.Libraries {
		if l.Name == name {
			return l
		}
	}
	return config.Library{}
}

type found struct {
	rel  string
	size int64
	mt   int64
	abs  string
	meta *store.ScannedItem // set for Calibre books: metadata comes from metadata.db
}

func (s *Scanner) library(ctx context.Context, lib store.Library) error {
	cfg := s.config(lib.Name)
	// A missing or unmounted root must not mark the whole library missing.
	if st, err := os.Stat(lib.Root); err != nil || !st.IsDir() {
		return fmt.Errorf("library root %s unavailable: %v", lib.Root, err)
	}
	realRoot, err := filepath.EvalSymlinks(lib.Root)
	if err != nil {
		return err
	}
	labels, err := LoadLabels(cfg.Labels)
	if err != nil {
		return err
	}
	known, err := s.Store.KnownItems(ctx, lib.ID)
	if err != nil {
		return err
	}
	w := &writer{s: s, ctx: ctx, lib: lib, labels: labels}
	defer w.close()

	jobs := make(chan found)
	var wg sync.WaitGroup
	for range max(s.Workers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				w.add(s.describe(lib, labels, f), known[f.rel].ID != 0)
			}
		}()
	}
	seen := map[string]bool{}
	// visit records one file found by the walk or listed in metadata.db. Only new
	// or changed files are queued to be opened.
	visit := func(f found) {
		seen[f.rel] = true
		k, ok := known[f.rel]
		// A readable file stuck at 0 pages failed to open last time; retry it so
		// reader fixes (misnamed archives, odd EPUBs) reach existing entries.
		retry := k.Pages == 0 && retryable(f.rel)
		if ok && k.Size == f.size && k.MTime == f.mt && !retry {
			if f.meta != nil { // Calibre metadata can change while the file does not
				it := *f.meta
				it.Pages = k.Pages
				w.refresh(it)
			} else if k.Missing {
				w.seen(k.ID)
			}
			return
		}
		jobs <- f
		if ok && s.OnChange != nil {
			s.OnChange(k.ID)
		}
	}
	var unreadable []string // dirs we could not list: their items are not missing
	if cfg.Calibre {
		err = s.calibreFiles(ctx, realRoot, visit)
	} else {
		unreadable, err = s.walk(ctx, realRoot, cfg.Include, visit)
	}
	close(jobs)
	wg.Wait()
	if err != nil {
		return err
	}
	for rel, k := range known {
		if !seen[rel] && !k.Missing && !under(unreadable, rel) {
			w.missing(k.ID)
			if s.OnChange != nil {
				s.OnChange(k.ID)
			}
		}
	}
	w.close()
	if w.err != nil {
		return w.err
	}
	return s.applyTags(ctx, lib, cfg.Tags)
}

// applyTags replaces the tags of every item listed in the sidecar. It touches the
// database only, so running it on every scan is cheap.
func (s *Scanner) applyTags(ctx context.Context, lib store.Library, sidecar string) error {
	if sidecar == "" {
		return nil
	}
	b, err := os.ReadFile(sidecar)
	if err != nil {
		return fmt.Errorf("tags sidecar: %w", err)
	}
	var tags map[string][]string
	if err := json.Unmarshal(b, &tags); err != nil {
		return fmt.Errorf("tags sidecar %s: %w", sidecar, err)
	}
	known, err := s.Store.KnownItems(ctx, lib.ID)
	if err != nil {
		return err
	}
	batch, err := s.Store.Begin(ctx, lib.ID)
	if err != nil {
		return err
	}
	defer batch.Rollback()
	for rel, ts := range tags {
		if k, ok := known[rel]; ok {
			if err := batch.SetTags(ctx, k.ID, ts); err != nil {
				return err
			}
		}
	}
	return batch.Commit()
}
