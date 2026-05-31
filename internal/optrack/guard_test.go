package optrack

import (
	"errors"
	"testing"
)

// A nil registry is unguarded: Guard returns no handle, a callable no-op
// release, and no error, so a caller stays one shape whether wired or not.
func TestGuard_NilRegistry(t *testing.T) {
	h, release, err := Guard(nil, "git:push")
	if err != nil {
		t.Fatalf("nil registry must not error: %v", err)
	}
	if h != nil {
		t.Errorf("nil registry yields no handle, got %+v", h)
	}
	release() // must not panic
}

// The first Guard of a kind registers it; the release func removes it.
func TestGuard_FirstRegistersAndReleases(t *testing.T) {
	reg := NewRegistry()
	h, release, err := Guard(reg, "git:clone")
	if err != nil || h == nil {
		t.Fatalf("first guard should succeed, got h=%v err=%v", h, err)
	}
	if got := reg.List(); len(got) != 1 || got[0].Kind != "git:clone" {
		t.Fatalf("op should be tracked, got %+v", got)
	}
	release()
	if got := reg.List(); len(got) != 0 {
		t.Errorf("release should remove the op, got %+v", got)
	}
}

// A second Guard of the same in-flight kind is refused with ErrAlreadyRunning;
// a different kind is unaffected.
func TestGuard_SecondSameKindRejected(t *testing.T) {
	reg := NewRegistry()
	if _, _, err := Guard(reg, "git:pull"); err != nil {
		t.Fatalf("first guard should succeed: %v", err)
	}
	_, _, err := Guard(reg, "git:pull")
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second same-kind guard must be ErrAlreadyRunning, got %v", err)
	}
	if _, _, err := Guard(reg, "git:push"); err != nil {
		t.Errorf("a different kind must not be blocked: %v", err)
	}
}

// Releasing lets the same kind run again: no app restart needed, the guard
// cannot get stuck open.
func TestGuard_ReleaseAllowsRerun(t *testing.T) {
	reg := NewRegistry()
	_, release, _ := Guard(reg, "pdf:export")
	release()
	if _, _, err := Guard(reg, "pdf:export"); err != nil {
		t.Errorf("after release the kind must run again, got %v", err)
	}
}
