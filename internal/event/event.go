// Package event defines the single Emitter contract backend services use to
// announce changes to the frontend (the backend is the source of truth; the
// frontend reacts). The app installs a Wails-backed relay as the Emitter.
package event

// Emitter publishes a named event with a payload to the host transport.
type Emitter interface {
	Emit(name string, data any)
}

// Emit fires name on e when e is non-nil; a nil emitter is a silent no-op,
// so callers never need their own nil guard.
func Emit(e Emitter, name string, data any) {
	if e != nil {
		e.Emit(name, data)
	}
}
