package optrack

import (
	"errors"
	"fmt"
)

// ErrAlreadyRunning marks a Guard refusal: an op of the same kind is in flight.
var ErrAlreadyRunning = errors.New("already running")

// Guard is the standard "cannot run twice" bracket for a long op: it begins an
// op of kind via TryBegin and returns its handle plus a release func to defer.
// When an op of the same kind already runs it returns ErrAlreadyRunning (wrapped
// with the kind). A nil registry is unguarded: nil handle, a no-op release, no
// error, so callers stay one shape whether wired or not. The release runs on
// success, error, or panic, so the guard cannot get stuck open (no app restart).
func Guard(reg *Registry, kind string) (*Handle, func(), error) {
	if reg == nil {
		return nil, func() {}, nil
	}
	h := reg.TryBegin(kind)
	if h == nil {
		return nil, func() {}, fmt.Errorf("%s: %w", kind, ErrAlreadyRunning)
	}
	return h, h.Done, nil
}
