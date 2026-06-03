//go:build !windows

package proc

import (
	"os/exec"
	"testing"
)

// Off Windows HideWindow must be inert: no SysProcAttr is set, so the command
// runs exactly as the caller built it.
func TestHideWindow_NoOpOffWindows(t *testing.T) {
	cmd := exec.Command("git", "status")
	HideWindow(cmd)
	if cmd.SysProcAttr != nil {
		t.Errorf("SysProcAttr = %+v, want nil off Windows", cmd.SysProcAttr)
	}
}

// A nil command must not panic (defensive: callers build the command before
// the guard runs).
func TestHideWindow_NilCommandIsSafe(t *testing.T) {
	HideWindow(nil)
}
