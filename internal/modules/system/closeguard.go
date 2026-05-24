package system

import "sync/atomic"

// Window-close guard. The frontend mirrors the active form's dirty flag
// into the backend via SetUnsavedChanges; the WindowClosing hook in
// main.go consults UnsavedChanges() and vetoes an OS-driven close
// (emitting "app:close-requested") so the SPA can show its Save /
// Discard / Cancel dialog. ConfirmClose lifts the veto and quits once
// the user has chosen to leave.
var (
	unsavedChanges atomic.Bool
	allowClose     atomic.Bool
)

// UnsavedChanges reports whether the frontend has flagged unsaved work.
// main.go's WindowClosing hook reads this to decide whether to veto.
func UnsavedChanges() bool { return unsavedChanges.Load() }

// AllowClose reports whether the user has confirmed leaving. Once true,
// the WindowClosing hook stops vetoing.
func AllowClose() bool { return allowClose.Load() }

// SetUnsavedChanges lets the frontend keep the backend in sync with the
// active form's dirty state. Called from a watcher in StorageWorkspace.
func (s *Service) SetUnsavedChanges(b bool) { unsavedChanges.Store(b) }

// ConfirmClose lifts the close veto and quits. The frontend calls this
// after its unsaved-changes guard resolves to Save or Discard.
func (s *Service) ConfirmClose() {
	allowClose.Store(true)
	s.Quit()
}
