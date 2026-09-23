package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/scan"
)

const sessionTTL = 30 * 24 * time.Hour

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": p != Anonymous,
		"admin":         p == Session,
		"passwordSet":   s.Cfg.PasswordHash != "",
		"panelflowUrl":  s.Cfg.PanelFlowURL,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if s.Cfg.PasswordHash == "" {
		writeError(w, http.StatusForbidden, "no password configured; run `armarium hash-password`")
		return
	}
	if !s.logins.reserve() {
		writeError(w, http.StatusTooManyRequests, "too many attempts, wait a minute")
		return
	}
	ok, err := s.logins.check(s.Cfg.PasswordHash, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	secret := auth.NewSecret()
	if err := s.Store.AddSession(r.Context(), auth.Digest(secret), sessionTTL); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: secret, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Cfg.SecureCookie,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true, "admin": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Store.DeleteSession(r.Context(), auth.Digest(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Cfg.SecureCookie})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

// loginLimiter allows 5 login attempts per minute across all clients, counted
// before the password is checked so parallel requests cannot all slip through.
// There is one password, so a global limit is the simplest brute-force brake.
// Checks also run one at a time: each argon2id check takes 64 MiB, and the NAS
// container has 256 MiB in total.
type loginLimiter struct {
	mu       sync.Mutex
	attempts []time.Time
	checking sync.Mutex
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{} }

// reserve counts an attempt, or refuses it when the minute's budget is spent.
func (l *loginLimiter) reserve() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-time.Minute)
	for len(l.attempts) > 0 && l.attempts[0].Before(cut) {
		l.attempts = l.attempts[1:]
	}
	if len(l.attempts) >= 5 {
		return false
	}
	l.attempts = append(l.attempts, time.Now())
	return true
}

func (l *loginLimiter) check(hash, password string) (bool, error) {
	l.checking.Lock()
	defer l.checking.Unlock()
	return auth.CheckPassword(hash, password)
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.Store.Tokens(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// createToken returns the plaintext exactly once; only its digest is stored.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" || len(body.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is required (max 64 chars)")
		return
	}
	secret := auth.NewSecret()
	t, err := s.Store.AddToken(r.Context(), strings.TrimSpace(body.Name), auth.Digest(secret))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a token with that name exists")
			return
		}
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.Store.DeleteToken(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Scanner.Status())
}

// postScan starts a scan in the background and returns at once.
func (s *Server) postScan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Library string `json:"library"`
	}
	if err := decodeJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if s.Scanner.Status().Running {
		writeError(w, http.StatusConflict, scan.ErrRunning.Error())
		return
	}
	go func() {
		if err := s.Scanner.Run(context.Background(), body.Library); err != nil && !errors.Is(err, scan.ErrRunning) {
			s.Log.Error("scan", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}
