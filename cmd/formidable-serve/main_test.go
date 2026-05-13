package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeBinaryAdvertisesNotImplemented builds the stub and runs it,
// asserting the marker message and the non-zero exit. Locks in the
// "directive but inert" posture so a refactor can't silently turn this
// into a half-functional daemon.
func TestSmokeBinaryAdvertisesNotImplemented(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "formidable-serve")

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}

	run := exec.Command(binPath)
	out, err := run.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit, got success")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("unexpected error type: %T %v", err, err)
	}

	got := string(out)
	for _, want := range []string{
		"formidable-serve: not yet implemented",
		"internal/modules/auth",
		"SubscriptionResolver",
		"LoopbackOnly",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
}
