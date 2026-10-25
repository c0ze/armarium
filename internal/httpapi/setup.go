package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/c0ze/armarium/internal/config"
)

// NewSetupCode returns a one-time setup code: 32 random bytes, base64url.
func NewSetupCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// setupGate answers the request itself unless setup is open and it carries
// the code: 410 once a password exists, 401 (counted against the login
// limits, like a password guess) for a missing or wrong code.
func (s *Server) setupGate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, _, err := s.passwordHash(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if hash != "" || s.SetupCode == "" {
			writeError(w, http.StatusGone, "Armarium is already set up")
			return
		}
		if !s.logins.reserve(s.clientAddr(r)) {
			writeError(w, http.StatusTooManyRequests, "too many attempts, wait a minute")
			return
		}
		got := r.Header.Get("X-Armarium-Setup")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.SetupCode)) != 1 {
			writeError(w, http.StatusUnauthorized, "setup code required")
			return
		}
		s.logins.forgive(s.clientAddr(r))
		next(w, r)
	}
}

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"desktop": s.Host != nil, "managed": s.Cfg.Desktop != nil, "settings": nil}
	if s.Cfg.Desktop != nil {
		out["settings"] = s.Cfg.Settings()
	}
	if s.Host != nil {
		out["tailscale"] = s.Host.Tailscale(r.Context())
	}
	writeJSON(w, http.StatusOK, out)
}

// finishSetup applies the first settings (desktop app), then sets the owner
// password and logs the owner in. A failed apply leaves no password, so
// setup stays open.
func (s *Server) finishSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password     string           `json:"password"`
		Settings     *config.Settings `json:"settings"`
		StartAtLogin *bool            `json:"startAtLogin"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "use at least 8 characters")
		return
	}
	finish := func() {
		hash, err := hashPassword(body.Password)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.SetOwnerPassword(r.Context(), hash); err != nil {
			s.fail(w, r, err)
			return
		}
		s.startSession(w, r)
	}
	if body.Settings != nil && s.editable() {
		s.applyChange(w, r, Change{Settings: *body.Settings, StartAtLogin: body.StartAtLogin}, finish)
		return
	}
	finish()
}
