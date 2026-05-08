package template

import (
	"errors"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func newTestManager(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	return NewManager(sys, "templates", nil), sys, root
}

// ─────────────────────────────────────────────────────────────────────
// Validation edge cases
// ─────────────────────────────────────────────────────────────────────

func TestValidate_NilTemplateReportsInvalid(t *testing.T) {
	errs := Validate(nil)
	if len(errs) != 1 || errs[0].Type != "invalid-template" {
		t.Errorf("expected invalid-template, got %+v", errs)
	}
}

func TestValidate_FieldsNilIsInvalid(t *testing.T) {
	errs := Validate(&Template{Name: "X"})
	if len(errs) != 1 || errs[0].Type != "invalid-template" {
		t.Errorf("expected invalid-template for nil Fields, got %+v", errs)
	}
}

func TestValidate_PrimaryKeyMultipleFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "a", Type: "text", PrimaryKey: true},
			{Key: "b", Type: "text", PrimaryKey: true},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "multiple-primary-keys" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected multiple-primary-keys; got %+v", errs)
	}
}

func TestValidate_ApiMapKeyRequired(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x", Map: []APIMap{{Key: ""}}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-map-key-required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-map-key-required; got %+v", errs)
	}
}

func TestValidate_ApiMapDuplicateKeysCaseInsensitive(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "ref", Type: "api", Collection: "x",
				Map: []APIMap{{Key: "Name"}, {Key: "name"}}},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "api-map-duplicate-keys" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected api-map-duplicate-keys; got %+v", errs)
	}
}

func TestValidate_MultipleGuidFieldsFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "alt", Type: "guid"},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "multiple-guid-fields" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected multiple-guid-fields; got %+v", errs)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "id" || got.Keys[1] != "alt" {
		t.Errorf("expected keys [id alt]; got %v", got.Keys)
	}
	if !strings.Contains(got.Message, "id") || !strings.Contains(got.Message, "alt") {
		t.Errorf("message should mention both keys; got %q", got.Message)
	}
}

func TestValidate_SingleGuidFieldIsFine(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "title", Type: "text"},
		},
	})
	for _, e := range errs {
		if e.Type == "multiple-guid-fields" {
			t.Errorf("single guid should not flag; got %+v", errs)
		}
	}
}

func TestValidate_NoGuidFieldIsFine(t *testing.T) {
	// missing-guid-for-collection only fires when EnableCollection is true;
	// a plain template with zero guids must be silent.
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "title", Type: "text"},
		},
	})
	for _, e := range errs {
		if e.Type == "multiple-guid-fields" {
			t.Errorf("no guid should not flag multiple-guid-fields; got %+v", errs)
		}
	}
}

func TestValidate_MultipleGuidWithEmptyKeyUsesPlaceholder(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "", Type: "guid"},
			{Key: "id", Type: "guid"},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "multiple-guid-fields" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected multiple-guid-fields; got %+v", errs)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "(no key)" {
		t.Errorf("empty key should render as placeholder; got %v", got.Keys)
	}
}

func TestValidate_NestedLoopsAtMaxDepthAreFine(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "outer", Type: "loopstart"},
			{Key: "inner", Type: "loopstart"},
			{Key: "x", Type: "text"},
			{Key: "inner", Type: "loopstop"},
			{Key: "outer", Type: "loopstop"},
		},
	})
	for _, e := range errs {
		if e.Type == "excessive-loop-nesting" {
			t.Errorf("depth-2 should not error; got %+v", errs)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// CRUD edge cases
// ─────────────────────────────────────────────────────────────────────

func TestLoadTemplate_RejectsEmptyName(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.LoadTemplate(""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSaveTemplate_RejectsNil(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.SaveTemplate("x.yaml", nil); err == nil {
		t.Fatal("expected error for nil template")
	}
}

func TestSaveTemplate_BackfillsFilename(t *testing.T) {
	m, sys, _ := newTestManager(t)
	tmpl := &Template{Name: "X", Fields: []Field{{Key: "a", Type: "text"}}}
	if err := m.SaveTemplate("x.yaml", tmpl); err != nil {
		t.Fatal(err)
	}
	body, _ := sys.LoadFile("templates/x.yaml")
	if !strings.Contains(body, "filename: x.yaml") {
		t.Errorf("filename not persisted: %q", body)
	}
}

func TestSaveTemplate_RejectsValidationFailure(t *testing.T) {
	m, sys, _ := newTestManager(t)
	tmpl := &Template{
		Name:     "Bad",
		Filename: "bad.yaml",
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "alt", Type: "guid"},
		},
	}
	err := m.SaveTemplate("bad.yaml", tmpl)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var verr *ValidationFailedError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationFailedError, got %T: %v", err, err)
	}
	found := false
	for _, ve := range verr.Errors {
		if ve.Type == "multiple-guid-fields" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected multiple-guid-fields in errors; got %+v", verr.Errors)
	}
	if sys.FileExists("templates/bad.yaml") {
		t.Error("validation failure must not write the file to disk")
	}
}

func TestSaveTemplate_AcceptsEmptyFieldsTemplate(t *testing.T) {
	// Empty templates are valid drafts — the editor creates them when
	// the user hits "New Template" before adding any fields.
	m, sys, _ := newTestManager(t)
	tmpl := &Template{Name: "Draft", Filename: "draft.yaml"}
	if err := m.SaveTemplate("draft.yaml", tmpl); err != nil {
		t.Fatalf("empty template should save: %v", err)
	}
	if !sys.FileExists("templates/draft.yaml") {
		t.Error("file not written")
	}
}

func TestSaveTemplate_DoesNotFireIndexerOnValidationFailure(t *testing.T) {
	m, _, _ := newTestManager(t)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)
	tmpl := &Template{
		Name:     "Bad",
		Filename: "bad.yaml",
		Fields: []Field{
			{Key: "t1", Type: "tags"},
			{Key: "t2", Type: "tags"},
		},
	}
	if err := m.SaveTemplate("bad.yaml", tmpl); err == nil {
		t.Fatal("expected validation error")
	}
	if len(idx.changed) != 0 {
		t.Errorf("indexer fired on validation failure: %v", idx.changed)
	}
}

func TestDeleteTemplate_MissingIsNoOp(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.DeleteTemplate("ghost.yaml"); err != nil {
		t.Errorf("delete missing should be no-op: %v", err)
	}
}

func TestListTemplates_FiltersNonYAML(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/a.yaml", "name: A\nfields: []\n")
	_ = sys.SaveFile("templates/notes.txt", "x")
	files, err := m.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "a.yaml" {
		t.Errorf("filter failed: %v", files)
	}
}

func TestHasTemplates_FalseOnEmptyAndMissingDir(t *testing.T) {
	m, _, _ := newTestManager(t)
	// Directory not yet created.
	if m.HasTemplates() {
		t.Error("expected false for missing templates dir")
	}
}

func TestHasTemplates_TrueAfterYAMLAdded(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/a.yaml", "name: A\nfields: []\n")
	if !m.HasTemplates() {
		t.Error("expected true after adding a.yaml")
	}
}

func TestHasTemplates_IgnoresNonYAML(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/notes.txt", "x")
	if m.HasTemplates() {
		t.Error("expected false when only non-YAML files exist")
	}
}

func TestSeedBasicIfEmpty_PreservesExisting(t *testing.T) {
	m, sys, _ := newTestManager(t)
	_ = sys.SaveFile("templates/other.yaml", "name: Other\nfields: []\n")
	if err := m.SeedBasicIfEmpty(); err != nil {
		t.Fatal(err)
	}
	if sys.FileExists(sys.JoinPath("templates", "basic.yaml")) {
		t.Error("basic.yaml should NOT have been created")
	}
}

func TestTopLevelTextFields_IgnoresNonTextAndLooped(t *testing.T) {
	fields := []Field{
		{Key: "title", Type: "text"},
		{Key: "n", Type: "number"},
		{Key: "items", Type: "loopstart"},
		{Key: "inner", Type: "text"},
		{Key: "items", Type: "loopstop"},
		{Key: "tail", Type: "text", Label: "Tail"},
	}
	got := TopLevelTextFields(fields)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %v", got)
	}
	if got[0].Key != "title" || got[1].Key != "tail" {
		t.Errorf("keys wrong: %v", got)
	}
	if got[1].Label != "Tail" {
		t.Errorf("label not preserved: %v", got[1])
	}
}

func TestYAMLRoundTrip_PreservesCustomFields(t *testing.T) {
	m, sys, _ := newTestManager(t)
	src := `name: Custom
filename: custom.yaml
fields:
  - key: x
    type: text
    custom_prop: hello
`
	_ = sys.SaveFile("templates/custom.yaml", src)
	loaded, err := m.LoadTemplate("custom.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.Fields[0].Extra["custom_prop"]; !ok || got != "hello" {
		t.Errorf("custom_prop not preserved: %+v", loaded.Fields[0].Extra)
	}
}
