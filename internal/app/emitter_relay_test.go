package app

import "testing"

// The relay reconciles the index before forwarding a context:reloaded, so the
// frontend's subsequent re-read never sees an orphan row (a discarded new
// record whose file is already gone). Ordering matters: reconcile must run
// before the transport fires.
func TestEmitterRelay_ContextReloaded_ReconcilesBeforeForward(t *testing.T) {
	var order []string
	e := &emitterRelay{}
	e.setReconcile(func() { order = append(order, "reconcile") })
	e.set(func(name string, _ any) { order = append(order, "emit:"+name) })

	e.Emit("context:reloaded", nil)

	if len(order) != 2 || order[0] != "reconcile" || order[1] != "emit:context:reloaded" {
		t.Fatalf("order = %v, want [reconcile emit:context:reloaded]", order)
	}
}

// Unrelated events must not trigger a reconcile: only a working-tree change
// (context:reloaded) invalidates the index.
func TestEmitterRelay_OtherEvents_DoNotReconcile(t *testing.T) {
	reconciled := 0
	e := &emitterRelay{}
	e.setReconcile(func() { reconciled++ })
	e.set(func(string, any) {})

	e.Emit("nav:changed", nil)
	e.Emit("plugin:run:bar", nil)
	e.Emit("storage:changed", "adapters.yaml")

	if reconciled != 0 {
		t.Fatalf("reconcile ran %d times for non-reload events, want 0", reconciled)
	}
}

// A context:reloaded with no reconcile hook installed (early boot, before
// SetReconcile) must still forward cleanly rather than panic.
func TestEmitterRelay_ContextReloaded_NoHook_StillForwards(t *testing.T) {
	forwarded := false
	e := &emitterRelay{}
	e.set(func(name string, _ any) {
		if name == "context:reloaded" {
			forwarded = true
		}
	})

	e.Emit("context:reloaded", nil)

	if !forwarded {
		t.Fatal("context:reloaded was not forwarded when no reconcile hook is set")
	}
}
