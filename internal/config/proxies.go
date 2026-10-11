package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// parseProxy reads a trusted_proxies entry: a CIDR, or a single address.
func parseProxy(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if p, err := netip.ParsePrefix(s); err == nil {
		bits := p.Bits() - unmappedBits(p.Addr())
		switch {
		case bits < 0:
			return netip.Prefix{}, fmt.Errorf("trusted_proxies entry %q: an IPv4-mapped prefix needs at least /96; write the IPv4 form, such as 172.17.0.1", s)
		case bits == 0:
			return netip.Prefix{}, fmt.Errorf("trusted_proxies entry %q would trust every address to report the client's address", s)
		}
		return netip.PrefixFrom(p.Addr().Unmap(), bits).Masked(), nil
	}
	if a, err := netip.ParseAddr(s); err == nil {
		a = a.Unmap()
		return netip.PrefixFrom(a, a.BitLen()), nil
	}
	return netip.Prefix{}, fmt.Errorf("trusted_proxies entry %q must be an IP address or CIDR, such as 172.17.0.1 or 10.0.0.0/8", s)
}

// unmappedBits is how many prefix bits an IPv4-mapped IPv6 prefix loses when
// it becomes plain IPv4.
func unmappedBits(a netip.Addr) int {
	if a.Is4In6() {
		return 96
	}
	return 0
}

// ProxyPrefixes returns the validated trusted_proxies.
func (c Config) ProxyPrefixes() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(c.TrustedProxies))
	for _, s := range c.TrustedProxies {
		if p, err := parseProxy(s); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// Trusted reports whether a is inside one of the prefixes.
func Trusted(prefixes []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
