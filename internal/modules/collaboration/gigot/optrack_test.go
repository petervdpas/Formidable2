package gigot

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// A reclone is rejected while another is already tracked as running. The guard
// is the shared registry's, so it releases when the op ends (no app restart).
func TestService_Reclone_RejectedWhileAnotherRuns(t *testing.T) {
	s := NewService(nil, nil, nil, nil, nil)
	reg := optrack.NewRegistry()
	AttachOps(s, reg)

	reg.Begin("gigot:reclone") // pretend one is already in flight
	if _, err := s.Reclone(); !errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Fatalf("reclone must be rejected while another runs, got %v", err)
	}
}

// Without a registry, Reclone is unguarded and proceeds past the guard (it then
// fails on the missing connection, which is fine: the guard did not block it).
func TestService_Reclone_NilRegistryNotBlocked(t *testing.T) {
	s := NewService(nil, nil, nil, nil, nil)
	_, err := s.Reclone()
	if err == nil {
		t.Fatal("expected a connection error, not a nil result")
	}
	if errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Errorf("nil registry must not block the reclone: %v", err)
	}
}
