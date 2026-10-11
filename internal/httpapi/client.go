package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/c0ze/armarium/internal/config"
)

// peer is the address the connection came from.
func peer(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

// clientAddr is the client a login attempt counts against: the peer, or,
// behind a trusted proxy, the right-most X-Forwarded-For hop that is not one
// of ours. Hops to its left can be written by the client, so they are never
// used, and text that is not an address never becomes a key.
func (s *Server) clientAddr(r *http.Request) netip.Addr {
	a := peer(r)
	if !config.Trusted(s.proxies, a) {
		return a
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return a
		}
		if hop = hop.Unmap(); !config.Trusted(s.proxies, hop) {
			return hop
		}
	}
	return a
}

// isHTTPS reports whether the visitor reached Armarium over HTTPS: directly,
// or through a trusted proxy that says so.
func (s *Server) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return config.Trusted(s.proxies, peer(r)) && strings.EqualFold(strings.TrimSpace(proto), "https")
}
