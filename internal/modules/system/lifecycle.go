package system

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// quitDelay is the small grace window between Restart returning and
// the process actually quitting, so the Wails RPC reply reaches the
// frontend (and its dialogs/buttons can settle) before the connection
// drops. 200 ms is invisible to the user but reliably long enough.
const quitDelay = 200 * time.Millisecond

// Restart spawns a fresh copy of this binary with the same arguments,
// then quits the current process after a short grace period. Used by
// the "Apply changes & restart" button in Settings when the user has
// changed a setting that only takes effect at launch (today: window
// size).
//
// On Linux/macOS the spawned child is detached from the parent's
// terminal/process group as soon as the parent exits - Go's runtime
// re-parents to PID 1 (init/launchd). On Windows the child gets a
// fresh console attachment by default. Either way, no orphaned
// goroutines or zombie processes are left behind.
//
// Returns nil if the spawn succeeds; the actual quit happens
// asynchronously, so callers should treat success as "restart is in
// progress."
func (s *Service) Restart() error { return restartProcess() }

// Quit is the soft-shutdown counterpart - used by future File → Quit
// menu wiring. Exposed today so the Wails surface stays stable when
// the menu lands.
func (s *Service) Quit() {
	go func() {
		time.Sleep(quitDelay)
		application.Get().Quit()
	}()
}

// restartProcess is split out so it can be exercised by name in
// tests (the spawn is integration-only, but the path-resolution logic
// is unit-testable separately).
func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	// Detach stdio so the child isn't tied to our pipes after we exit.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	// Schedule the quit asynchronously so this RPC call returns first.
	go func() {
		time.Sleep(quitDelay)
		application.Get().Quit()
	}()
	return nil
}
