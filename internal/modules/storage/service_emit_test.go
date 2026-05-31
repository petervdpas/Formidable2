package storage

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

type recordingEmitter struct {
	names []string
	data  []any
}

func (r *recordingEmitter) Emit(name string, data any) {
	r.names = append(r.names, name)
	r.data = append(r.data, data)
}

func facetTemplate(t *testing.T, tplM *template.Manager) {
	t.Helper()
	if err := tplM.SaveTemplate("gaps.yaml", &template.Template{
		Name: "gaps", Filename: "gaps.yaml",
		Facets: []template.Facet{{Key: "status", Icon: "fa-flag", Options: []template.FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "status-field", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

const newShapeNoFacets = `{"meta":{"id":"f1","template":"gaps",` +
	`"created":{"at":"2026-01-01T00:00:00Z","name":"P","email":"p@x"},` +
	`"updated":{"at":"2026-01-01T00:00:00Z","name":"P","email":"p@x"}},"data":{"title":"Hi"}}`

// A rewrite (a missing facet default seeded onto disk) must announce storage:changed
// so the frontend reloads the affected forms instead of showing a stale view.
func TestService_MigrateTemplateMeta_EmitsStorageChangedOnRewrite(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	facetTemplate(t, tplM)
	if err := sys.SaveFile("storage/gaps/f1.meta.json", newShapeNoFacets); err != nil {
		t.Fatal(err)
	}

	fe := &recordingEmitter{}
	res, err := NewService(m, fe).MigrateTemplateMeta("gaps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res.Migrated != 1 {
		t.Fatalf("migrated = %d, want 1", res.Migrated)
	}
	if len(fe.names) != 1 || fe.names[0] != "storage:changed" || fe.data[0] != "gaps.yaml" {
		t.Errorf("emitted %v / %v, want one storage:changed for gaps.yaml", fe.names, fe.data)
	}
}

// A no-op migration (nothing rewritten) must NOT emit, so the user's view is not
// needlessly reloaded.
func TestService_MigrateTemplateMeta_NoEmitWhenNothingChanged(t *testing.T) {
	m, sys, tplM, _ := newTestStack(t)
	if err := tplM.SaveTemplate("gaps.yaml", &template.Template{
		Name: "gaps", Filename: "gaps.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sys.SaveFile("storage/gaps/f1.meta.json", newShapeNoFacets); err != nil {
		t.Fatal(err)
	}

	fe := &recordingEmitter{}
	res, err := NewService(m, fe).MigrateTemplateMeta("gaps.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res.Migrated != 0 {
		t.Fatalf("migrated = %d, want 0", res.Migrated)
	}
	if len(fe.names) != 0 {
		t.Errorf("emitted %v, want nothing on a no-op migration", fe.names)
	}
}
