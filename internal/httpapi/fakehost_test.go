package httpapi

import (
	"context"
	"sync"
)

// fakeHost stands in for the desktop app.
type fakeHost struct {
	mu      sync.Mutex
	changes []Change
	err     error
	folder  string
	ts      TailscaleStatus
}

func (h *fakeHost) Apply(_ context.Context, ch Change) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.err != nil {
		return h.err
	}
	h.changes = append(h.changes, ch)
	return nil
}

func (h *fakeHost) ChooseFolder(context.Context) (string, error) {
	if h.folder == "" {
		return "", ErrCancelled
	}
	return h.folder, nil
}

func (h *fakeHost) Tailscale(context.Context) TailscaleStatus { return h.ts }

func (h *fakeHost) last() (Change, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.changes) == 0 {
		return Change{}, false
	}
	return h.changes[len(h.changes)-1], true
}
