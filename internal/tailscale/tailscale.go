// Package tailscale talks to the Tailscale command-line client: this
// machine's tailnet name and `tailscale serve` for Armarium's port.
package tailscale

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CLI returns the path of the tailscale command, "" when it isn't installed.
func CLI() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale"}
	case "windows":
		candidates = []string{filepath.Join(os.Getenv("ProgramFiles"), "Tailscale", "tailscale.exe")}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	cli := CLI()
	if cli == "" {
		return nil, exec.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, args...)
	hideWindow(cmd)
	return cmd.Output()
}

// Name is this machine's MagicDNS name (machine.tailnet.ts.net), "" when
// Tailscale is missing, stopped or logged out.
func Name(ctx context.Context) string {
	out, err := run(ctx, "status", "--json")
	if err != nil {
		return ""
	}
	return parseName(out)
}

func parseName(statusJSON []byte) string {
	var st struct {
		BackendState string
		Self         struct{ DNSName string }
	}
	if json.Unmarshal(statusJSON, &st) != nil || st.BackendState != "Running" {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(st.Self.DNSName, "."))
}
