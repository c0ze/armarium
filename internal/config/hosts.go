package config

import (
	"net"
	"strings"
)

// HostAllowed is the DNS-rebinding defence: an entry with a port must match
// exactly, an entry without one (including a bare or bracketed IPv6 address)
// matches that host on any port.
func HostAllowed(allowed []string, hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	for _, a := range allowed {
		if _, _, err := net.SplitHostPort(a); err == nil {
			if strings.EqualFold(a, hostport) {
				return true
			}
			continue
		}
		if strings.EqualFold(strings.Trim(a, "[]"), host) {
			return true
		}
	}
	return false
}
