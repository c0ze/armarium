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

// ServeState is how `tailscale serve` stands on https:443 for Armarium.
type ServeState struct {
	Owned    bool   // "/" proxies to Armarium
	Conflict string // why Armarium won't put itself on "/", "" when it may
}

// Conflict is what the owner reads when "/" on https:443 is taken.
const Conflict = "Tailscale already serves something else at the root of this computer's address. Armarium left it alone."

// Serve reports the serve setup for the Armarium at target
// (http://127.0.0.1:<port>).
func Serve(ctx context.Context, target string) (ServeState, error) {
	out, err := run(ctx, "serve", "status", "--json")
	if err != nil {
		return ServeState{}, err
	}
	return parseServe(out, target)
}

// parseServe reads `tailscale serve status --json`. Other paths on 443 (such
// as /tincan) can share the address; only "/" or a raw TCP forward on 443
// is in the way.
func parseServe(statusJSON []byte, target string) (ServeState, error) {
	var st struct {
		TCP map[string]struct {
			HTTPS      bool
			TCPForward string
		}
		Web map[string]struct {
			Handlers map[string]struct{ Proxy, Path, Text string }
		}
	}
	if err := json.Unmarshal(statusJSON, &st); err != nil {
		return ServeState{}, err
	}
	if tcp, ok := st.TCP["443"]; ok && tcp.TCPForward != "" {
		return ServeState{Conflict: Conflict}, nil
	}
	for hostport, web := range st.Web {
		if !strings.HasSuffix(hostport, ":443") {
			continue
		}
		if h, ok := web.Handlers["/"]; ok {
			if h.Proxy == target {
				return ServeState{Owned: true}, nil
			}
			return ServeState{Conflict: Conflict}, nil
		}
	}
	return ServeState{}, nil
}

// ServeOn puts the Armarium at target on "/" of https://<name>, leaving any
// other paths as they are.
func ServeOn(ctx context.Context, target string) error {
	_, err := run(ctx, "serve", "--bg", "--https=443", "--set-path=/", target)
	return err
}

// ServeOff removes "/" from https:443; call it only when Armarium owns it.
func ServeOff(ctx context.Context) error {
	_, err := run(ctx, "serve", "--https=443", "--set-path=/", "off")
	return err
}

// ServeCommand is the command an owner runs by hand when ServeOn fails.
func ServeCommand(target string) string {
	return "tailscale serve --bg --https=443 --set-path=/ " + target
}
