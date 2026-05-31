package app

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openInDefaultBrowser is wiki.Service's platform shim for OpenInBrowser.
// Lives in the composition root so the wiki module stays free of os/exec.
//
// Windows uses rundll32 (not `cmd /c start`, which needs an empty title to
// handle URLs with spaces). All commands run detached.
func openInDefaultBrowser(url string) error {
	if url == "" {
		return fmt.Errorf("browser: empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("browser: unsupported platform %q", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("browser: launch %q: %w", runtime.GOOS, err)
	}
	// Don't reap: desktop launchers exec asynchronously.
	return nil
}
