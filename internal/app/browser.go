package app

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openInDefaultBrowser is the platform shim wiki.Service uses for its
// OpenInBrowser method. Lives in the composition root so the wiki
// module stays free of os/exec - keeps the surface unit-testable in
// pure Go and the OS-specific commands in one obvious place.
//
// Linux: xdg-open <url>  - provided by xdg-utils on every desktop
// macOS: open <url>      - system-supplied
// Windows: rundll32 url.dll,FileProtocolHandler <url>  - works under
//   cmd shells and PowerShell without quoting issues; `cmd /c start`
//   needs an empty title to handle URLs with spaces.
//
// All commands run detached: we don't wait or capture output.
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
	// Don't reap - desktop launchers exec asynchronously and may stay
	// alive longer than this process needs to wait for them.
	return nil
}
