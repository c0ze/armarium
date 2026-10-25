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
	"github.com/c0ze/armarium/internal/config"
)

const (
	sessionCookie = "armarium_session"
	maxBodyBytes  = 64 << 10
	// The largest panel correction Valid() accepts is about 100 KiB of JSON.
	maxPanelBytes = 256 << 10
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
		limit := int64(maxBodyBytes)
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/panels") {
			limit = maxPanelBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
		next.ServeHTTP(w, r)
	})
}

// hostAllowed is the Host allowlist, plus this machine's own IP addresses when
// a managed config serves devices at home.
func (s *Server) hostAllowed(hostport string) bool {
	if config.HostAllowed(s.Cfg.AllowedHosts, hostport) {
		return true
	}
	if !s.Cfg.AllowLocalAddresses {
		return false
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return config.LocalAddress(host)
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
		p := r.URL.Path
		if p != "/opds" && !strings.HasPrefix(p, "/api/") && !strings.HasPrefix(p, "/opds/") {
			next.ServeHTTP(w, r)
			return
		}
		// Responses differ by Origin even for origins that get no CORS headers.
		h := w.Header()
		h.Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" && slices.Contains(s.Cfg.CORSOrigins, origin) {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Content-Disposition, ETag, Retry-After, X-Armarium-Pages")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// The answer depends on what the preflight asks for, not only on its origin.
				h.Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers, Access-Control-Request-Private-Network")
				h.Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, POST, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range, If-None-Match, If-Range")
				if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
					h.Set("Access-Control-Allow-Private-Network", "true")
				}
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
		p, err := s.identify(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
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

// identify says how a request authenticated. A store failure is an error, not
// an anonymous request: a broken database must not look like a revoked token.
func (s *Server) identify(r *http.Request) (Principal, error) {
	ctx := r.Context()
	valid := func(secret string) (bool, error) { return s.Store.TokenValid(ctx, auth.Digest(secret)) }
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		ok, err := s.Store.SessionValid(ctx, auth.Digest(c.Value))
		if err != nil {
			return Anonymous, err
		}
		if ok {
			return Session, nil
		}
	}
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, cred, _ := strings.Cut(h, " ")
		secret, as := "", Anonymous
		switch strings.ToLower(scheme) {
		case "bearer":
			secret, as = strings.TrimSpace(cred), Token
		case "basic":
			if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cred)); err == nil {
				if _, pass, ok := strings.Cut(string(raw), ":"); ok {
					secret, as = pass, Basic
				}
			}
		}
		if secret != "" {
			ok, err := valid(secret)
			if err != nil {
				return Anonymous, err
			}
			if ok {
				return as, nil
			}
		}
		// An unusable header (a proxy's own login, a revoked token) does not
		// hide a valid ?token= or path token below.
	}
	for _, t := range []string{r.URL.Query().Get("token"), r.PathValue("token")} {
		if t == "" {
			continue
		}
		ok, err := valid(t)
		if err != nil {
			return Anonymous, err
		}
		if ok {
			return Token, nil
		}
	}
	return Anonymous, nil
}

// writeAllowed is the CSRF check for state-changing requests. JSON-only bodies
// force a CORS preflight for cross-site callers; ambient credentials (cookie,
// cached Basic) additionally need an Origin that is one of our own hosts.
func (s *Server) writeAllowed(w http.ResponseWriter, r *http.Request, p Principal) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	// A DELETE has no body: its route and Origin are the whole request. A
	// cross-origin DELETE always needs a preflight, which only cors_origins pass.
	if r.Method != http.MethodDelete || r.ContentLength != 0 {
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
			return false
		}
	}
	origin := r.Header.Get("Origin")
	u, err := url.Parse(origin)
	// A custom-scheme webview is a token client, not Armarium's own web UI,
	// even when its host is localhost and localhost is in allowed_hosts.
	own := err == nil && (u.Scheme == "http" || u.Scheme == "https") && s.hostAllowed(u.Host)
	ok := own
	// KOSync clients send Basic with no Origin. Their X-Auth-User header can't be
	// added cross-site without a preflight, and /kosync/ answers none.
	koSync := p == Basic && strings.HasPrefix(r.URL.Path, "/kosync/") && r.Header.Get("X-Auth-User") != ""
	if p == Token || koSync { // not ambient: a script had to attach it explicitly
		ok = origin == "" || own || slices.Contains(s.Cfg.CORSOrigins, origin)
	}
	if !ok {
		writeError(w, http.StatusForbidden, "cross-origin request refused")
	}
	return ok
}
