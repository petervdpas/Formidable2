package pdf

// Service is the Wails-bound surface over Manager. The Information
// workspace activation panel and any "Export as PDF" trigger call
// these methods directly — there is no HTTP handler peer, PDF
// generation stays Wails-only (see types.go).
type Service struct{ m *Manager }

// NewService wraps a Manager. Panics on nil — that's a composition-
// root bug and must surface at boot, not later in a rare branch.
func NewService(m *Manager) *Service {
	if m == nil {
		panic("pdf: NewService called with nil manager")
	}
	return &Service{m: m}
}

// GetStatus returns the live engine state. Cheap; safe to poll from
// the Information page status row.
func (s *Service) GetStatus() Status { return s.m.Status() }

// ProbeChrome lists every Chrome/Chromium binary the activation
// flow could adopt — env-var override, then system paths in their
// platform's conventional order, then the latest managed-cache
// pick. Empty Candidates means no Chrome was found; the dialog
// should offer the managed-download path (Phase D).
func (s *Service) ProbeChrome() ProbeResult { return s.m.Probe() }

// Activate is the user's "yes, set up PDF export" action. Stage 1
// always returns ErrPDFNotActivated; Stage 2 wires probing +
// managed-download flow.
func (s *Service) Activate(opts ActivateOpts) (Status, error) {
	return s.m.Activate(opts)
}

// Deactivate releases the bound Chrome binary without deleting any
// managed download. Stage 1 always returns ErrPDFNotActivated.
func (s *Service) Deactivate() error { return s.m.Deactivate() }

// ExportPDF renders the form identified by formGUID. Stage 1 always
// returns ErrPDFNotActivated; Stage 4 wires the render → picoloom →
// system.SaveFile pipeline.
func (s *Service) ExportPDF(formGUID string, opts ExportOpts) (Result, error) {
	return s.m.Export(formGUID, opts)
}

// SetExportDir adopts a per-machine "where PDFs land" preference.
// Empty path clears the preference. Non-empty paths must be
// absolute + existent + a directory — otherwise the service
// returns ErrInvalidExportDir, which the frontend should surface
// as a user-correctable error (typically a re-pick via the native
// folder picker). Independent of activation state.
func (s *Service) SetExportDir(path string) (Status, error) {
	return s.m.SetExportDir(path)
}

// GetDirectivesDoc returns the embedded markdown reference for the
// picoloom frontmatter directives in the requested locale. Unknown
// locale falls back to English. Static content — safe to call any
// time, cheap, no I/O beyond the embedded FS.
func (s *Service) GetDirectivesDoc(locale string) (string, error) {
	return directivesDoc(locale)
}
