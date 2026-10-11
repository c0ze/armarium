package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// canonicalPublicURL turns public_url into the origin a browser would send:
// lower-case scheme and host, compressed IPv6, a numeric port without leading
// zeros, and no default port. Empty stays empty.
func canonicalPublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	bad := func(why string) error { return fmt.Errorf("public_url %q %s", raw, why) }
	if strings.ContainsAny(raw, "?#") {
		return "", bad("must not have a query or fragment")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", bad("is not a URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", bad("must be an http(s) URL like https://armarium.example")
	}
	if u.User != nil {
		return "", bad("must not contain a user name or password")
	}
	if u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return "", bad("must not have a path: Armarium is served at the root")
	}
	host := u.Hostname()
	if host == "" {
		return "", bad("has no host")
	}
	for i := 0; i < len(host); i++ {
		if host[i] >= 0x80 {
			return "", bad("must use the punycode (xn--) form of the host name")
		}
	}
	host = strings.ToLower(host)
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.String()
	}
	port := ""
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", bad("has an invalid port")
		}
		if !(scheme == "https" && n == 443) && !(scheme == "http" && n == 80) {
			port = strconv.Itoa(n)
		}
	}
	authority := host
	if port != "" {
		authority = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	return scheme + "://" + authority, nil
}

var errPublicHost = errors.New("public_url's host must be listed in allowed_hosts, or phones get 421 unknown host")

// canonicalOrigin turns a cors_origins entry into the Origin a browser sends:
// lower-case scheme and host, no default port, no trailing slash. Tauri's macOS
// webview uses the custom-scheme origin tauri://localhost; other custom schemes
// and opaque (null) origins stay disallowed.
func canonicalOrigin(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if strings.EqualFold(s, "tauri://localhost") {
		return "tauri://localhost", nil
	}
	bad := fmt.Errorf("cors_origins entry %q must be an origin such as https://books.skriv.ist (scheme and host, no path), or tauri://localhost", raw)
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || u.Host == "" {
		return "", bad
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", bad
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, nil
}
