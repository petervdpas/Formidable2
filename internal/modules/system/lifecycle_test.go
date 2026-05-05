package system

import (
	"os"
	"path/filepath"
	"testing"
)

// The spawn-and-quit dance in Restart needs a running Wails app to
// actually verify, which is integration-only. What we CAN check
// without one is that os.Executable() resolves to a real, executable
// file on this platform — that's what restartProcess relies on. If
// the runtime ever broke that contract, our restart path would
// silently fail; this test catches that.

func TestOsExecutable_PointsAtARealFile(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if exe == "" {
		t.Fatalf("empty executable path")
	}
	if !filepath.IsAbs(exe) {
		t.Errorf("executable path not absolute: %q", exe)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat executable: %v", err)
	}
	if info.IsDir() {
		t.Errorf("executable %q is a directory", exe)
	}
}

func TestService_RestartAndQuit_AreCallable(t *testing.T) {
	t.Parallel()
	// The function-pointer dance — confirms a refactor that drops or
	// renames either method shows up in the build.
	m, _ := newTestManager(t)
	s := NewService(m)
	_ = s.Restart
	_ = s.Quit
}
