package git

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// Each long git op refuses to start while another of its kind is in flight, and
// the guard releases when the op ends (shared registry, no app restart needed).
func TestService_GitOps_RejectedWhileSameKindRuns(t *testing.T) {
	cases := []struct {
		kind string
		call func(s *Service) error
	}{
		{"git:clone", func(s *Service) error { _, err := s.Clone(CloneOptions{}); return err }},
		{"git:commit", func(s *Service) error { _, err := s.Commit(CommitOptions{}); return err }},
		{"git:push", func(s *Service) error { _, err := s.Push(PushOptions{}); return err }},
		{"git:pull", func(s *Service) error { _, err := s.Pull(PullOptions{}); return err }},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			s := NewService(NewManager(), nil, nil, nil)
			reg := optrack.NewRegistry()
			AttachOps(s, reg)
			reg.Begin(c.kind) // pretend one is already in flight
			if err := c.call(s); !errors.Is(err, optrack.ErrAlreadyRunning) {
				t.Fatalf("%s must be rejected while another runs, got %v", c.kind, err)
			}
		})
	}
}

// The auto-stash pull shares the git:pull kind: it cannot run while a plain
// pull (or another stash pull) is in flight.
func TestService_PullWithStash_RejectedWhilePullRuns(t *testing.T) {
	s := NewService(NewManager(), nil, nil, nil)
	reg := optrack.NewRegistry()
	AttachOps(s, reg)
	reg.Begin("git:pull") // a pull is already in flight
	if _, err := s.PullWithStash(PullOptions{}); !errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Fatalf("stash pull must be rejected while a pull runs, got %v", err)
	}
}

// Without a registry a git op is unguarded: it proceeds past the guard and
// fails on the real git error, never on ErrAlreadyRunning.
func TestService_Clone_NilRegistryNotBlocked(t *testing.T) {
	s := NewService(NewManager(), nil, nil, nil)
	_, err := s.Clone(CloneOptions{})
	if err == nil {
		t.Fatal("expected a real clone error, not a nil result")
	}
	if errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Errorf("nil registry must not block the clone: %v", err)
	}
}
