package pdf

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// A PDF export refuses to start while another is in flight; the guard is the
// shared registry's, so it releases when the export ends (no app restart).
func TestService_ExportPDF_RejectedWhileAnotherRuns(t *testing.T) {
	s := NewService(NewManager(nil, nil, nil, nil, nil, nil))
	reg := optrack.NewRegistry()
	AttachOps(s, reg)

	reg.Begin("pdf:export") // pretend one is already in flight
	if _, err := s.ExportPDF("t.yaml", "d.md", ExportOpts{}); !errors.Is(err, optrack.ErrAlreadyRunning) {
		t.Fatalf("export must be rejected while another runs, got %v", err)
	}
}
