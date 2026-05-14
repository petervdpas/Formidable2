package pdf

import (
	"log/slog"
	"sync"
)

// Manager owns the runtime activation state of the PDF engine. Stage
// 1 only models the inactive-by-default state; Stage 2 wires real
// Chrome probing + go-rod's managed download via activate.go.
//
// All exported methods are safe for concurrent use.
type Manager struct {
	log *slog.Logger

	mu     sync.RWMutex
	status Status
}

// NewManager constructs an inactive manager. Composition root calls
// once at boot; the active profile's persisted `pdf:` block (added in
// Stage 2) seeds the initial state through a later Restore() call.
func NewManager(log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		log:    log,
		status: Status{Source: SourceUnset},
	}
}

// Status returns the live snapshot. Zero value (Active=false,
// Source=SourceUnset) is the fresh-install state.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Activate is a Stage 2 entry point. Stage 1 always reports
// ErrPDFNotActivated so the frontend's activation panel is the only
// path that touches Chrome — there are no "activate by side effect"
// flows.
func (m *Manager) Activate(opts ActivateOpts) (Status, error) {
	m.log.Debug("pdf: activate", "browser_bin", opts.BrowserBin, "allow_download", opts.AllowDownload)
	return m.Status(), ErrPDFNotActivated
}

// Deactivate is a Stage 2 entry point. Stage 1 reports
// ErrPDFNotActivated rather than silently succeeding — calling
// Deactivate before Activate is a frontend bug, not an idempotent
// no-op.
func (m *Manager) Deactivate() error {
	m.log.Debug("pdf: deactivate")
	return ErrPDFNotActivated
}

// Export is a Stage 4 entry point. Stage 1 short-circuits on the
// activation check so the rendering pipeline is never constructed
// while inactive — Chrome must not boot for an inactive engine.
func (m *Manager) Export(formGUID string, opts ExportOpts) (Result, error) {
	m.log.Debug("pdf: export", "form_guid", formGUID, "output_path", opts.OutputPath, "style", opts.Style)
	return Result{}, ErrPDFNotActivated
}
