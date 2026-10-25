package desktop

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/c0ze/armarium/internal/httpapi"
)

// chooseFolder shows the macOS folder picker in front of other windows.
func chooseFolder(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript",
		"-e", "tell me to activate",
		"-e", `POSIX path of (choose folder with prompt "Choose a folder for Armarium")`).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "-128") { // User canceled.
			return "", httpapi.ErrCancelled
		}
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path, nil
}
