package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesFileEnvAndValidation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "armarium.toml")
	os.WriteFile(p, []byte(`
listen = "0.0.0.0:9000"
[[library]]
name = "Comics"
root = "comics"
kind = "comics"
`), 0o600)
	t.Setenv("ARMARIUM_ALLOWED_HOSTS", "nas.local, armarium.example:443")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:9000" || len(c.AllowedHosts) != 2 || c.AllowedHosts[1] != "armarium.example:443" {
		t.Fatalf("unexpected %+v", c)
	}
	if !filepath.IsAbs(c.Libraries[0].Root) {
		t.Fatalf("root not absolute: %s", c.Libraries[0].Root)
	}
	if c.Limits.MaxEntryBytes != 32<<20 {
		t.Fatal("defaults lost")
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	cases := map[string]string{
		"bad kind":    "[[library]]\nname='a'\nroot='/x'\nkind='movies'\n",
		"dup name":    "[[library]]\nname='a'\nroot='/x'\nkind='books'\n[[library]]\nname='a'\nroot='/y'\nkind='books'\n",
		"bad origin":  "cors_origins=['comics.skriv.ist']\n",
		"empty hosts": "allowed_hosts=[]\n",
	}
	for name, body := range cases {
		p := filepath.Join(t.TempDir(), "f.toml")
		os.WriteFile(p, []byte(body), 0o600)
		if _, err := Load(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func loadPublicURL(t *testing.T, publicURL string, hosts ...string) (Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.toml")
	body := fmt.Sprintf("public_url = %q\nallowed_hosts = [", publicURL)
	for i, h := range hosts {
		if i > 0 {
			body += ", "
		}
		body += fmt.Sprintf("%q", h)
	}
	os.WriteFile(p, []byte(body+"]\n"), 0o600)
	return Load(p)
}

func TestPublicURL(t *testing.T) {
	hosts := []string{"h", "192.168.1.10", "nas.example", "[2001:db8::1]", "::1", "xn--bcher-kva.example"}
	ok := map[string]string{
		"":                               "",
		"  ":                             "",
		"https://h":                      "https://h",
		"https://h/":                     "https://h",
		"https://h:8443":                 "https://h:8443",
		"https://h:443":                  "https://h",
		"http://h:80":                    "http://h",
		"HTTPS://NAS.Example:443/":       "https://nas.example",
		"http://192.168.1.10:8580":       "http://192.168.1.10:8580",
		"https://h:08443":                "https://h:8443",
		"https://h:0443":                 "https://h",
		"https://[2001:0DB8::0001]:443/": "https://[2001:db8::1]",
		"http://[::1]:8580":              "http://[::1]:8580",
		"https://xn--bcher-kva.example":  "https://xn--bcher-kva.example",
	}
	for in, want := range ok {
		c, err := loadPublicURL(t, in, hosts...)
		if err != nil {
			t.Errorf("%q: %v", in, err)
		} else if c.PublicURL != want {
			t.Errorf("%q: got %q, want %q", in, c.PublicURL, want)
		}
	}
	for _, in := range []string{"h", "ftp://h", "https://u:p@h", "https://h?", "https://h?q=1", "https://h#",
		"https://h/armarium", "https://h:0", "https://h:70000"} {
		if _, err := loadPublicURL(t, in, hosts...); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
	_, err := loadPublicURL(t, "https://other.example", hosts...)
	if err == nil || !strings.Contains(err.Error(), "public_url") || !strings.Contains(err.Error(), "allowed_hosts") {
		t.Errorf("host not allowed: %v", err)
	}
	_, err = loadPublicURL(t, "https://bücher.example", "bücher.example")
	if err == nil || !strings.Contains(err.Error(), "punycode") {
		t.Errorf("non-ASCII host: %v", err)
	}
}

func TestPublicURLEnv(t *testing.T) {
	t.Setenv("ARMARIUM_PUBLIC_URL", "https://h/")
	c, err := loadPublicURL(t, "", "h")
	if err != nil || c.PublicURL != "https://h" {
		t.Fatalf("env override: %q %v", c.PublicURL, err)
	}
}
