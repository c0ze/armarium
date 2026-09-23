package config

import (
	"os"
	"path/filepath"
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
