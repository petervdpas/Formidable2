package optrack

import "testing"

func TestRegistry_TracksAnInFlightOp(t *testing.T) {
	r := NewRegistry()
	if len(r.List()) != 0 {
		t.Fatalf("a fresh registry is empty, got %+v", r.List())
	}

	h := r.Begin("reclone")
	got := r.List()
	if len(got) != 1 || got[0].Kind != "reclone" || got[0].State != Running {
		t.Fatalf("want one running reclone, got %+v", got)
	}

	h.Note(5, 10, "templates/x.yaml")
	got = r.List()
	if got[0].Current != 5 || got[0].Total != 10 || got[0].Label != "templates/x.yaml" {
		t.Errorf("progress not recorded, got %+v", got[0])
	}

	h.Done()
	if len(r.List()) != 0 {
		t.Errorf("a finished op leaves the in-flight list, got %+v", r.List())
	}
}

// Two concurrent ops are tracked independently and listed in begin order.
func TestRegistry_MultipleOps(t *testing.T) {
	r := NewRegistry()
	a := r.Begin("reclone")
	b := r.Begin("pull")

	got := r.List()
	if len(got) != 2 || got[0].Kind != "reclone" || got[1].Kind != "pull" {
		t.Fatalf("want reclone then pull, got %+v", got)
	}

	a.Done()
	got = r.List()
	if len(got) != 1 || got[0].Kind != "pull" {
		t.Fatalf("only pull should remain, got %+v", got)
	}
	b.Fail()
	if len(r.List()) != 0 {
		t.Errorf("both finished, list should be empty, got %+v", r.List())
	}
}

// TryBegin refuses a second op of the same kind while one is running (the
// "cannot run twice" guard), but allows it once the first finishes and allows
// a different kind concurrently.
func TestRegistry_TryBegin_RejectsDuplicateKind(t *testing.T) {
	r := NewRegistry()

	h1 := r.TryBegin("reclone")
	if h1 == nil {
		t.Fatal("first reclone should start")
	}
	if r.TryBegin("reclone") != nil {
		t.Error("a second reclone must be rejected while one runs")
	}
	if other := r.TryBegin("pull"); other == nil {
		t.Error("a different kind should be allowed concurrently")
	}

	h1.Done()
	if again := r.TryBegin("reclone"); again == nil {
		t.Error("reclone should be allowed again after the first finished")
	}
}

// Note/Done on a stale handle (op already gone) is a safe no-op.
func TestRegistry_StaleHandleIsSafe(t *testing.T) {
	r := NewRegistry()
	h := r.Begin("reclone")
	h.Done()
	h.Note(1, 2, "x") // must not panic or resurrect
	h.Done()
	if len(r.List()) != 0 {
		t.Errorf("stale handle resurrected an op: %+v", r.List())
	}
}
