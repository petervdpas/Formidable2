package template

import "testing"

func TestParseProjectDoc_RoundTrip(t *testing.T) {
	in := map[string]any{"name": "HR2DAY connector"}
	doc, err := ParseProjectDoc(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Name != "HR2DAY connector" {
		t.Errorf("round-trip mismatch: %+v", doc)
	}
}

func TestTimeBlocks_VocabularyAndDefensiveCopy(t *testing.T) {
	a := TimeBlocks()
	want := []string{"day", "week", "2-week", "3-week", "month"}
	if len(a) != len(want) {
		t.Fatalf("want %d time blocks, got %d (%v)", len(want), len(a), a)
	}
	for i, w := range want {
		if a[i] != w {
			t.Errorf("time block %d = %q, want %q", i, a[i], w)
		}
	}
	a[0] = "mutated"
	if b := TimeBlocks(); b[0] != TimeBlockDay {
		t.Errorf("TimeBlocks not a defensive copy: %q", b[0])
	}
}

func TestIsTimeBlock(t *testing.T) {
	for _, b := range []string{"day", "week", "2-week", "3-week", "month"} {
		if !IsTimeBlock(b) {
			t.Errorf("%q should be a valid time block", b)
		}
	}
	for _, b := range []string{"", "weekly", "4-week", "WEEK"} {
		if IsTimeBlock(b) {
			t.Errorf("%q should not be a valid time block", b)
		}
	}
}

func TestProjectAxisReaders(t *testing.T) {
	f := Field{Type: "project", Options: []any{
		map[string]any{"value": "from", "label": "2026-06-27"},
		map[string]any{"value": "to", "label": "2026-08-16"},
		map[string]any{"value": "timeblock", "label": "2-week"},
	}}
	if from, to := ProjectDateRange(f); from != "2026-06-27" || to != "2026-08-16" {
		t.Errorf("ProjectDateRange = %q/%q", from, to)
	}
	if tb := ProjectTimeBlock(f); tb != "2-week" {
		t.Errorf("ProjectTimeBlock = %q, want 2-week", tb)
	}
	// Unset/garbage granularity falls back to weekly; empty dates read as "".
	empty := Field{Type: "project"}
	if from, to := ProjectDateRange(empty); from != "" || to != "" {
		t.Errorf("empty ProjectDateRange = %q/%q, want empty", from, to)
	}
	if tb := ProjectTimeBlock(empty); tb != TimeBlockWeek {
		t.Errorf("default ProjectTimeBlock = %q, want week", tb)
	}
}

func TestParseProjectDoc_Nil(t *testing.T) {
	doc, err := ParseProjectDoc(nil)
	if err != nil {
		t.Fatalf("parse nil: %v", err)
	}
	if doc != (ProjectDoc{}) {
		t.Errorf("nil should decode to empty doc, got %+v", doc)
	}
}

func TestParseProjectDoc_WrongInnerType(t *testing.T) {
	// A numeric name can't unmarshal into a string field: caller treats as drift.
	if _, err := ParseProjectDoc(map[string]any{"name": 7}); err == nil {
		t.Error("expected error decoding numeric name, got nil")
	}
}

// project is the plan-board singleton: forced read-only key, one per template,
// reserved key, requires collection (a project-bearing template is a collection
// of referenceable projects). Unlike slideset it needs no companion field in the
// same template: events reference it cross-template.
func TestProjectFieldDescriptor_IsSingletonRequiringCollection(t *testing.T) {
	got, ok := fieldDescriptors["project"]
	if !ok {
		t.Fatalf("project descriptor missing")
	}
	a := got.Abilities
	if !a.Key || !a.Type {
		t.Errorf("project must keep Key + Type")
	}
	if !a.Options {
		t.Errorf("project must advertise options (the board's from/to/timeblock axis)")
	}
	if !a.ExpressionItem {
		t.Errorf("project must advertise Expression field (root-level item-field candidate)")
	}
	if a.Label || a.Description || a.Default ||
		a.PrimaryKey || a.UseInStatistics {
		t.Errorf("project modal stays lean apart from axis options + expression; got %+v", a)
	}
	if got.OptionsShape == nil || len(got.OptionsShape.Rows) != 3 {
		t.Errorf("project axis is a fixed 3-row shape (from/to/timeblock); got %+v", got.OptionsShape)
	}
	if !got.KeyReadonly {
		t.Errorf("project key must be read-only (forced singleton)")
	}
	if got.RequiresCollection {
		t.Errorf("a board is a single record; project must not require collection mode")
	}
}

func TestNormalize_ForcesProjectKey(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "whatever", Type: "project"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Key; got != "project" {
		t.Errorf("project key = %q, want forced to \"project\"", got)
	}
}

func TestValidate_MultipleProjectFields_Flagged(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{
		{Key: "project", Type: "project"},
		{Key: "project2", Type: "project"},
	}})
	if !hasErr(errs, "multiple-project-fields") {
		t.Errorf("expected multiple-project-fields; got %+v", errs)
	}
}

func TestValidate_ProjectReservedKey(t *testing.T) {
	if errs := Validate(&Template{Fields: []Field{{Key: "project", Type: "text"}}}); !hasErr(errs, "reserved-key") {
		t.Errorf("text field keyed \"project\" should be reserved-key; got %+v", errs)
	}
	if errs := Validate(&Template{Fields: []Field{{Key: "project", Type: "project"}}}); hasErr(errs, "reserved-key") {
		t.Errorf("the project field may use key \"project\"; got %+v", errs)
	}
}

func TestValidate_PresentationAndProjectModeConflict(t *testing.T) {
	// Both modes on is flagged (incompatible record models).
	if errs := Validate(&Template{Presentation: true, ProjectMode: true, Fields: []Field{
		{Key: "project", Type: "project"},
	}}); !hasErr(errs, "presentation-project-mode-conflict") {
		t.Errorf("both modes on should be flagged; got %+v", errs)
	}
	// Either alone is fine (no conflict error).
	if errs := Validate(&Template{ProjectMode: true, Fields: []Field{
		{Key: "project", Type: "project"},
	}}); hasErr(errs, "presentation-project-mode-conflict") {
		t.Errorf("project mode alone should not conflict; got %+v", errs)
	}
	if errs := Validate(&Template{Presentation: true, Fields: []Field{
		{Key: "seq", Type: "sequence"},
	}}); hasErr(errs, "presentation-project-mode-conflict") {
		t.Errorf("presentation alone should not conflict; got %+v", errs)
	}
}

func TestValidate_ProjectNeedsNoCollection(t *testing.T) {
	// A lone project board is a single record; no collection required.
	if errs := Validate(&Template{Fields: []Field{{Key: "project", Type: "project"}}}); hasErr(errs, "project-needs-collection") {
		t.Errorf("project must not require collection; got %+v", errs)
	}
}
