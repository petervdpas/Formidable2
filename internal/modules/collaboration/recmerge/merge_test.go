package recmerge

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

func recordFrom(t *testing.T, jsonStr string) Record {
	t.Helper()
	r, err := ParseRecord([]byte(jsonStr))
	if err != nil {
		t.Fatalf("ParseRecord(%q): %v", jsonStr, err)
	}
	return r
}

func extractData(t *testing.T, merged []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(merged, &envelope); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	return envelope.Data
}

func TestMerge_DisjointDataFieldsAutoMerge(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Oak","country":"nl"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"name":"Oak Rd","country":"nl"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"name":"Oak","country":"uk"}}`)

	res, err := Merge("storage/x/r.meta.json", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", res.Conflict)
	}
	data := extractData(t, res.Merged)
	if data["name"] != "Oak Rd" {
		t.Errorf("name = %v, want Oak Rd (theirs)", data["name"])
	}
	if data["country"] != "uk" {
		t.Errorf("country = %v, want uk (yours)", data["country"])
	}
}

func TestMerge_SameFieldDifferentValuesLWW_YoursNewer(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"name":"Theirs"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"name":"Yours"}}`)

	res, err := Merge("p", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatal("unexpected conflict")
	}
	if extractData(t, res.Merged)["name"] != "Yours" {
		t.Errorf("expected yours to win, got %v", extractData(t, res.Merged)["name"])
	}
}

func TestMerge_SameFieldDifferentValuesLWW_TheirsNewer(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-06-01T00:00:00Z"},"data":{"name":"Theirs"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"name":"Yours"}}`)

	res, _ := Merge("p", base, theirs, yours)
	if extractData(t, res.Merged)["name"] != "Theirs" {
		t.Errorf("expected theirs to win, got %v", extractData(t, res.Merged)["name"])
	}
}

func TestMerge_SameValueBothSidesNoOp(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"name":"Same"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"name":"Same"}}`)

	res, _ := Merge("p", base, theirs, yours)
	if extractData(t, res.Merged)["name"] != "Same" {
		t.Errorf("same value should resolve to that value")
	}
}

func TestMerge_ImmutableMetaViolationReturnsConflict(t *testing.T) {
	base := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-02-01T00:00:00Z"},"data":{"x":1}}`)
	yours := recordFrom(t, `{"meta":{"created":"2030-01-01T00:00:00Z","updated":"2025-03-01T00:00:00Z"},"data":{"y":2}}`)

	res, err := Merge("storage/x/r.meta.json", base, theirs, yours)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict == nil {
		t.Fatal("expected conflict, got merged")
	}
	if res.Conflict.Path != "storage/x/r.meta.json" {
		t.Errorf("conflict path = %v", res.Conflict.Path)
	}
	if len(res.Conflict.FieldConflicts) != 1 {
		t.Fatalf("conflicts = %+v", res.Conflict.FieldConflicts)
	}
	if res.Conflict.FieldConflicts[0].Key != "created" {
		t.Errorf("expected created conflict, got %+v", res.Conflict.FieldConflicts[0])
	}
	if res.Merged != nil {
		t.Error("Merged should be nil on conflict")
	}
}

func TestMerge_NestedStructureAtomic(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"addr":{"city":"NYC","zip":"10001"}}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"addr":{"city":"NYC","zip":"10002"}}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"addr":{"city":"LA","zip":"10001"}}}`)

	res, _ := Merge("p", base, theirs, yours)
	addr := extractData(t, res.Merged)["addr"].(map[string]any)
	// yours is newer → yours wins the whole sub-object, not a deep merge.
	if addr["city"] != "LA" || addr["zip"] != "10001" {
		t.Errorf("nested object should resolve atomically via LWW, got %+v", addr)
	}
}

func TestMerge_OneSideRemovesField(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"name":"Old"}}`)

	// theirs removed, yours unchanged → removal wins.
	res, _ := Merge("p", base, theirs, yours)
	data := extractData(t, res.Merged)
	if _, present := data["name"]; present {
		t.Errorf("theirs removed and yours unchanged - expected key gone, got %v", data["name"])
	}
}

func TestMerge_CanonicalOutputIsDeterministic(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"a":1}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"a":1,"b":2}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"a":1,"c":3}}`)

	var prev []byte
	for range 10 {
		res, _ := Merge("p", base, theirs, yours)
		if prev != nil && string(prev) != string(res.Merged) {
			t.Fatalf("non-deterministic output: %s vs %s", prev, res.Merged)
		}
		prev = res.Merged
	}
	if !strings.Contains(string(prev), `"a":1`) || !strings.Contains(string(prev), `"b":2`) || !strings.Contains(string(prev), `"c":3`) {
		t.Errorf("expected a,b,c in merged output: %s", prev)
	}
}

// IsRecordPath tests - Formidable2-side addition (gigot has its own
// isFormidableRecordPath in server/). Locks the contract so the
// PullWithStash caller can rely on it for path gating.

func TestIsRecordPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"storage/notes/foo.meta.json", true},
		{"storage/x/r.meta.json", true},
		{"storage/notes/foo.json", false},          // wrong suffix
		{"storage/notes/foo.meta.yaml", false},     // wrong suffix
		{"templates/x.yaml", false},                // not under storage/
		{"storage/foo.meta.json", false},           // missing template dir
		{"storage/notes/sub/foo.meta.json", false}, // too deep
		{"storage/images/foo.meta.json", false},    // images dir reserved
		{"../storage/x/foo.meta.json", false},      // traversal
		{"storage/../etc/foo.meta.json", false},    // traversal
		{"", false},
	}
	for _, tc := range tests {
		got := IsRecordPath(tc.path)
		if got != tc.want {
			t.Errorf("IsRecordPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

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

// Empty input on all three sides is a no-op that yields canonical empty maps.
func TestMerge_AllSidesEmptyYieldsEmptyEnvelope(t *testing.T) {
	empty := recordFrom(t, ``)
	res, err := Merge("p", empty, empty, empty)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", res.Conflict)
	}
	if string(res.Merged) != `{"data":{},"meta":{}}` {
		t.Errorf("merged = %s, want empty envelope", res.Merged)
	}
}

// Zero-value Records (nil Meta/Data maps) merge without panic to the empty envelope.
func TestMerge_NilMapRecordsMergeToEmpty(t *testing.T) {
	var z Record
	res, err := Merge("p", z, z, z)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict != nil {
		t.Fatalf("unexpected conflict: %+v", res.Conflict)
	}
	if string(res.Merged) != `{"data":{},"meta":{}}` {
		t.Errorf("merged = %s, want empty envelope", res.Merged)
	}
}

// Several malformed JSON shapes must all wrap ErrMalformedRecord and return a zero Record.
func TestParseRecord_MalformedVariants(t *testing.T) {
	cases := []string{
		`{"meta":{"x":}`,     // unterminated value
		`not json at all`,    // garbage
		`{"meta":[]}`,        // meta is not an object
		`{`,                  // truncated
		`[1,2,3]`,            // array root, not an envelope object
		`{"data":{}}{"x":1}`, // trailing object after a complete value
	}
	for _, in := range cases {
		rec, err := ParseRecord([]byte(in))
		if !errors.Is(err, ErrMalformedRecord) {
			t.Errorf("ParseRecord(%q) err = %v, want wrapped ErrMalformedRecord", in, err)
		}
		if rec.Meta != nil || rec.Data != nil {
			t.Errorf("ParseRecord(%q) should return zero Record, got meta=%v data=%v", in, rec.Meta, rec.Data)
		}
	}
}

// updated as the audit-block object form ({at,...}) drives LWW via the .at field; yours is newer.
func TestMerge_AuditBlockUpdatedDrivesLWW(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":{"at":"2025-01-01T00:00:00Z","name":"x"}},"data":{"v":"b"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":{"at":"2025-02-01T00:00:00Z"}},"data":{"v":"T"}}`)
	yours := recordFrom(t, `{"meta":{"updated":{"at":"2025-03-01T00:00:00Z"}},"data":{"v":"Y"}}`)

	if w := UpdatedWinner(theirs.Meta, yours.Meta); w != "yours" {
		t.Errorf("UpdatedWinner over audit blocks = %q, want yours", w)
	}
	res, _ := Merge("p", base, theirs, yours)
	want := `{"data":{"v":"Y"},"meta":{"updated":{"at":"2025-03-01T00:00:00Z"}}}`
	if string(res.Merged) != want {
		t.Errorf("merged = %s, want %s", res.Merged, want)
	}
}

// Both sides change an immutable key away from base to the SAME new value: still an immutable conflict.
func TestMerge_ImmutableSameNewValueStillConflicts(t *testing.T) {
	base := recordFrom(t, `{"meta":{"id":"a","updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"id":"b","updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"id":"b","updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	if res.Conflict == nil {
		t.Fatal("expected immutable conflict on id divergence")
	}
	if len(res.Conflict.FieldConflicts) != 1 {
		t.Fatalf("conflicts = %+v, want exactly 1", res.Conflict.FieldConflicts)
	}
	fc := res.Conflict.FieldConflicts[0]
	if fc.Key != "id" || fc.Scope != "meta" || fc.Reason != "immutable" {
		t.Errorf("conflict = %+v, want id/meta/immutable", fc)
	}
	if res.Merged != nil {
		t.Error("Merged must be nil on conflict")
	}
}

// One side changes an immutable key, the other leaves it equal to base: that is a divergence and conflicts.
func TestMerge_ImmutableOneSideChangesConflicts(t *testing.T) {
	base := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"created":"2025-01-01T00:00:00Z","updated":"2025-02-01T00:00:00Z"},"data":{}}`)
	yours := recordFrom(t, `{"meta":{"created":"2099-01-01T00:00:00Z","updated":"2025-03-01T00:00:00Z"},"data":{}}`)

	res, _ := Merge("p", base, theirs, yours)
	if res.Conflict == nil {
		t.Fatal("expected conflict: yours diverged created from base")
	}
	if len(res.Conflict.FieldConflicts) != 1 || res.Conflict.FieldConflicts[0].Key != "created" {
		t.Fatalf("conflicts = %+v, want single created conflict", res.Conflict.FieldConflicts)
	}
}

// Concurrency: identical input merged in parallel must produce byte-identical output and never race.
func TestMerge_ParallelDeterministic(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{"a":1,"name":"Old"}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"a":1,"name":"Theirs","b":2}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"a":1,"name":"Yours","c":3}}`)

	want := `{"data":{"a":1,"b":2,"c":3,"name":"Yours"},"meta":{"updated":"2025-03-01T00:00:00Z"}}`
	// Single-shot baseline pins the exact expected merge.
	res0, _ := Merge("p", base, theirs, yours)
	if string(res0.Merged) != want {
		t.Fatalf("baseline merged = %s, want %s", res0.Merged, want)
	}

	const n = 32
	var wg sync.WaitGroup
	out := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := Merge("p", base, theirs, yours)
			if err != nil {
				t.Errorf("goroutine %d err: %v", idx, err)
				return
			}
			out[idx] = string(r.Merged)
		}(i)
	}
	wg.Wait()
	for i, got := range out {
		if got != want {
			t.Errorf("parallel merge %d = %s, want %s", i, got, want)
		}
	}
}

// Both sides add the same new key absent from base: that value lands, no conflict, no LWW.
func TestMerge_BothAddSameNewKey(t *testing.T) {
	base := recordFrom(t, `{"meta":{"updated":"2025-01-01T00:00:00Z"},"data":{}}`)
	theirs := recordFrom(t, `{"meta":{"updated":"2025-02-01T00:00:00Z"},"data":{"new":"v"}}`)
	yours := recordFrom(t, `{"meta":{"updated":"2025-03-01T00:00:00Z"},"data":{"new":"v"}}`)

	res, _ := Merge("p", base, theirs, yours)
	want := `{"data":{"new":"v"},"meta":{"updated":"2025-03-01T00:00:00Z"}}`
	if string(res.Merged) != want {
		t.Errorf("merged = %s, want %s", res.Merged, want)
	}
}

// IsRecordPath boundary cases the table in merge_test.go does not cover.
func TestIsRecordPath_Boundaries(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"storage/notes/.meta.json", true},      // empty stem before suffix, still 3 valid non-empty parts
		{"storage//foo.meta.json", false},       // empty template dir
		{"storage/notes/", false},               // trailing slash, no file
		{"storage/notes/foo.meta.json/", false}, // trailing slash, 4 parts
		{".meta.json", false},                   // no storage prefix
		{"storage/notes/foo..meta.json", false}, // contains double dot
	}
	for _, tc := range cases {
		if got := IsRecordPath(tc.path); got != tc.want {
			t.Errorf("IsRecordPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
