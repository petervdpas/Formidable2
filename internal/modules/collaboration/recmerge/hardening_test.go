package recmerge

import (
	"errors"
	"sync"
	"testing"
)

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
		`{"meta":{"x":}`,  // unterminated value
		`not json at all`, // garbage
		`{"meta":[]}`,     // meta is not an object
		`{`,               // truncated
		`[1,2,3]`,         // array root, not an envelope object
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
