package config

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/c0ze/armarium/internal/tailscale"
)

// SkrivistOrigins are the Skrivist Books and Comics readers: the hosted web
// apps and the macOS and Windows desktop apps. A managed config always
// allows exactly these.
var SkrivistOrigins = []string{
	"https://comics.skriv.ist",
	"https://books.skriv.ist",
	"tauri://localhost",
	"http://tauri.localhost",
	"https://tauri.localhost",
}

// DefaultPort is where Armarium listens unless told otherwise.
const DefaultPort = 8580

// Desktop is the [desktop] table of a managed config: what the owner chose
// in setup or Settings. The listen address, host names and public URL are
// worked out from it at every start.
type Desktop struct {
	Network    string   `toml:"network"`   // "local": this computer only; "home": devices on the local network
	Tailscale  bool     `toml:"tailscale"` // also serve https://<machine>.<tailnet>.ts.net through `tailscale serve`
	Port       int      `toml:"port"`
	PublicURL  string   `toml:"public_url,omitempty"`  // advanced: the owner's own HTTPS address
	ExtraHosts []string `toml:"extra_hosts,omitempty"` // advanced: more host names to answer to
}

// Settings is what setup, Settings and the desktop app exchange.
type Settings struct {
	Libraries  []Library `json:"libraries"`
	Network    string    `json:"network"`
	Tailscale  bool      `json:"tailscale"`
	Port       int       `json:"port"`
	PublicURL  string    `json:"publicUrl"`
	ExtraHosts []string  `json:"extraHosts"`
}

// Settings returns what a managed config holds.
func (c Config) Settings() Settings {
	s := Settings{Libraries: slices.Clone(c.Libraries), Network: "local", Port: DefaultPort}
	if d := c.Desktop; d != nil {
		s.Network, s.Tailscale, s.Port = d.Network, d.Tailscale, d.Port
		s.PublicURL, s.ExtraHosts = d.PublicURL, slices.Clone(d.ExtraHosts)
	}
	if s.Libraries == nil {
		s.Libraries = []Library{}
	}
	return s
}

// Probe answers the questions derivation asks about this machine.
type Probe interface {
	LANIPv4() (netip.Addr, bool)  // the address other devices on the network reach this one at
	InterfaceAddrs() []netip.Addr // every address of this machine
	LocalHostName() string        // e.g. "studio.local", "" when unknown
	TailscaleName() string        // e.g. "studio.tail1234.ts.net", "" without a running Tailscale
}

// SystemProbe asks the operating system.
var SystemProbe Probe = systemProbe{}

var (
	probe     Probe = SystemProbe
	addrMu    sync.Mutex
	addrCache []netip.Addr
	addrAt    time.Time
)

func setProbe(p Probe) {
	addrMu.Lock()
	probe, addrCache, addrAt = p, nil, time.Time{}
	addrMu.Unlock()
}

// deriveDesktop fills in the network settings of a managed config. md says
// which top-level keys the file set: an explicit allowed_hosts or public_url
// there is kept on top of the derived ones.
func (c *Config) deriveDesktop(md toml.MetaData) error {
	d := c.Desktop
	if d.Network == "" {
		d.Network = "local"
	}
	if d.Network != "local" && d.Network != "home" {
		return fmt.Errorf("[desktop] network %q must be \"local\" or \"home\"", d.Network)
	}
	if d.Port == 0 {
		d.Port = DefaultPort
	}
	if d.Port < 1 || d.Port > 65535 {
		return fmt.Errorf("[desktop] port %d is not a port number", d.Port)
	}
	port := strconv.Itoa(d.Port)
	var extra []string
	if md.IsDefined("allowed_hosts") {
		extra = c.AllowedHosts
	}
	explicitURL := ""
	if md.IsDefined("public_url") {
		explicitURL = c.PublicURL
	}
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	c.Listen, c.PublicURL, c.TrustedProxies, c.AllowLocalAddresses = "127.0.0.1:"+port, "", nil, false
	if d.Network == "home" {
		c.Listen = ":" + port
		c.AllowLocalAddresses = true
		if name := probe.LocalHostName(); name != "" {
			hosts = append(hosts, name)
		}
		if ip, ok := probe.LANIPv4(); ok {
			hosts = append(hosts, ip.String())
			c.PublicURL = "http://" + net.JoinHostPort(ip.String(), port)
		}
	}
	if d.Tailscale {
		if name := probe.TailscaleName(); name != "" {
			hosts = append(hosts, name)
			c.PublicURL = "https://" + name
			c.TrustedProxies = []string{"127.0.0.1", "::1"}
		}
	}
	for _, h := range append(slices.Clone(d.ExtraHosts), extra...) {
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	if explicitURL != "" {
		c.PublicURL = explicitURL
	}
	if d.PublicURL != "" {
		c.PublicURL = d.PublicURL
	}
	c.AllowedHosts = hosts
	c.CORSOrigins = slices.Clone(SkrivistOrigins)
	return nil
}

// LocalAddress reports whether host (an IP literal, brackets allowed) is one
// of this machine's own addresses. A request naming an IP literal can't come
// from a DNS-rebinding page: the literal is its own origin.
func LocalAddress(host string) bool {
	a, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	a = a.Unmap().WithZone("")
	if a.IsLoopback() {
		return true
	}
	addrMu.Lock()
	if addrCache == nil || time.Since(addrAt) > 10*time.Second {
		addrCache, addrAt = probe.InterfaceAddrs(), time.Now()
		if addrCache == nil {
			addrCache = []netip.Addr{}
		}
	}
	addrs := addrCache
	addrMu.Unlock()
	return slices.Contains(addrs, a)
}

type systemProbe struct{}

func (systemProbe) LANIPv4() (netip.Addr, bool) {
	// Dialling UDP sends nothing; it only asks the OS which address the
	// default route would use.
	if conn, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		local := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if a, ok := netip.AddrFromSlice(local.IP); ok && a.Unmap().IsPrivate() {
			return a.Unmap(), true
		}
	}
	for _, a := range (systemProbe{}).InterfaceAddrs() {
		if a.Is4() && a.IsPrivate() {
			return a, true
		}
	}
	return netip.Addr{}, false
}

func (systemProbe) InterfaceAddrs() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().Unmap().WithZone(""))
		}
	}
	return out
}

func (systemProbe) LocalHostName() string {
	name := ""
	if runtime.GOOS == "darwin" {
		// The Bonjour name, which may differ from the DHCP host name.
		if out, err := exec.Command("scutil", "--get", "LocalHostName").Output(); err == nil {
			name = strings.TrimSpace(string(out))
		}
	}
	if name == "" {
		name, _ = os.Hostname()
	}
	name, _, _ = strings.Cut(name, ".")
	if name == "" {
		return ""
	}
	return strings.ToLower(name) + ".local"
}

func (systemProbe) TailscaleName() string {
	return tailscale.Name(context.Background())
}
