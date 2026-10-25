// Package desktop runs Armarium as a desktop app: the server in-process on a
// config Armarium manages, restarted when Settings change.
package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Paths are where the desktop app keeps its config, database and logs.
type Paths struct {
	Config string // armarium.toml
	Data   string // database and cover cache
	Logs   string
}

// DefaultPaths are the per-user folders of this platform.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return pathsFor(runtime.GOOS, os.Getenv, home)
}

func pathsFor(goos string, getenv func(string) string, home string) (Paths, error) {
	var base, logs string
	switch goos {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support", "Armarium")
		logs = filepath.Join(home, "Library", "Logs", "Armarium")
	case "windows":
		local := getenv("LOCALAPPDATA")
		if local == "" {
			return Paths{}, errors.New("LOCALAPPDATA is not set")
		}
		base = filepath.Join(local, "Armarium")
		logs = filepath.Join(base, "logs")
	default:
		base = filepath.Join(home, ".config", "armarium")
		logs = filepath.Join(base, "logs")
	}
	return Paths{Config: filepath.Join(base, "armarium.toml"), Data: filepath.Join(base, "data"), Logs: logs}, nil
}
