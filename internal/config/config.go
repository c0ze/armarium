// Package config loads armarium.toml and applies ARMARIUM_* environment overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Library struct {
	Name string `toml:"name"`
	Root string `toml:"root"`
	Kind string `toml:"kind"` // "comics" or "books"
	// Include limits the scan to top-level entries matching these globs, e.g.
	// ["*_downloads"] to serve only matching folders. Empty means everything.
	Include []string `toml:"include"`
	// Labels is an optional JSON file naming the sources (top-level folders):
	// {"<folder>": "Label"} or {"<folder>": {"label": "..", "year": 2015}}.
	Labels string `toml:"labels"`
	// Tags is an optional JSON sidecar of subject tags by library-relative path:
	// {"oreilly_downloads/Learning Go.epub": ["Go"]}.
	Tags string `toml:"tags"`
	// Calibre reads the library from root/metadata.db (titles, authors, series,
	// tags, covers) instead of walking folders, like Calibre-Web.
	Calibre bool `toml:"calibre"`
}

type Scan struct {
	Workers    int `toml:"workers"`      // concurrent file opens while scanning
	DirSleepMS int `toml:"dir_sleep_ms"` // pause between directories, keeps NAS I/O low
	IntervalH  int `toml:"interval_hours"`
}

type Limits struct {
	MaxEntryBytes   int64 `toml:"max_entry_bytes"`   // largest decompressed archive entry
	MaxImagePixels  int64 `toml:"max_image_pixels"`  // decompression-bomb guard for covers
	MaxEntries      int   `toml:"max_entries"`       // entries per archive
	MaxOpenArchives int   `toml:"max_open_archives"` // concurrent archive opens
}

type Config struct {
	Listen       string   `toml:"listen"`
	DataDir      string   `toml:"data_dir"`
	AllowedHosts []string `toml:"allowed_hosts"`
	CORSOrigins  []string `toml:"cors_origins"`
	// PublicURL is the address phones and HTTPS readers use, e.g.
	// https://armarium.example; pairing QR codes point there.
	PublicURL    string `toml:"public_url"`
	PasswordHash string `toml:"password_hash"`
	SecureCookie bool   `toml:"secure_cookie"`
	// TrustedProxies are reverse proxies (IPs or CIDRs) whose X-Forwarded-For
	// and X-Forwarded-Proto are believed: login limits count the real client
	// and the session cookie is Secure for HTTPS visitors.
	TrustedProxies []string `toml:"trusted_proxies"`
	// PanelFlowURL is where PanelFlow is served (e.g. https://host/comics); the UI
	// links comics to its Armarium mode there.
	PanelFlowURL string    `toml:"panelflow_url"`
	Libraries    []Library `toml:"library"`
	Scan         Scan      `toml:"scan"`
	Limits       Limits    `toml:"limits"`
	// Overrides lists keys set in the file that an ARMARIUM_* variable replaced.
	// The Docker image sets ARMARIUM_LISTEN and ARMARIUM_DATA_DIR, so a listen
	// in armarium.toml would otherwise be ignored without a word.
	Overrides []string `toml:"-"`
}

func Default() Config {
	return Config{
		Listen:       "127.0.0.1:8580",
		DataDir:      "data",
		AllowedHosts: []string{"localhost", "127.0.0.1"},
		Scan:         Scan{Workers: 2, DirSleepMS: 20},
		Limits: Limits{
			// Sized for the NAS container (256 MiB): a cover decode is at most
			// 16 Mpx × 4 B = 64 MiB, a CBR page is read whole (≤ 32 MiB).
			MaxEntryBytes:   32 << 20,
			MaxImagePixels:  16_000_000,
			MaxEntries:      50_000, // some EPUBs hold 11k+ images; an entry is ~200 B of index
			MaxOpenArchives: 4,
		},
	}
}

// Load reads path (if non-empty) over the defaults, then env overrides, then validates.
func Load(path string) (Config, error) {
	c := Default()
	var md toml.MetaData
	if path != "" {
		var err error
		if md, err = toml.DecodeFile(path, &c); err != nil {
			return c, fmt.Errorf("config %s: %w", path, err)
		}
	}
	applyEnv(&c, md)
	return c, c.validate()
}

func applyEnv(c *Config, md toml.MetaData) {
	set := func(env, key string, apply func(string)) {
		if v := os.Getenv(env); v != "" {
			apply(v)
			if md.IsDefined(key) {
				c.Overrides = append(c.Overrides, key)
			}
		}
	}
	set("ARMARIUM_LISTEN", "listen", func(v string) { c.Listen = v })
	set("ARMARIUM_DATA_DIR", "data_dir", func(v string) { c.DataDir = v })
	set("ARMARIUM_PASSWORD_HASH", "password_hash", func(v string) { c.PasswordHash = v })
	set("ARMARIUM_ALLOWED_HOSTS", "allowed_hosts", func(v string) { c.AllowedHosts = splitList(v) })
	set("ARMARIUM_PUBLIC_URL", "public_url", func(v string) { c.PublicURL = v })
	set("ARMARIUM_CORS_ORIGINS", "cors_origins", func(v string) { c.CORSOrigins = splitList(v) })
	set("ARMARIUM_TRUSTED_PROXIES", "trusted_proxies", func(v string) { c.TrustedProxies = splitList(v) })
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (c *Config) validate() error {
	if len(c.AllowedHosts) == 0 {
		return errors.New("allowed_hosts must not be empty")
	}
	if c.PanelFlowURL != "" && !strings.HasPrefix(c.PanelFlowURL, "https://") && !strings.HasPrefix(c.PanelFlowURL, "http://") {
		return fmt.Errorf("panelflow_url %q must be an http(s) URL", c.PanelFlowURL)
	}
	for _, p := range c.TrustedProxies {
		if _, err := parseProxy(p); err != nil {
			return err
		}
	}
	pub, err := canonicalPublicURL(c.PublicURL)
	if err != nil {
		return err
	}
	c.PublicURL = pub
	if pub != "" && !HostAllowed(c.AllowedHosts, strings.SplitN(pub, "://", 2)[1]) {
		return fmt.Errorf("%w (public_url %q, allowed_hosts %q)", errPublicHost, pub, c.AllowedHosts)
	}
	// Browsers send an exact Origin, so each entry is stored in that form: an
	// entry with a trailing slash or upper case would otherwise never match.
	for i, o := range c.CORSOrigins {
		canon, err := canonicalOrigin(o)
		if err != nil {
			return err
		}
		c.CORSOrigins[i] = canon
	}
	seen := map[string]bool{}
	for i := range c.Libraries {
		l := &c.Libraries[i]
		if l.Name == "" || l.Root == "" {
			return fmt.Errorf("library %d: name and root are required", i)
		}
		if l.Kind != "comics" && l.Kind != "books" {
			return fmt.Errorf("library %q: kind must be comics or books", l.Name)
		}
		if seen[l.Name] {
			return fmt.Errorf("library %q defined twice", l.Name)
		}
		seen[l.Name] = true
		if l.Calibre && len(l.Include) > 0 {
			return fmt.Errorf("library %q: include does not apply to a calibre library", l.Name)
		}
		for _, g := range l.Include {
			if _, err := filepath.Match(g, ""); err != nil {
				return fmt.Errorf("library %q: bad include glob %q", l.Name, g)
			}
		}
		abs, err := filepath.Abs(l.Root)
		if err != nil {
			return err
		}
		l.Root = abs
	}
	if c.Scan.Workers < 1 {
		c.Scan.Workers = 1
	}
	if c.Limits.MaxOpenArchives < 1 {
		c.Limits.MaxOpenArchives = 1
	}
	return nil
}
