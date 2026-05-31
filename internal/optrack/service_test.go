package optrack

import "testing"

// Active exposes the running ops so any frontend view can reflect or resume
// them on reload, for every long-running process, not just one backend.
func TestService_Active_ReturnsRunningOps(t *testing.T) {
	reg := NewRegistry()
	reg.Begin("gigot:reclone")
	reg.Begin("pdf:export")

	got := NewService(reg).Active()
	if len(got) != 2 || got[0].Kind != "gigot:reclone" || got[1].Kind != "pdf:export" {
		t.Errorf("Active should list every running op, got %+v", got)
	}
}

func TestService_Active_NilRegistry(t *testing.T) {
	if got := NewService(nil).Active(); got != nil {
		t.Errorf("a nil registry yields no active ops, got %+v", got)
	}
}
