//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is Windows' CREATE_NO_WINDOW process-creation flag: run the
// console child without allocating a console for it.
const createNoWindow = 0x08000000

// HideWindow stops a console child (git.exe, cmd /C, a --version probe) from
// flashing a terminal window. The GUI app has no console of its own, so Windows
// would otherwise allocate a fresh one for every spawned console program.
func HideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
