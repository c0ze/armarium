package config

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ManagedHeader is the first line of a config Armarium writes itself.
const ManagedHeader = "# Written by Armarium. Quit Armarium before editing this file by hand; Armarium rewrites it when you change Settings."

type managedFile struct {
	DataDir string    `toml:"data_dir"`
	Desktop Desktop   `toml:"desktop"`
	Library []Library `toml:"library"`
}

// WriteManaged writes a managed config for s. The old file stays whole until
// the new one is complete.
func WriteManaged(path string, s Settings, dataDir string) error {
	var buf bytes.Buffer
	buf.WriteString(ManagedHeader + "\n\n")
	f := managedFile{
		DataDir: dataDir,
		Desktop: Desktop{Network: s.Network, Tailscale: s.Tailscale, Port: s.Port, PublicURL: s.PublicURL, ExtraHosts: s.ExtraHosts},
		Library: s.Libraries,
	}
	if f.Desktop.Port == 0 {
		f.Desktop.Port = DefaultPort
	}
	if err := toml.NewEncoder(&buf).Encode(f); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".armarium-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	if err := tmp.Chmod(0o600); err != nil && !os.IsPermission(err) {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
