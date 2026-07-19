package storage

import (
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A loop iteration that holds no data anywhere (empty string / empty map / empty
// array / nil, recursively) is noise: the frontend can create one by adding a
// row and abandoning it. Sanitize runs on every save, so it is the single choke
// point where the "loopers never persist empty entries" invariant is enforced,
// for EVERY loop, not just the events board.
func loopFields() []template.Field {
	return []template.Field{
		{Key: "events", Type: "loopstart"},
		{Key: "event", Type: "event"},
		{Key: "description", Type: "textarea"},
		{Key: "events", Type: "loopstop"},
	}
}

func TestSanitize_PrunesEmptyLoopIterations(t *testing.T) {
	raw := map[string]any{
		"events": []any{
			map[string]any{"event": map[string]any{"start": "2026-07-16", "kind": "Taak", "resource": "peter"}},
			map[string]any{},                                     // fully empty
			map[string]any{"event": map[string]any{}},            // empty event object
			map[string]any{"event": map[string]any{"start": ""}}, // present but blank
			map[string]any{"event": map[string]any{"start": "2026-07-21", "resource": "jack"}},
		},
	}
	out := Sanitize(raw, loopFields(), SanitizeOptions{})
	got, ok := out.Data["events"].([]any)
	if !ok {
		t.Fatalf("events not a slice: %T", out.Data["events"])
	}
	if len(got) != 2 {
		t.Fatalf("want 2 surviving iterations, got %d: %+v", len(got), got)
	}
	first := got[0].(map[string]any)["event"].(map[string]any)
	if first["resource"] != "peter" {
		t.Errorf("first survivor wrong: %+v", got[0])
	}
	second := got[1].(map[string]any)["event"].(map[string]any)
	if second["resource"] != "jack" {
		t.Errorf("second survivor wrong: %+v", got[1])
	}
}

// An iteration with a value in ANY inner field (including a folded author field
// like description) is real and must survive even when the event bar itself is
// still blank, we never delete typed content.
func TestSanitize_KeepsIterationWithOnlyDescription(t *testing.T) {
	raw := map[string]any{
		"events": []any{
			map[string]any{"event": map[string]any{"description": "just a note"}},
			map[string]any{},
		},
	}
	out := Sanitize(raw, loopFields(), SanitizeOptions{})
	got := out.Data["events"].([]any)
	if len(got) != 1 {
		t.Fatalf("want 1 survivor (the note), got %d: %+v", len(got), got)
	}
}

// Numbers and booleans are data, not emptiness: an iteration carrying only a 0
// or false must not be pruned.
func TestSanitize_ZeroAndFalseAreNotEmpty(t *testing.T) {
	fields := []template.Field{
		{Key: "rows", Type: "loopstart"},
		{Key: "count", Type: "number"},
		{Key: "flag", Type: "boolean"},
		{Key: "rows", Type: "loopstop"},
	}
	raw := map[string]any{
		"rows": []any{
			map[string]any{"count": float64(0)},
			map[string]any{"flag": false},
			map[string]any{"count": "", "flag": nil},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	got := out.Data["rows"].([]any)
	if len(got) != 2 {
		t.Fatalf("want 2 survivors (0 and false are data), got %d: %+v", len(got), got)
	}
}

// Pruning reaches every nesting level: an empty iteration of a loop NESTED inside
// another loop's item is dropped too, and an outer item that becomes empty once
// its only (nested) loop is emptied is itself dropped.
func TestSanitize_PrunesNestedLoopIterations(t *testing.T) {
	fields := []template.Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "name", Type: "text"},
		{Key: "inner", Type: "loopstart"},
		{Key: "task", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "outer", Type: "loopstop"},
	}
	raw := map[string]any{
		"outer": []any{
			// keeps its name; inner has one real + one empty task -> empty dropped
			map[string]any{"name": "A", "inner": []any{
				map[string]any{"task": "real"},
				map[string]any{},
			}},
			// no name; inner is all-empty -> whole outer item collapses to empty -> dropped
			map[string]any{"inner": []any{map[string]any{}, map[string]any{"task": ""}}},
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	outer := out.Data["outer"].([]any)
	if len(outer) != 1 {
		t.Fatalf("want 1 outer survivor, got %d: %+v", len(outer), outer)
	}
	survivor := outer[0].(map[string]any)
	if survivor["name"] != "A" {
		t.Errorf("wrong outer survivor: %+v", survivor)
	}
	inner := survivor["inner"].([]any)
	if len(inner) != 1 || inner[0].(map[string]any)["task"] != "real" {
		t.Errorf("nested loop not pruned to the one real task: %+v", inner)
	}
}

// An all-empty loop collapses to an empty slice, never nil, so the on-disk shape
// stays a JSON array.
func TestSanitize_AllEmptyLoopBecomesEmptySlice(t *testing.T) {
	raw := map[string]any{
		"events": []any{map[string]any{}, map[string]any{"event": map[string]any{}}},
	}
	out := Sanitize(raw, loopFields(), SanitizeOptions{})
	got, ok := out.Data["events"].([]any)
	if !ok {
		t.Fatalf("events not a slice: %T", out.Data["events"])
	}
	if got == nil {
		t.Fatal("events pruned to nil; want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("want 0 survivors, got %d", len(got))
	}
	if !reflect.DeepEqual(got, []any{}) {
		t.Errorf("want []any{}, got %#v", got)
	}
}
