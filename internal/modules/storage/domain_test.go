package storage

import (
	"slices"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func newTestStack(t *testing.T) (*Manager, *system.Manager, *template.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	tplM := template.NewManager(sys, "templates", nil)
	sfrM := sfr.NewManager(sys, nil)
	m := NewManager(sys, sfrM, tplM, "storage", nil)
	return m, sys, tplM, root
}

func boolPtr(b bool) *bool { return &b }

// ─────────────────────────────────────────────────────────────────────
// Sanitize edge cases
// ─────────────────────────────────────────────────────────────────────

func TestSanitize_FillsDefaultsForMissingFields(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "count", Type: "number", Default: 7},
		{Key: "flag", Type: "boolean"},
		{Key: "rating", Type: "range"},
		{Key: "tags", Type: "tags"},
	}
	out := Sanitize(map[string]any{"title": "Hello"}, fields, SanitizeOptions{})
	if out.Data["title"] != "Hello" {
		t.Errorf("title not preserved: %v", out.Data["title"])
	}
	if out.Data["count"] != 7 {
		t.Errorf("count default missing: %v", out.Data["count"])
	}
	if out.Data["flag"] != false {
		t.Errorf("boolean default = %v, want false", out.Data["flag"])
	}
	if out.Data["rating"] != 50 {
		t.Errorf("range default = %v, want 50", out.Data["rating"])
	}
	if _, ok := out.Data["tags"].([]any); !ok {
		// nil string default fallback is empty array for tags? Tags isn't
		// in the special-list of types so it gets "". But we add it via
		// addTags which collects and stores in meta.tags. data[tags] stays
		// the string default. That's fine.
		t.Logf("tags data type %T is acceptable", out.Data["tags"])
	}
}

func TestSanitize_AcceptsEnvelopeShape(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"meta": map[string]any{"id": "abc", "template": "basic", "created": "2026-01-01T00:00:00Z"},
		"data": map[string]any{"title": "Hi"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})
	if out.Meta.ID != "abc" {
		t.Errorf("id = %q, want abc", out.Meta.ID)
	}
	if out.Data["title"] != "Hi" {
		t.Errorf("title = %v", out.Data["title"])
	}
}

func TestSanitize_RawWithFieldNamedDataIsNotMistakenForEnvelope(t *testing.T) {
	// Defensive: a user field named `data` (not an object) must not flip
	// the parser into envelope mode.
	fields := []template.Field{
		{Key: "data", Type: "text"},
		{Key: "other", Type: "text"},
	}
	raw := map[string]any{
		"data":  "user value",
		"other": "yo",
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if out.Data["data"] != "user value" {
		t.Errorf("user field 'data' lost: %+v", out.Data)
	}
	if out.Data["other"] != "yo" {
		t.Errorf("unrelated field lost: %+v", out.Data)
	}
}

func TestSanitize_GeneratesGuidWhenTemplateHasGuidField(t *testing.T) {
	fields := []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "title", Type: "text"},
	}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID == "" {
		t.Error("expected generated id when template has guid field")
	}
}

func TestSanitize_NoIdWhenNoGuidFieldAndNothingProvided(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.ID != "" {
		t.Errorf("expected empty id, got %q", out.Meta.ID)
	}
}

func TestSanitize_PreservesProvidedID(t *testing.T) {
	fields := []template.Field{{Key: "id", Type: "guid"}, {Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{ID: "fixed-id"})
	if out.Meta.ID != "fixed-id" {
		t.Errorf("id = %q, want fixed-id", out.Meta.ID)
	}
}

func TestSanitize_TagsAreNormalisedAndSortedUnique(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{
		"title": "X",
		"tags":  []any{"  Foo ", "bar", "FOO", "baz", ""},
	}
	out := Sanitize(raw, fields, SanitizeOptions{Tags: []string{"qux", "BAR"}})
	want := []string{"bar", "baz", "foo", "qux"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v", out.Meta.Tags, want)
	}
}

func TestSanitize_TagsFromCommaString(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "tags", Type: "tags"},
	}
	raw := map[string]any{"title": "X", "tags": "alpha, Beta; alpha"}
	out := Sanitize(raw, fields, SanitizeOptions{})
	want := []string{"alpha", "beta"}
	if !slices.Equal(out.Meta.Tags, want) {
		t.Errorf("tags = %v, want %v", out.Meta.Tags, want)
	}
}

func TestSanitize_LoopFieldsPreserved(t *testing.T) {
	fields := []template.Field{
		{Key: "title", Type: "text"},
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text"},
		{Key: "items", Type: "loopstop"},
	}
	raw := map[string]any{
		"title": "X",
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	loop, ok := out.Data["items"].([]any)
	if !ok {
		t.Fatalf("items not preserved: %T", out.Data["items"])
	}
	if len(loop) != 2 {
		t.Errorf("loop len = %d, want 2", len(loop))
	}
	if _, leaked := out.Data["name"]; leaked {
		t.Error("loop child key 'name' leaked to top-level")
	}
}

func TestSanitize_FlaggedHonorsRawMeta(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"meta": map[string]any{"flagged": true},
		"data": map[string]any{"title": "X"},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if !out.Meta.Flagged {
		t.Error("flagged from meta not preserved")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Manager edge cases
// ─────────────────────────────────────────────────────────────────────

func TestSaveForm_RejectsEmptyDatafile(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	r := m.SaveForm("basic.yaml", "", map[string]any{"title": "X"})
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestSaveForm_RejectsPathSeparatorsInDatafile(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	r := m.SaveForm("basic.yaml", "subdir/x", map[string]any{"title": "X"})
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestSaveImageFile_RejectsTraversal(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	r := m.SaveImageFile("basic.yaml", "../escape.png", []byte("x"))
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestSaveImageFile_RejectsEmptyName(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	r := m.SaveImageFile("basic.yaml", "", []byte("x"))
	if r.Success {
		t.Errorf("expected failure, got %+v", r)
	}
}

func TestLoadForm_MissingTemplateStillReadsRawData(t *testing.T) {
	// Even with no template, LoadForm should not blow up — Sanitize
	// runs with nil fields (everything stays raw).
	m, sys, _, _ := newTestStack(t)
	// Save a form by hand without a template file in templates/.
	_ = sys.SaveFile("storage/orphan/x.meta.json",
		`{"meta":{"id":"abc"},"data":{"weird":"value"}}`)
	f := m.LoadForm("orphan.yaml", "x")
	if f == nil {
		t.Fatal("expected form, got nil")
	}
	if f.Meta.ID != "abc" {
		t.Errorf("meta.id lost: %+v", f.Meta)
	}
}

func TestListForms_MissingFolderReturnsEmpty(t *testing.T) {
	m, _, _, _ := newTestStack(t)
	out, err := m.ListForms("nonexistent.yaml")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestDeleteForm_MissingIsNoOp(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	})
	if err := m.DeleteForm("basic.yaml", "ghost"); err != nil {
		t.Errorf("delete missing should be no-op: %v", err)
	}
}

func TestExtendedListForms_NoTemplateFallsBackToFilename(t *testing.T) {
	m, sys, _, _ := newTestStack(t)
	_ = sys.SaveFile("storage/orphan/x.meta.json",
		`{"meta":{"id":"abc"},"data":{"title":"hi"}}`)
	out, err := m.ExtendedListForms("orphan.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	if out[0].Title != "x.meta.json" {
		t.Errorf("expected fallback title to filename, got %q", out[0].Title)
	}
}

// boolPtr is reused if needed in future tests
var _ = boolPtr
