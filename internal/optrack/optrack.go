// Package optrack is the backend's single source of truth for in-flight
// operations. Any long-running op (git/gigot sync, PDF export, reindex,
// cleanup, plugin run) registers a handle, reports progress, and ends; the
// frontend reflects or resumes from the registry instead of holding its own
// state. Queuing is a thin layer to add on top the day an op needs it.
package optrack

import (
	"sort"
	"strconv"
	"sync"

	"github.com/petervdpas/formidable2/internal/event"
)

// State is the lifecycle of a tracked op; only active ops appear in List.
type State string

const (
	Running State = "running"
	Queued  State = "queued"
)

// Status is a snapshot of one tracked op, for the frontend to reflect.
type Status struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	State   State  `json:"state"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Label   string `json:"label,omitempty"`
}

// Registry tracks in-flight ops. Use NewRegistry; the zero value is not usable.
type Registry struct {
	mu   sync.Mutex
	seq  int
	ops  map[int]*Status
	emit event.Emitter
}

func NewRegistry() *Registry {
	return &Registry{ops: map[int]*Status{}}
}

// changedEvent rides every membership change so the frontend reflects what is
// running without polling; the snapshot is the event payload.
const changedEvent = "optrack:changed"

// SetEmitter installs the transport so begin/end announce optrack:changed,
// keeping the backend the single source of truth. Nil is safe (no announce).
func (r *Registry) SetEmitter(e event.Emitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emit = e
}

// snapshotLocked builds the active-op list; caller holds the mutex.
func (r *Registry) snapshotLocked() []Status {
	ids := make([]int, 0, len(r.ops))
	for id := range r.ops {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		out = append(out, *r.ops[id])
	}
	return out
}

// Begin registers a running op and returns its handle (allows concurrent ops of any kind).
func (r *Registry) Begin(kind string) *Handle {
	r.mu.Lock()
	h := r.beginLocked(kind)
	emit, snap := r.emit, r.snapshotLocked()
	r.mu.Unlock()
	event.Emit(emit, changedEvent, snap)
	return h
}

// TryBegin registers a running op only when no op of the same kind is already
// running; it returns nil otherwise. This is the "cannot run twice" guard for
// ops that must not overlap (e.g. a gigot reclone). A refusal changes nothing
// and emits nothing.
func (r *Registry) TryBegin(kind string) *Handle {
	r.mu.Lock()
	for _, op := range r.ops {
		if op.Kind == kind {
			r.mu.Unlock()
			return nil
		}
	}
	h := r.beginLocked(kind)
	emit, snap := r.emit, r.snapshotLocked()
	r.mu.Unlock()
	event.Emit(emit, changedEvent, snap)
	return h
}

func (r *Registry) beginLocked(kind string) *Handle {
	r.seq++
	id := r.seq
	r.ops[id] = &Status{ID: strconv.Itoa(id), Kind: kind, State: Running}
	return &Handle{r: r, id: id}
}

// List returns active ops in begin order: a stable snapshot for the frontend.
func (r *Registry) List() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

// Handle controls one tracked op; methods on a finished op are safe no-ops.
type Handle struct {
	r  *Registry
	id int
}

// Note updates the op's progress.
func (h *Handle) Note(current, total int, label string) {
	if h == nil {
		return
	}
	h.r.mu.Lock()
	defer h.r.mu.Unlock()
	if op := h.r.ops[h.id]; op != nil {
		op.Current, op.Total, op.Label = current, total, label
	}
}

// Done removes the op from the in-flight list (success).
func (h *Handle) Done() { h.remove() }

// Fail removes the op from the in-flight list (the caller surfaces the error itself).
func (h *Handle) Fail() { h.remove() }

func (h *Handle) remove() {
	if h == nil {
		return
	}
	r := h.r
	r.mu.Lock()
	_, existed := r.ops[h.id]
	delete(r.ops, h.id)
	emit, snap := r.emit, r.snapshotLocked()
	r.mu.Unlock()
	if existed {
		event.Emit(emit, changedEvent, snap)
	}
}
