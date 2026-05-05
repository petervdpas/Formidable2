package form

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// ComputeLoopGroups — pairs loopstart/loopstop, computes depth + summary
// + collapsed-default. Mirrors fieldGroupRenderer's pairing pass.
// ─────────────────────────────────────────────────────────────────────

func TestComputeLoopGroups_NoLoops(t *testing.T) {
	fields := []template.Field{
		{Key: "a", Type: "text"},
		{Key: "b", Type: "boolean"},
	}
	got := ComputeLoopGroups(fields, false)
	if len(got) != 0 {
		t.Errorf("no loops: want 0 groups, got %d (%+v)", len(got), got)
	}
}

func TestComputeLoopGroups_SingleTopLevelLoop(t *testing.T) {
	fields := []template.Field{
		{Key: "before", Type: "text"},
		{Key: "items", Type: "loopstart", SummaryField: "name"},
		{Key: "name", Type: "text"},
		{Key: "qty", Type: "number"},
		{Key: "items", Type: "loopstop"},
		{Key: "after", Type: "text"},
	}
	got := ComputeLoopGroups(fields, false)
	if len(got) != 1 {
		t.Fatalf("want 1 group, got %d", len(got))
	}
	g := got[0]
	if g.Key != "items" || g.StartIndex != 1 || g.StopIndex != 4 {
		t.Errorf("unexpected indices: %+v", g)
	}
	if g.Depth != 1 {
		t.Errorf("top-level depth: want 1, got %d", g.Depth)
	}
	if g.SummaryFieldKey != "name" {
		t.Errorf("summary: want %q, got %q", "name", g.SummaryFieldKey)
	}
}

func TestComputeLoopGroups_NestedLoop(t *testing.T) {
	fields := []template.Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "label", Type: "text"},
		{Key: "inner", Type: "loopstart"},
		{Key: "leaf", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "outer", Type: "loopstop"},
	}
	got := ComputeLoopGroups(fields, false)
	if len(got) != 2 {
		t.Fatalf("want 2 groups, got %d (%+v)", len(got), got)
	}

	// Outer first (start order), then inner.
	if got[0].Key != "outer" || got[0].Depth != 1 {
		t.Errorf("outer: %+v", got[0])
	}
	if got[1].Key != "inner" || got[1].Depth != 2 {
		t.Errorf("inner: %+v", got[1])
	}
	if got[0].StartIndex != 0 || got[0].StopIndex != 5 {
		t.Errorf("outer indices: %+v", got[0])
	}
	if got[1].StartIndex != 2 || got[1].StopIndex != 4 {
		t.Errorf("inner indices: %+v", got[1])
	}
}

func TestComputeLoopGroups_DefaultCollapsedFromConfig(t *testing.T) {
	fields := []template.Field{
		{Key: "k", Type: "loopstart"},
		{Key: "k", Type: "loopstop"},
	}
	got := ComputeLoopGroups(fields, true)
	if len(got) != 1 || !got[0].DefaultCollapsed {
		t.Errorf("config-collapsed should propagate: %+v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Unhappy paths — bad input must not panic; the result is best-effort.
// ─────────────────────────────────────────────────────────────────────

func TestComputeLoopGroups_NilFieldsIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil fields panicked: %v", r)
		}
	}()
	got := ComputeLoopGroups(nil, false)
	if got == nil {
		t.Errorf("want empty slice, got nil")
	}
}

func TestComputeLoopGroups_UnmatchedLoopstart(t *testing.T) {
	fields := []template.Field{
		{Key: "ghost", Type: "loopstart"},
		{Key: "leaf", Type: "text"},
	}
	got := ComputeLoopGroups(fields, false)
	// Best-effort behaviour: unpaired loops are dropped (validation
	// catches them elsewhere). Asserting "no panic + 0 groups" is the
	// floor; if we change behaviour later, update both here and
	// the doc comment in loops.go.
	if len(got) != 0 {
		t.Errorf("unmatched loopstart: want 0 groups, got %d (%+v)", len(got), got)
	}
}

func TestComputeLoopGroups_UnmatchedLoopstop(t *testing.T) {
	fields := []template.Field{
		{Key: "leaf", Type: "text"},
		{Key: "stranded", Type: "loopstop"},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unmatched loopstop panicked: %v", r)
		}
	}()
	got := ComputeLoopGroups(fields, false)
	if len(got) != 0 {
		t.Errorf("unmatched loopstop: want 0 groups, got %d", len(got))
	}
}

func TestComputeLoopGroups_KeyMismatchSkipsPair(t *testing.T) {
	// loopstart "a" closed by loopstop "b" — validation rejects this
	// upstream; here we just don't crash.
	fields := []template.Field{
		{Key: "a", Type: "loopstart"},
		{Key: "b", Type: "loopstop"},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("mismatched pair panicked: %v", r)
		}
	}()
	_ = ComputeLoopGroups(fields, false)
}
