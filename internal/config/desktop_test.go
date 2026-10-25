package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
)

type fakeProbe struct {
	lan       string
	addrs     []string
	name      string
	tailscale string
}

func (p fakeProbe) LANIPv4() (netip.Addr, bool) {
	a, err := netip.ParseAddr(p.lan)
	return a, err == nil
}
func (p fakeProbe) InterfaceAddrs() []netip.Addr {
	var out []netip.Addr
	for _, s := range p.addrs {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}
func (p fakeProbe) LocalHostName() string { return p.name }
func (p fakeProbe) TailscaleName() string { return p.tailscale }

var studio = fakeProbe{lan: "192.168.1.20", addrs: []string{"192.168.1.20", "fe80::1", "100.101.102.103"},
	name: "studio.local", tailscale: "studio.tail1234.ts.net"}

func useProbe(t *testing.T, p Probe) {
	t.Helper()
	old := probe
	setProbe(p)
	t.Cleanup(func() { setProbe(old) })
}

func loadManaged(t *testing.T, body string) Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "armarium.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDeriveLocal(t *testing.T) {
	useProbe(t, studio)
	c := loadManaged(t, "[desktop]\nnetwork = \"local\"\n")
	if c.Listen != "127.0.0.1:8580" {
		t.Errorf("listen %q", c.Listen)
	}
	if want := []string{"localhost", "127.0.0.1", "::1"}; !reflect.DeepEqual(c.AllowedHosts, want) {
		t.Errorf("hosts %q", c.AllowedHosts)
	}
	if c.PublicURL != "" || c.AllowLocalAddresses || len(c.TrustedProxies) != 0 {
		t.Errorf("local mode leaked: %q %v %q", c.PublicURL, c.AllowLocalAddresses, c.TrustedProxies)
	}
	if !reflect.DeepEqual(c.CORSOrigins, SkrivistOrigins) {
		t.Errorf("cors %q", c.CORSOrigins)
	}
}

func TestDeriveHome(t *testing.T) {
	useProbe(t, studio)
	c := loadManaged(t, "[desktop]\nnetwork = \"home\"\nport = 9000\n")
	if c.Listen != ":9000" {
		t.Errorf("listen %q", c.Listen)
	}
	if !slices.Contains(c.AllowedHosts, "studio.local") || !slices.Contains(c.AllowedHosts, "localhost") {
		t.Errorf("hosts %q", c.AllowedHosts)
	}
	if !c.AllowLocalAddresses {
		t.Error("own addresses not allowed")
	}
	if c.PublicURL != "http://192.168.1.20:9000" {
		t.Errorf("public url %q", c.PublicURL)
	}
}

func TestDeriveTailscale(t *testing.T) {
	useProbe(t, studio)
	for _, network := range []string{"local", "home"} {
		c := loadManaged(t, "[desktop]\nnetwork = \""+network+"\"\ntailscale = true\n")
		if !slices.Contains(c.AllowedHosts, "studio.tail1234.ts.net") {
			t.Errorf("%s: hosts %q", network, c.AllowedHosts)
		}
		if c.PublicURL != "https://studio.tail1234.ts.net" {
			t.Errorf("%s: public url %q", network, c.PublicURL)
		}
		if !reflect.DeepEqual(c.TrustedProxies, []string{"127.0.0.1", "::1"}) {
			t.Errorf("%s: proxies %q", network, c.TrustedProxies)
		}
	}
	// Tailscale stopped: no name, nothing added, still loads.
	useProbe(t, fakeProbe{lan: "192.168.1.20", name: "studio.local"})
	c := loadManaged(t, "[desktop]\nnetwork = \"home\"\ntailscale = true\n")
	if c.PublicURL != "http://192.168.1.20:8580" {
		t.Errorf("without a tailnet name: %q", c.PublicURL)
	}
}

func TestDeriveExplicitAddressWins(t *testing.T) {
	useProbe(t, studio)
	c := loadManaged(t, "[desktop]\nnetwork = \"home\"\npublic_url = \"https://books.example.com\"\nextra_hosts = [\"books.example.com\"]\n")
	if c.PublicURL != "https://books.example.com" || !slices.Contains(c.AllowedHosts, "books.example.com") {
		t.Errorf("explicit: %q %q", c.PublicURL, c.AllowedHosts)
	}
}

func TestDeriveRejectsUnknownNetwork(t *testing.T) {
	useProbe(t, studio)
	p := filepath.Join(t.TempDir(), "a.toml")
	os.WriteFile(p, []byte("[desktop]\nnetwork = \"everyone\"\n"), 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("unknown network accepted")
	}
}

func TestDeriveHomeFollowsAddressChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "armarium.toml")
	if err := WriteManaged(p, Settings{Network: "home", Port: 8580}, filepath.Join(dir, "data")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	useProbe(t, fakeProbe{lan: "192.168.1.20", addrs: []string{"192.168.1.20"}})
	c1, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	useProbe(t, fakeProbe{lan: "192.168.1.37", addrs: []string{"192.168.1.37"}})
	c2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c1.PublicURL != "http://192.168.1.20:8580" || c2.PublicURL != "http://192.168.1.37:8580" {
		t.Fatalf("public url did not follow: %q then %q", c1.PublicURL, c2.PublicURL)
	}
	if !LocalAddress("192.168.1.37") || LocalAddress("192.168.1.20") {
		t.Fatal("own-address rule did not follow the new address")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("loading rewrote the file")
	}
}

func TestWriteManagedRoundTrip(t *testing.T) {
	useProbe(t, studio)
	dir := t.TempDir()
	p := filepath.Join(dir, "armarium.toml")
	comics, cal := t.TempDir(), t.TempDir()
	in := Settings{
		Libraries: []Library{{Name: "Comics", Root: comics, Kind: "comics"}, {Name: "Calibre", Root: cal, Kind: "books", Calibre: true}},
		Network:   "home", Tailscale: true, Port: 8581,
		PublicURL: "https://books.example.com", ExtraHosts: []string{"books.example.com"},
	}
	if err := WriteManaged(p, in, filepath.Join(dir, "data")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if first, _, _ := cutLine(string(b)); first != ManagedHeader {
		t.Fatalf("first line %q", first)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Settings(); !reflect.DeepEqual(got, in) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, in)
	}
	if c.DataDir != filepath.Join(dir, "data") {
		t.Fatalf("data dir %q", c.DataDir)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", fi.Mode().Perm())
		}
	}
}

func TestWriteManagedKeepsOldFileOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a read-only directory")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "armarium.toml")
	if err := WriteManaged(p, Settings{Network: "local"}, dir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	if err := WriteManaged(p, Settings{Network: "home"}, dir); err == nil {
		t.Fatal("write into a read-only directory succeeded")
	}
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Fatal("the old file changed")
	}
}

func TestLocalAddress(t *testing.T) {
	useProbe(t, studio)
	for host, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "[::1]": true, "192.168.1.20": true, "100.101.102.103": true,
		"192.168.1.21": false, "8.8.8.8": false, "studio.local": false, "evil.example": false, "": false,
	} {
		if got := LocalAddress(host); got != want {
			t.Errorf("LocalAddress(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestShippedExampleCORSIsSkrivistOrigins(t *testing.T) {
	c, err := Load("../../deploy/armarium.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.CORSOrigins, SkrivistOrigins) {
		t.Fatalf("example %q, SkrivistOrigins %q", c.CORSOrigins, SkrivistOrigins)
	}
}

func cutLine(s string) (string, string, bool) {
	for i := range s {
		if s[i] == '\n' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
