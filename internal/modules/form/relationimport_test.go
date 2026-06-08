package form

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// importHarness wires a form Manager with an api-field template, a record
// resolver over the fake storage, and a capturing edge syncer.
func importHarness(t *testing.T) (*Manager, *fakeStorage, *fakeEdgeSyncer) {
	t.Helper()
	m, tpls, store, _ := newTestManager()
	tpls.byName["applicatie.yaml"] = &template.Template{
		Filename: "applicatie.yaml",
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "naam", Type: "text"},
			{Key: "applicatiefuncties", Type: "api", Collection: "applicatie-functie.yaml"},
		},
	}
	// Source records (applicatie) keyed guid -> datafile.
	src := map[string]string{"app-1": "app-1.meta.json", "app-2": "app-2.meta.json"}
	store.forms["applicatie.yaml"] = map[string]*storage.Form{
		"app-1.meta.json": {Meta: storage.FormMeta{ID: "app-1"}, Data: map[string]any{"id": "app-1", "naam": "A1"}},
		"app-2.meta.json": {Meta: storage.FormMeta{ID: "app-2"}, Data: map[string]any{"id": "app-2", "naam": "A2"}},
	}
	// Target records (functions) that exist.
	fns := map[string]string{"fn-1": "fn-1.meta.json", "fn-2": "fn-2.meta.json"}

	m.SetRecordResolver(func(tpl, guid string) (string, bool) {
		switch tpl {
		case "applicatie.yaml":
			df, ok := src[guid]
			return df, ok
		case "applicatie-functie.yaml":
			df, ok := fns[guid]
			return df, ok
		}
		return "", false
	})
	sync := &fakeEdgeSyncer{}
	m.SetReferenceEdgeSyncer(sync)
	return m, store, sync
}

func TestImportRelationEdges_GroupsAndSyncs(t *testing.T) {
	m, store, sync := importHarness(t)
	res, err := m.ImportRelationEdges("applicatie.yaml", "applicatiefuncties", []EdgePair{
		{From: "app-1", To: "fn-1"},
		{From: "app-1", To: "fn-2"},
		{From: "app-2", To: "fn-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 2 || res.Linked != 3 {
		t.Fatalf("want Records=2 Linked=3, got %+v", res)
	}
	// app-1's api value must hold both function ids, persisted.
	got := store.forms["applicatie.yaml"]["app-1.meta.json"].Data["applicatiefuncties"]
	ids, ok := got.([]string)
	if !ok || len(ids) != 2 {
		t.Fatalf("app-1 api value wrong: %#v", got)
	}
	// The edge syncer must have run for each touched record.
	if len(sync.calls) != 2 {
		t.Errorf("want 2 edge-sync calls, got %d", len(sync.calls))
	}
}

func TestImportRelationEdges_MergesWithExisting(t *testing.T) {
	m, store, _ := importHarness(t)
	// app-1 already links fn-1.
	store.forms["applicatie.yaml"]["app-1.meta.json"].Data["applicatiefuncties"] = []any{"fn-1"}
	res, err := m.ImportRelationEdges("applicatie.yaml", "applicatiefuncties", []EdgePair{
		{From: "app-1", To: "fn-1"}, // duplicate, must not double
		{From: "app-1", To: "fn-2"}, // new
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 1 {
		t.Fatalf("want Linked=1 (only fn-2 is new), got %+v", res)
	}
	ids := store.forms["applicatie.yaml"]["app-1.meta.json"].Data["applicatiefuncties"].([]string)
	if len(ids) != 2 {
		t.Fatalf("want 2 unique ids after merge, got %v", ids)
	}
}

func TestImportRelationEdges_Idempotent(t *testing.T) {
	m, _, _ := importHarness(t)
	pairs := []EdgePair{{From: "app-1", To: "fn-1"}, {From: "app-1", To: "fn-2"}}
	if _, err := m.ImportRelationEdges("applicatie.yaml", "applicatiefuncties", pairs); err != nil {
		t.Fatal(err)
	}
	res, err := m.ImportRelationEdges("applicatie.yaml", "applicatiefuncties", pairs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 0 {
		t.Errorf("re-run should add nothing, got Linked=%d", res.Linked)
	}
}

func TestImportRelationEdges_SkipsMissingEndpoints(t *testing.T) {
	m, _, _ := importHarness(t)
	res, err := m.ImportRelationEdges("applicatie.yaml", "applicatiefuncties", []EdgePair{
		{From: "app-1", To: "ghost"}, // missing target
		{From: "ghost", To: "fn-1"},  // missing source
		{From: "app-1", To: "fn-1"},  // good
		{From: "", To: "fn-2"},       // empty, ignored
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.MissingTo != 1 {
		t.Errorf("want MissingTo=1, got %+v", res)
	}
	if res.MissingFrom != 1 {
		t.Errorf("want MissingFrom=1, got %+v", res)
	}
	if res.Linked != 1 || res.Records != 1 {
		t.Errorf("want Linked=1 Records=1, got %+v", res)
	}
}

func TestImportRelationEdges_RejectsNonAPIField(t *testing.T) {
	m, _, _ := importHarness(t)
	if _, err := m.ImportRelationEdges("applicatie.yaml", "naam", nil); err == nil {
		t.Error("expected error mapping to a non-api field")
	}
}

func TestImportRelationEdges_RejectsUnknownField(t *testing.T) {
	m, _, _ := importHarness(t)
	if _, err := m.ImportRelationEdges("applicatie.yaml", "nope", nil); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestImportRelationEdges_ResolverNotWired(t *testing.T) {
	m, tpls, _, _ := newTestManager()
	tpls.byName["t.yaml"] = &template.Template{
		Filename: "t.yaml",
		Fields:   []template.Field{{Key: "ref", Type: "api", Collection: "x.yaml"}},
	}
	if _, err := m.ImportRelationEdges("t.yaml", "ref", []EdgePair{{From: "a", To: "b"}}); err == nil {
		t.Error("expected error when resolver not wired")
	}
}
