package system

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// quitDelay is the grace window between Restart returning and the process
// quitting, so the Wails RPC reply reaches the frontend before the connection
// drops. Invisible to the user but reliably long enough.
const quitDelay = 200 * time.Millisecond

// Restart spawns a fresh copy of this binary with the same arguments, then
// quits the current process after quitDelay. Used by Settings' "Apply changes
// & restart" for settings that only take effect at launch (today: window size).
//
// The spawned child is detached: on Linux/macOS Go's runtime re-parents it to
// PID 1 once the parent exits; on Windows it gets a fresh console attachment.
// No orphaned goroutines or zombies are left behind.
//
// Returns nil if the spawn succeeds; the quit happens asynchronously, so
// success means "restart is in progress."
func (s *Service) Restart() error { return restartProcess() }

// Quit is the soft-shutdown counterpart, for future File > Quit menu wiring.
func (s *Service) Quit() {
	go func() {
		time.Sleep(quitDelay)
		application.Get().Quit()
	}()
}

func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	go func() {
		time.Sleep(quitDelay)
		application.Get().Quit()
	}()
	return nil
}
