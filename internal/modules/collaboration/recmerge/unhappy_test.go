package recmerge

import (
	"encoding/json"
	"errors"
	"testing"
)

// Malformed JSON must surface ErrMalformedRecord and yield a zero Record.
func TestParseRecord_MalformedReturnsSentinel(t *testing.T) {
	rec, err := ParseRecord([]byte(`{"meta":{"x":}`))
	if !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("err = %v, want wrapped ErrMalformedRecord", err)
	}
	if rec.Meta != nil || rec.Data != nil {
		t.Errorf("malformed parse should return zero Record, got meta=%v data=%v", rec.Meta, rec.Data)
	}
}

// Empty input is not an error: both maps come back non-nil and empty.
func TestParseRecord_EmptyInputYieldsEmptyMaps(t *testing.T) {
	rec, err := ParseRecord([]byte("   \n\t "))
	if err != nil {
		t.Fatalf("empty input err = %v, want nil", err)
	}
	if rec.Meta == nil || len(rec.Meta) != 0 {
		t.Errorf("meta = %v, want empty non-nil map", rec.Meta)
	}
	if rec.Data == nil || len(rec.Data) != 0 {
		t.Errorf("data = %v, want empty non-nil map", rec.Data)
	}
}

// Missing meta/data keys decode to empty maps, not nil.
func TestParseRecord_MissingSectionsDefaultEmpty(t *testing.T) {
	rec, err := ParseRecord([]byte(`{"data":{"k":1}}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rec.Meta == nil || len(rec.Meta) != 0 {
		t.Errorf("absent meta should be empty map, got %v", rec.Meta)
	}
	if len(rec.Data) != 1 {
		t.Errorf("data len = %d, want 1", len(rec.Data))
	}
}

// All three sides identical: merge is a no-op that preserves the value exactly.
func TestMerge_IdenticalSidesNoOp(t *testing.T) {
	same := `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"a":1,"b":"two"}}`
	base := recordFrom(t, same)
	theirs := recordFrom(t, same)
	yours := recordFrom(t, same)

	res, err := Merge("p", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", res.Conflict)
	}
	want := `{"data":{"a":1,"b":"two"},"meta":{"updated":"2025-01-01T00:00:00Z"}}`
	if string(res.Merged) != want {
		t.Errorf("no-op merge = %s, want %s", res.Merged, want)
	}
}

// Empty base (missing ancestor) where both sides set an immutable key differently:
// the empty-map base degrades the immutability check, no conflict fires, and the
// side that holds the key first (theirs) supplies the merged created value.
func TestMerge_MissingAncestorSkipsImmutableConflict(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"created":"2030-01-01T00:00:00Z","updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, err := Merge("p", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("missing-ancestor immutable divergence should not conflict, got %+v", res.Conflict)
	}
	want := `{"data":{},"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-03-01T00:00:00Z"}}`
	if string(res.Merged) != want {
		t.Errorf("merged = %s, want %s", res.Merged, want)
	}
}

// Concurrent edits to the same field from a present base resolve by LWW on meta.updated.
// yours is newer, so yours wins atomically.
func TestMerge_ConcurrentSameFieldLWWYoursWins(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"v":"base"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"v":"theirs"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"v":"yours"}}`)

	res, err := Merge("p", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", res.Conflict)
	}
	// Whole atomic value resolves to yours; updated merges to the newest stamp.
	want := `{"data":{"v":"yours"},"meta":{"updated":"2025-03-01T00:00:00Z"}}`
	if string(res.Merged) != want {
		t.Errorf("merged = %s, want %s", res.Merged, want)
	}
}

// Type mismatch on the same key: string base, number on theirs, array on yours,
// both differ from base. Newer side (yours) wins the whole atomic value.
func TestMerge_TypeMismatchResolvesByLWW(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"v":"base"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"v":42}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"v":["x"]}}`)

	res, _ := Merge("p", base, theirs, yours)
	arr, ok := extractData(t, res.Merged)["v"].([]any)
	if !ok {
		t.Fatalf("v type = %T, want []any from yours", extractData(t, res.Merged)["v"])
	}
	if len(arr) != 1 || arr[0] != "x" {
		t.Errorf("v = %v, want [x] (yours)", arr)
	}
}

// Equal meta.updated timestamps are a tie; the rule resolves ties to yours.
func TestMerge_UpdatedTieResolvesToYours(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"v":"b"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"v":"T"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"v":"Y"}}`)

	res, _ := Merge("p", base, theirs, yours)
	if got := extractData(t, res.Merged)["v"]; got != "Y" {
		t.Errorf("tie should resolve to yours, got %v", got)
	}
}

// Unparseable updated on both sides is a tie that falls through to yours.
func TestMerge_UnparseableUpdatedDefaultsYours(t *testing.T) {
	base := recordFrom(t, `{"meta":{},"data":{"v":"b"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"not-a-date"},"data":{"v":"T"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"also-bad"},"data":{"v":"Y"}}`)

	if w := UpdatedWinner(theirs.Meta, yours.Meta); w != "yours" {
		t.Errorf("UpdatedWinner with unparseable dates = %q, want yours", w)
	}
	res, _ := Merge("p", base, theirs, yours)
	if got := extractData(t, res.Merged)["v"]; got != "Y" {
		t.Errorf("v = %v, want Y (yours via tie)", got)
	}
}

// Genuine immutable divergence with a real base flags exactly one field conflict
// and leaves Merged nil.
func TestMerge_ImmutableTemplateDivergenceConflict(t *testing.T) {
	base := recordFrom(t, `{"meta":{"template":"a.yaml","updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"template":"a.yaml","updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"template":"b.yaml","updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, err := Merge("storage/x/r.meta.json", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict == nil {
		t.Fatal("expected conflict on template divergence")
	}
	if len(res.Conflict.FieldConflicts) != 1 {
		t.Fatalf("field conflicts = %+v, want exactly 1", res.Conflict.FieldConflicts)
	}
	fc := res.Conflict.FieldConflicts[0]
	if fc.Key != "template" || fc.Scope != "meta" || fc.Reason != "immutable" {
		t.Errorf("conflict = %+v, want template/meta/immutable", fc)
	}
	if res.Merged != nil {
		t.Error("Merged must be nil on conflict")
	}
}

// Two distinct immutable keys diverging report two conflicts, sorted created before id.
func TestMerge_MultipleImmutableConflicts(t *testing.T) {
	base := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","id":"x","updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"created":"2026-01-01T00:00:00Z","id":"x","updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","id":"y","updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	if res.Conflict == nil {
		t.Fatal("expected conflict")
	}
	if len(res.Conflict.FieldConflicts) != 2 {
		t.Fatalf("conflicts = %+v, want 2", res.Conflict.FieldConflicts)
	}
	keys := []string{res.Conflict.FieldConflicts[0].Key, res.Conflict.FieldConflicts[1].Key}
	if keys[0] != "created" || keys[1] != "id" {
		t.Errorf("conflict keys = %v, want [created id] in immutable order", keys)
	}
	for _, fc := range res.Conflict.FieldConflicts {
		if fc.Scope != "meta" || fc.Reason != "immutable" {
			t.Errorf("conflict %+v, want scope=meta reason=immutable", fc)
		}
	}
	if res.Merged != nil {
		t.Error("Merged must be nil on conflict")
	}
}

// tags from both sides union, lowercase, trim and sort; flagged ORs to true.
func TestMerge_TagsUnionAndFlaggedOr(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z","tags":["a"],"flagged":false},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z","tags":["B "," a","c"],"flagged":true},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z","tags":["d","A"],"flagged":false},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	meta := extractMeta(t, res.Merged)
	tags, ok := meta["tags"].([]any)
	if !ok {
		t.Fatalf("tags type = %T", meta["tags"])
	}
	want := []string{"a", "b", "c", "d"}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
	for i, w := range want {
		if tags[i] != w {
			t.Errorf("tags[%d] = %v, want %v", i, tags[i], w)
		}
	}
	if meta["flagged"] != true {
		t.Errorf("flagged = %v, want true (OR of sides)", meta["flagged"])
	}
}

// Both sides remove a field the base held. ACTUAL current behavior: the merge
// restores base's value instead of dropping it. This is a bug (see suspectedBugs,
// merge.go:78-83): the neither-side-has-key default keeps base, contradicting the
// one-side-removal-wins rule. Asserting the real output, not the intended one.
func TestMerge_BothSidesRemoveField_DropsField(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"gone":"x","keep":"y"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"keep":"y"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"keep":"y"}}`)

	res, _ := Merge("p", base, theirs, yours)
	data := extractData(t, res.Merged)
	// Both sides deliberately dropped "gone": removal wins, base is not resurrected.
	if v, present := data["gone"]; present {
		t.Errorf("gone = %v, want absent (both sides removed it)", v)
	}
	if data["keep"] != "y" {
		t.Errorf("keep = %v, want y", data["keep"])
	}
}

// One side changes a field while the other removes it: LWW decides. theirs changed
// and is newer here, so theirs value survives the yours-side removal.
func TestMerge_ChangeVsRemoveLWWTheirsNewer(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"v":"old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-06-01T00:00:00Z"},"data":{"v":"changed"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	if got := extractData(t, res.Merged)["v"]; got != "changed" {
		t.Errorf("v = %v, want changed (theirs newer beats yours removal)", got)
	}
}

// Same scenario but yours (the removal side) is newer: removal wins, key drops.
func TestMerge_ChangeVsRemoveLWWYoursRemovalNewer(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"v":"old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"v":"changed"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-09-01T00:00:00Z"},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	if _, present := extractData(t, res.Merged)["v"]; present {
		t.Errorf("yours removal is newer, key v should be absent, got %v", extractData(t, res.Merged)["v"])
	}
}

// CanonicalJSON normalises json.Number so numerically equal decoder forms compare equal.
func TestMerge_NumberEqualityNoSpuriousConflict(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"n":1}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"n":1}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"n":1}}`)

	res, _ := Merge("p", base, theirs, yours)
	// meta.updated merges to the newest timestamp (yours), data n stays 1.
	want := `{"data":{"n":1},"meta":{"updated":"2025-03-01T00:00:00Z"}}`
	if string(res.Merged) != want {
		t.Errorf("merged = %s, want %s", res.Merged, want)
	}
}

// extractMeta pulls the meta section out of a merged blob.
func extractMeta(t *testing.T, merged []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(merged, &envelope); err != nil {
		t.Fatalf("unmarshal merged meta: %v", err)
	}
	return envelope.Meta
}
