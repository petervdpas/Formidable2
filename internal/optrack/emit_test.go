package optrack

import "testing"

type captureEmitter struct {
	last []Status
	hits int
}

func (c *captureEmitter) Emit(name string, data any) {
	if name != "optrack:changed" {
		return
	}
	c.hits++
	if snap, ok := data.([]Status); ok {
		c.last = snap
	}
}

// Begin and Done announce the membership change so a reloaded frontend reflects
// what is running without polling: the snapshot rides the event.
func TestRegistry_EmitsSnapshotOnBeginAndDone(t *testing.T) {
	cap := &captureEmitter{}
	r := NewRegistry()
	r.SetEmitter(cap)

	h := r.Begin("pdf:export")
	if cap.hits != 1 || len(cap.last) != 1 || cap.last[0].Kind != "pdf:export" {
		t.Fatalf("begin should emit a 1-op snapshot, got hits=%d last=%+v", cap.hits, cap.last)
	}

	h.Done()
	if cap.hits != 2 || len(cap.last) != 0 {
		t.Fatalf("done should emit an empty snapshot, got hits=%d last=%+v", cap.hits, cap.last)
	}
}

// A refused TryBegin changes nothing, so it must not emit.
func TestRegistry_TryBeginRejection_DoesNotEmit(t *testing.T) {
	cap := &captureEmitter{}
	r := NewRegistry()
	r.SetEmitter(cap)

	r.TryBegin("git:pull")
	before := cap.hits
	if r.TryBegin("git:pull") != nil {
		t.Fatal("second same-kind TryBegin should be refused")
	}
	if cap.hits != before {
		t.Errorf("a refused TryBegin must not emit, hits went %d -> %d", before, cap.hits)
	}
}

// A stale handle (op already gone) must not emit a redundant change.
func TestRegistry_StaleHandle_DoesNotEmit(t *testing.T) {
	cap := &captureEmitter{}
	r := NewRegistry()
	r.SetEmitter(cap)

	h := r.Begin("git:clone")
	h.Done()
	at := cap.hits
	h.Done() // already gone
	if cap.hits != at {
		t.Errorf("a stale Done must not emit, hits went %d -> %d", at, cap.hits)
	}
}

// A nil emitter is safe: the registry still works, it just announces nothing.
func TestRegistry_NilEmitterIsSafe(t *testing.T) {
	r := NewRegistry()
	h := r.Begin("git:push")
	h.Done()
	if len(r.List()) != 0 {
		t.Errorf("registry should still work without an emitter, got %+v", r.List())
	}
}
