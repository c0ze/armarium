package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/c0ze/armarium/internal/formats"
	"github.com/c0ze/armarium/internal/store"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// fail maps an internal error to a response without leaking details.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, formats.ErrNoPage), errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, formats.ErrTooLarge), errors.Is(err, formats.ErrTooMany):
		writeError(w, http.StatusUnprocessableEntity, "file exceeds server limits")
	case errors.Is(err, errBusy):
		w.Header().Set("Retry-After", "2")
		writeError(w, http.StatusServiceUnavailable, "busy, retry shortly")
	case errors.Is(err, context.Canceled):
	default:
		// The route pattern, not the path: /opds/t/{token}/ paths carry a secret.
		s.Log.Error("request failed", "route", r.Pattern, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

var (
	errNotFound = errors.New("not found")
	errBusy     = errors.New("too many concurrent archive reads")
)

// pathID parses a positive int64 path value. Garbage and overflow are a 404.
func pathID(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return v, err == nil && v >= 0
}

func queryInt(r *http.Request, name string, def, lo, hi int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return def
	}
	return min(max(v, lo), hi)
}

func queryID(r *http.Request, name string) int64 {
	v, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// acquire takes an archive-read slot, waiting briefly before giving up.
func (s *Server) acquire(ctx context.Context) (func(), error) {
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	select {
	case s.sem <- struct{}{}:
		return func() { <-s.sem }, nil
	case <-t.C:
		return nil, errBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func decodeJSON(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}
