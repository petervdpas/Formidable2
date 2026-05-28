package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestPOST_API_WriteIsJournaled is the regression-test version of the
// "you can't write to disk without going through the journal" claim.
//
// It assembles the API handler against the REAL system / sfr / template /
// storage / journal stack (only the dataprovider is stubbed because it
// would otherwise drag in SQLite). After a single POST the journal must
// have one pending entry for the freshly-written form file. If a future
// refactor introduces a Writer impl that bypasses system.Manager, this
// test fails - that's the contract we're locking.
func TestPOST_API_WriteIsJournaled(t *testing.T) {
	root := t.TempDir()

	// Lay out a context folder the journal recognises: templates/ and
	// storage/ at the root, plus a recepten template the API handler
	// can resolve.
	tplDir := filepath.Join(root, "templates")
	stoDir := filepath.Join(root, "storage")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := strings.Join([]string{
		`name: Recepten`,
		`filename: recepten.yaml`,
		`enable_collection: true`,
		`fields:`,
		`  - key: guid`,
		`    type: guid`,
		`  - key: naam`,
		`    type: text`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(tplDir, "recepten.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real composition: system.Manager → journal.Manager wired as the
	// FS mutation emitter, journal configured with backend "git" so it
	// actually accumulates pending changes.
	sysM := system.NewManager(root, nil)
	jrnM := journal.NewManager(sysM, nil, nil)
	sysM.SetJournal(jrnM)
	if err := jrnM.Configure(root, "git"); err != nil {
		t.Fatalf("journal Configure: %v", err)
	}
	if pr := jrnM.Pending("git"); pr.Count != 0 {
		t.Fatalf("preconditions: pending must be empty, got %+v", pr)
	}

	// Real sfr / template / storage chain. storage.Manager satisfies
	// the api.Writer interface - that's the choke point this test
	// proves can't be bypassed.
	sfrM := sfr.NewManager(sysM, nil)
	tplM := template.NewManager(sysM, tplDir, nil)
	stoM := storage.NewManager(sysM, sfrM, tplM, stoDir, nil)

	// Stub Provider - same hand-rolled type the unit tests use. The
	// API only needs IsCollectionEnabled=true and an empty existing
	// set so the POST resolves to a new item.
	sp := &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "recepten", Filename: "recepten.yaml", Name: "Recepten",
				EnableCollection: true, GuidField: "guid"},
		},
		forms: map[string][]dataprovider.FormSummary{},
	}

	h := NewHandler(sp, stoM, stoM, tplM, nil, nil)

	// Single POST → 201.
	req := httptest.NewRequest(http.MethodPost, "/api/collections/recepten",
		strings.NewReader(`{"data":{"naam":"Pasta"}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}

	// Contract: the journal must now hold exactly one pending entry,
	// for the new form file under storage/recepten/.
	pr := jrnM.Pending("git")
	if pr.Count != 1 {
		t.Fatalf("expected exactly 1 pending entry after API POST, got %d (%+v)", pr.Count, pr.Paths)
	}
	got := pr.Paths[0]
	if got.Op != "create" {
		t.Errorf("pending op = %q, want \"create\"", got.Op)
	}
	if !strings.HasPrefix(got.Path, "storage/recepten/") {
		t.Errorf("pending path = %q, want prefix storage/recepten/", got.Path)
	}
	if !strings.HasSuffix(got.Path, ".meta.json") {
		t.Errorf("pending path = %q, want .meta.json suffix", got.Path)
	}
}

// TestDELETE_API_WriteIsJournaled mirrors the POST contract on the
// delete path: API DELETE → journal sees a "delete" entry.
func TestDELETE_API_WriteIsJournaled(t *testing.T) {
	root := t.TempDir()
	tplDir := filepath.Join(root, "templates")
	stoDir := filepath.Join(root, "storage", "recepten")
	if err := os.MkdirAll(tplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := strings.Join([]string{
		`name: Recepten`,
		`filename: recepten.yaml`,
		`enable_collection: true`,
		`fields:`,
		`  - key: guid`,
		`    type: guid`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(tplDir, "recepten.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing form so DELETE has something to remove. Written
	// directly via os.WriteFile (NOT system.Manager) so the journal
	// stays empty until the API call runs - otherwise we'd be
	// measuring our own setup.
	formPath := filepath.Join(stoDir, "brood.meta.json")
	if err := os.WriteFile(formPath, []byte(`{"id":"g-abc","data":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	sysM := system.NewManager(root, nil)
	jrnM := journal.NewManager(sysM, nil, nil)
	sysM.SetJournal(jrnM)
	if err := jrnM.Configure(root, "git"); err != nil {
		t.Fatalf("journal Configure: %v", err)
	}
	// Configure ran; pending must be empty (we wrote the seed file via
	// raw os.WriteFile, not via system.Manager, so it's invisible to
	// the journal).
	if pr := jrnM.Pending("git"); pr.Count != 0 {
		t.Fatalf("preconditions: pending must be empty, got %+v", pr)
	}

	sfrM := sfr.NewManager(sysM, nil)
	tplM := template.NewManager(sysM, tplDir, nil)
	stoM := storage.NewManager(sysM, sfrM, tplM, filepath.Join(root, "storage"), nil)

	sp := &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "recepten", Filename: "recepten.yaml", Name: "Recepten",
				EnableCollection: true, GuidField: "guid"},
		},
		forms: map[string][]dataprovider.FormSummary{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "brood.meta.json", ID: "g-abc", Title: "Brood"},
			},
		},
	}

	h := NewHandler(sp, stoM, stoM, tplM, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/collections/recepten/g-abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}

	pr := jrnM.Pending("git")
	if pr.Count != 1 {
		t.Fatalf("expected 1 pending entry after API DELETE, got %d (%+v)", pr.Count, pr.Paths)
	}
	if pr.Paths[0].Op != "delete" {
		t.Errorf("pending op = %q, want \"delete\"", pr.Paths[0].Op)
	}
	if pr.Paths[0].Path != "storage/recepten/brood.meta.json" {
		t.Errorf("pending path = %q, want storage/recepten/brood.meta.json", pr.Paths[0].Path)
	}
}
