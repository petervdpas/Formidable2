package template

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBuildFieldTree_FlatNoLoops(t *testing.T) {
	fields := []Field{
		{Key: "a", Type: "text"},
		{Key: "b", Type: "boolean"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 2 {
		t.Fatalf("expected 2 units, got %d", len(units))
	}
	for i, u := range units {
		if u.Kind != "field" {
			t.Fatalf("unit[%d] kind=%q want field", i, u.Kind)
		}
		if u.Field == nil || u.Field.Key != fields[i].Key {
			t.Fatalf("unit[%d] field mismatch", i)
		}
	}
}

func TestBuildFieldTree_SimpleLoop(t *testing.T) {
	fields := []Field{
		{Key: "before", Type: "boolean"},
		{Key: "rel", Type: "loopstart"},
		{Key: "link", Type: "link"},
		{Key: "rel", Type: "loopstop"},
		{Key: "after", Type: "boolean"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 3 {
		t.Fatalf("expected 3 top-level units, got %d", len(units))
	}
	if units[0].Kind != "field" || units[0].Field.Key != "before" {
		t.Fatalf("units[0] mismatch: %+v", units[0])
	}
	if units[1].Kind != "loop" || units[1].Start.Key != "rel" || units[1].Stop.Key != "rel" {
		t.Fatalf("units[1] not a paired loop: %+v", units[1])
	}
	if len(units[1].Items) != 1 || units[1].Items[0].Field.Key != "link" {
		t.Fatalf("loop interior mismatch: %+v", units[1].Items)
	}
	if units[2].Kind != "field" || units[2].Field.Key != "after" {
		t.Fatalf("units[2] mismatch: %+v", units[2])
	}
}

func TestBuildFieldTree_NestedLoops(t *testing.T) {
	fields := []Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "x", Type: "text"},
		{Key: "inner", Type: "loopstart"},
		{Key: "y", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "z", Type: "text"},
		{Key: "outer", Type: "loopstop"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 1 || units[0].Kind != "loop" || units[0].Start.Key != "outer" {
		t.Fatalf("outer loop missing: %+v", units)
	}
	inner := units[0].Items
	if len(inner) != 3 {
		t.Fatalf("outer should hold 3 items (x, inner-loop, z), got %d", len(inner))
	}
	if inner[1].Kind != "loop" || inner[1].Start.Key != "inner" || len(inner[1].Items) != 1 {
		t.Fatalf("inner loop malformed: %+v", inner[1])
	}
	if inner[1].Items[0].Field.Key != "y" {
		t.Fatalf("inner.items[0] should be y, got %+v", inner[1].Items[0])
	}
}

func TestBuildFieldTree_OrphanLoopstop(t *testing.T) {
	// A loopstop with no preceding loopstart must NOT be silently
	// dropped - emit it as a plain row so the user can see and delete
	// it, and let backend validation flag it as unmatched.
	fields := []Field{
		{Key: "a", Type: "text"},
		{Key: "orphan", Type: "loopstop"},
		{Key: "b", Type: "text"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
	if units[1].Kind != "field" || units[1].Field.Type != "loopstop" {
		t.Fatalf("orphan stop not preserved: %+v", units[1])
	}
}

func TestBuildFieldTree_OrphanLoopstart(t *testing.T) {
	// A loopstart with no matching loopstop falls through as a plain
	// row and its supposed-interior content stays at the same level.
	fields := []Field{
		{Key: "a", Type: "text"},
		{Key: "orphan", Type: "loopstart"},
		{Key: "interior", Type: "text"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d (%+v)", len(units), units)
	}
	if units[1].Kind != "field" || units[1].Field.Type != "loopstart" {
		t.Fatalf("orphan start not preserved: %+v", units[1])
	}
	if units[2].Field.Key != "interior" {
		t.Fatalf("interior should be sibling, got %+v", units[2])
	}
}

func TestFlattenFieldTree_RoundTripIdentity(t *testing.T) {
	// Build → flatten is identity for well-formed inputs.
	cases := [][]Field{
		{{Key: "a", Type: "text"}, {Key: "b", Type: "boolean"}},
		{
			{Key: "a", Type: "text"},
			{Key: "loop", Type: "loopstart"},
			{Key: "x", Type: "text"},
			{Key: "loop", Type: "loopstop"},
			{Key: "b", Type: "text"},
		},
		{
			{Key: "o", Type: "loopstart"},
			{Key: "x", Type: "text"},
			{Key: "i", Type: "loopstart"},
			{Key: "y", Type: "text"},
			{Key: "i", Type: "loopstop"},
			{Key: "z", Type: "text"},
			{Key: "o", Type: "loopstop"},
		},
	}
	for ci, fields := range cases {
		round := FlattenFieldTree(BuildFieldTree(fields))
		if !reflect.DeepEqual(round, fields) {
			t.Fatalf("case %d: round-trip mismatch\n got: %+v\nwant: %+v", ci, round, fields)
		}
	}
}

func TestFlattenFieldTree_LoopMarkersAlwaysBracket(t *testing.T) {
	// After ANY reorder of units, flatten must always emit
	// loopstart immediately before its loop's items and loopstop
	// immediately after. This is the invariant that fixes the
	// drag-into-loop corruption: a plain field unit can never end up
	// between start and stop unless it was reordered as part of the
	// loop's `Items` slice.
	fields := []Field{
		{Key: "before", Type: "boolean"},
		{Key: "rel", Type: "loopstart"},
		{Key: "link", Type: "link"},
		{Key: "rel", Type: "loopstop"},
		{Key: "isDekkend", Type: "boolean"},
	}
	units := BuildFieldTree(fields)
	// Reorder: move the "isDekkend" unit up to position 1 (right after
	// "before") - the exact malformation from the bug screenshot.
	dragged := units[2]
	units = []FieldUnit{units[0], dragged, units[1]}
	flat := FlattenFieldTree(units)

	want := []Field{
		{Key: "before", Type: "boolean"},
		{Key: "isDekkend", Type: "boolean"},
		{Key: "rel", Type: "loopstart"},
		{Key: "link", Type: "link"},
		{Key: "rel", Type: "loopstop"},
	}
	if !reflect.DeepEqual(flat, want) {
		t.Fatalf("reordered flatten broke bracket invariant\n got: %+v\nwant: %+v", flat, want)
	}
}

func TestBuildFieldTree_NilAndEmpty(t *testing.T) {
	if units := BuildFieldTree(nil); units == nil {
		t.Fatalf("BuildFieldTree(nil) should return non-nil slice for JSON friendliness")
	}
	if units := BuildFieldTree([]Field{}); len(units) != 0 {
		t.Fatalf("BuildFieldTree([]) should return empty slice, got %+v", units)
	}
}

func TestFlattenFieldTree_NilAndEmpty(t *testing.T) {
	if fields := FlattenFieldTree(nil); fields == nil {
		t.Fatalf("FlattenFieldTree(nil) should return non-nil slice")
	}
	if fields := FlattenFieldTree([]FieldUnit{}); len(fields) != 0 {
		t.Fatalf("FlattenFieldTree([]) should return empty, got %+v", fields)
	}
}

// Regression: empty-loop items must serialise as `"items": []`, not be
// dropped by omitempty. The frontend's vuedraggable :list binding needs
// a real array reference to mutate; with an omitted key, dragging into
// the loop falls into a temporary fallback array and the field vanishes.
func TestFieldUnit_EmptyLoopItemsRoundTripsAsEmptyArray(t *testing.T) {
	fields := []Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "outer", Type: "loopstop"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 1 || units[0].Kind != "loop" {
		t.Fatalf("expected one loop unit, got %+v", units)
	}
	if units[0].Items == nil {
		t.Fatalf("Items must be non-nil empty slice, got nil")
	}
	raw, err := json.Marshal(units[0])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Errorf("JSON must contain `\"items\":[]`, got: %s", raw)
	}
}

func TestBuildFieldTree_CaseInsensitiveType(t *testing.T) {
	// YAML round-trip + hand edits can produce mixed casing. Pairing
	// must still recognise the markers.
	fields := []Field{
		{Key: "rel", Type: "LOOPSTART"},
		{Key: "x", Type: "text"},
		{Key: "rel", Type: "LoopStop"},
	}
	units := BuildFieldTree(fields)
	if len(units) != 1 || units[0].Kind != "loop" {
		t.Fatalf("case-insensitive pairing failed: %+v", units)
	}
}
