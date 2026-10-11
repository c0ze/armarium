package httpapi

import (
	"crypto/tls"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientAddr(t *testing.T) {
	s := &Server{proxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	for _, c := range []struct{ remote, xff, want string }{
		{"192.0.2.1:5", "", "192.0.2.1"},
		{"192.0.2.1:5", "198.51.100.7", "192.0.2.1"},                      // untrusted peer: its header is ignored
		{"10.0.0.2:5", "6.6.6.6, 198.51.100.7, 10.0.0.9", "198.51.100.7"}, // right-most untrusted hop, never the spoofable left
		{"10.0.0.2:5", "garbage, ,", "10.0.0.2"},                          // unparsable: the peer, never the text
		{"10.0.0.2:5", "", "10.0.0.2"},
		{"10.0.0.2:5", "10.0.0.7, 10.0.0.9", "10.0.0.2"}, // every hop trusted: the peer
		{"[2001:db8::1]:5", "", "2001:db8::1"},
		{"[::ffff:192.0.2.9]:5", "", "192.0.2.9"},
	} {
		r := httptest.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.clientAddr(r); got != netip.MustParseAddr(c.want) {
			t.Errorf("%s via %q: %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestIsHTTPS(t *testing.T) {
	s := &Server{proxies: []netip.Prefix{netip.MustParsePrefix("172.17.0.1/32")}}
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "172.17.0.1:5"
	r.Header.Set("X-Forwarded-Proto", "https")
	if !s.isHTTPS(r) {
		t.Error("a trusted proxy's HTTPS")
	}
	r.RemoteAddr = "192.0.2.1:5"
	if s.isHTTPS(r) {
		t.Error("an untrusted peer cannot claim HTTPS")
	}
	r.Header.Del("X-Forwarded-Proto")
	r.TLS = &tls.ConnectionState{}
	if !s.isHTTPS(r) {
		t.Error("direct TLS")
	}
}
