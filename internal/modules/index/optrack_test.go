package index

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// A reindex of a template refuses to start while another reindex of the same
// template is in flight; a different template is guarded independently.
func TestService_RescanTemplate_RejectedWhileSameTemplateRuns(t *testing.T) {
	s := NewService(nil)
	reg := optrack.NewRegistry()
	AttachOps(s, reg)

	reg.Begin("index:rescan:report.yaml") // pretend one is already in flight
	if err := s.RescanTemplate("report.yaml"); !errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Fatalf("reindex must be rejected while the same template runs, got %v", err)
	}
}
