//go:build !windows

package proc

import "os/exec"

// HideWindow is a no-op off Windows: a GUI parent there doesn't allocate a
// console for its children, so there's no window to suppress.
func HideWindow(*exec.Cmd) {}
