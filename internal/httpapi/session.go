package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/c0ze/armarium/internal/auth"
	"github.com/c0ze/armarium/internal/scan"
)

const sessionTTL = 30 * 24 * time.Hour

// passwordHash is the owner password in force: password_hash from the config
// wins over the one set from the browser or `armarium password`.
func (s *Server) passwordHash(ctx context.Context) (hash string, fromConfig bool, err error) {
	if s.Cfg.PasswordHash != "" {
		return s.Cfg.PasswordHash, true, nil
	}
	hash, err = s.Store.OwnerPassword(ctx)
	return hash, false, err
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	hash, fromConfig, err := s.passwordHash(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := principal(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":      p != Anonymous,
		"admin":              p == Session,
		"passwordSet":        hash != "",
		"passwordFromConfig": fromConfig,
		"setupOpen":          hash == "",
		"panelflowUrl":       s.Cfg.PanelFlowURL,
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
	hash, _, err := s.passwordHash(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if hash == "" {
		writeError(w, http.StatusForbidden, "no password set yet; finish setup, or run `armarium password`")
		return
	}
	if !s.checkPassword(w, r, hash, body.Password) {
		return
	}
	s.startSession(w, r)
}

// checkPassword counts the attempt against the login limits, then checks it.
// It has answered the request when it returns false.
func (s *Server) checkPassword(w http.ResponseWriter, r *http.Request, hash, password string) bool {
	if !s.logins.reserve(s.clientAddr(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, wait a minute")
		return false
	}
	ok, err := s.logins.check(hash, password)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "wrong password")
	}
	return ok
}

// startSession logs the owner in with a new session cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	secret := auth.NewSecret()
	if err := s.Store.AddSession(r.Context(), auth.Digest(secret), sessionTTL); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: secret, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Cfg.SecureCookie || s.isHTTPS(r),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true, "admin": true})
}

const minPasswordLen = 8

var hashPassword = auth.HashPassword

// changePassword replaces the stored owner password. Every session ends; this
// one continues with a new cookie.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	hash, fromConfig, err := s.passwordHash(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if fromConfig {
		writeError(w, http.StatusConflict, "password_hash is set in armarium.toml")
		return
	}
	if len(body.New) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "use at least 8 characters")
		return
	}
	if !s.checkPassword(w, r, hash, body.Current) {
		return
	}
	newHash, err := hashPassword(body.New)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetOwnerPassword(r.Context(), newHash); err != nil {
		s.fail(w, r, err)
		return
	}
	s.startSession(w, r)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Store.DeleteSession(r.Context(), auth.Digest(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Cfg.SecureCookie || s.isHTTPS(r)})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

// loginLimiter allows 5 login attempts per minute from each client, and 30 a
// minute in all, counted before the password is checked so parallel requests
// cannot all slip through. Per client, so someone hammering the login cannot
// keep the owner out from another address; the ceiling still caps a spread-out
// guess. Checks also run one at a time: each argon2id check takes 64 MiB, and
// the NAS container has 256 MiB in total.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[netip.Addr][]time.Time
	all      []time.Time
	checking sync.Mutex
}

const (
	loginsPerClient = 5
	loginsInAll     = 30
)

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[netip.Addr][]time.Time{}} }

// recent drops attempts older than a minute.
func recent(times []time.Time, cut time.Time) []time.Time {
	for len(times) > 0 && times[0].Before(cut) {
		times = times[1:]
	}
	return times
}

// reserve counts an attempt from client, or refuses it when its minute's budget
// (or everyone's) is spent.
func (l *loginLimiter) reserve(client netip.Addr) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-time.Minute)
	for a, times := range l.attempts {
		if times = recent(times, cut); len(times) == 0 {
			delete(l.attempts, a)
		} else {
			l.attempts[a] = times
		}
	}
	l.all = recent(l.all, cut)
	if len(l.attempts[client]) >= loginsPerClient || len(l.all) >= loginsInAll {
		return false
	}
	l.attempts[client] = append(l.attempts[client], now)
	l.all = append(l.all, now)
	return true
}

// forgive takes back client's latest attempt: a right setup code is not a guess.
func (l *loginLimiter) forgive(client netip.Addr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.attempts[client]; len(t) > 0 {
		l.attempts[client] = t[:len(t)-1]
	}
	if len(l.all) > 0 {
		l.all = l.all[:len(l.all)-1]
	}
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
