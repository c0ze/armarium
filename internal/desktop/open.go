package desktop

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, url)
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd.Start()
}

// OpenFolder shows a folder in Finder or Explorer.
func OpenFolder(dir string) error {
	name := "open"
	if runtime.GOOS == "windows" {
		name = "explorer"
	} else if runtime.GOOS != "darwin" {
		name = "xdg-open"
	}
	return exec.Command(name, dir).Start()
}

func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
