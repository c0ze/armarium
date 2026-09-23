package httpapi

import (
	"context"
	"encoding/base64"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/c0ze/armarium/internal/auth"
)

const (
	sessionCookie = "armarium_session"
	maxBodyBytes  = 64 << 10
)

// Principal says how a request authenticated.
type Principal int

const (
	Anonymous Principal = iota
	Session             // the owner, via the UI cookie: may administer
	Token               // an API token via Bearer, ?token= or an /opds/t/{token}/ path
	Basic               // an API token as the HTTP Basic password (OPDS clients)
)

type ctxKey struct{}

func principal(r *http.Request) Principal {
	p, _ := r.Context().Value(ctxKey{}).(Principal)
	return p
}

// base applies limits and baseline headers, and turns panics into a bare 500.
func (s *Server) base(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "route", r.Pattern, "err", v)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		next.ServeHTTP(w, r)
	})
}

// hostAllowed is the DNS-rebinding defence: an entry with a port must match
// exactly, an entry without one matches that host on any port.
func (s *Server) hostAllowed(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	for _, a := range s.Cfg.AllowedHosts {
		if strings.EqualFold(a, hostport) || (!strings.Contains(strings.Trim(a, "[]"), ":") && strings.EqualFold(a, host)) {
			return true
		}
	}
	return false
}

func (s *Server) hosts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && !s.hostAllowed(r.Host) {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cors lets the configured reader origins (e.g. PanelFlow, comics.skriv.ist) call the
// API and OPDS with tokens. No credentials: those clients never send cookies.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		api := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/opds/")
		if api && origin != "" && slices.Contains(s.Cfg.CORSOrigins, origin) {
			h := w.Header()
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, POST, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Access is what a route requires.
type Access int

const (
	Public Access = iota
	Reader        // any valid credential
	Admin         // the owner's UI session only
)

// guard runs after routing (so /opds/t/{token}/ path values are visible): it
// identifies the caller, enforces the route's access level, then the CSRF rules.
func (s *Server) guard(level Access, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := s.identify(r)
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, p))
		switch {
		case level == Reader && p == Anonymous:
			if strings.HasPrefix(r.URL.Path, "/opds/") {
				w.Header().Set("WWW-Authenticate", `Basic realm="Armarium", charset="UTF-8"`)
			}
			writeError(w, http.StatusUnauthorized, "authentication required")
		case level == Admin && p == Anonymous:
			writeError(w, http.StatusUnauthorized, "owner session required")
		case level == Admin && p != Session:
			writeError(w, http.StatusForbidden, "owner session required")
		case !s.writeAllowed(w, r, p):
		default:
			h(w, r)
		}
	})
}

func (s *Server) identify(r *http.Request) Principal {
	ctx := r.Context()
	valid := func(secret string) bool {
		ok, err := s.Store.TokenValid(ctx, auth.Digest(secret))
		return err == nil && ok
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if ok, err := s.Store.SessionValid(ctx, auth.Digest(c.Value)); err == nil && ok {
			return Session
		}
	}
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, cred, _ := strings.Cut(h, " ")
		switch strings.ToLower(scheme) {
		case "bearer":
			if valid(strings.TrimSpace(cred)) {
				return Token
			}
		case "basic":
			if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cred)); err == nil {
				if _, pass, ok := strings.Cut(string(raw), ":"); ok && valid(pass) {
					return Basic
				}
			}
		}
		return Anonymous
	}
	if t := r.URL.Query().Get("token"); t != "" && valid(t) {
		return Token
	}
	if t := r.PathValue("token"); t != "" && valid(t) {
		return Token
	}
	return Anonymous
}

// writeAllowed is the CSRF check for state-changing requests. JSON-only bodies
// force a CORS preflight for cross-site callers; ambient credentials (cookie,
// cached Basic) additionally need an Origin that is one of our own hosts.
func (s *Server) writeAllowed(w http.ResponseWriter, r *http.Request, p Principal) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return false
	}
	origin := r.Header.Get("Origin")
	u, err := url.Parse(origin)
	own := err == nil && origin != "" && s.hostAllowed(u.Host)
	ok := own
	if p == Token { // not ambient: a script had to attach it explicitly
		ok = origin == "" || own || slices.Contains(s.Cfg.CORSOrigins, origin)
	}
	if !ok {
		writeError(w, http.StatusForbidden, "cross-origin request refused")
	}
	return ok
}
