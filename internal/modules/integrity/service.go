package integrity

// Service is the Wails-facing surface for the Cleanup Storage dialog.
//
// Analyze is read-only and runs the drift report against the
// template's stored forms.
//
// Fix takes a per-kind plan and applies the chosen strategy to every
// matching issue. Behaviour is opt-in: kinds without a plan item are
// left untouched, and FixSkip is the explicit no-op. The frontend
// never calls Fix automatically — only on a deliberate "Repair
// Selected" button press.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Analyze inspects every form under templateFilename and returns the
// drift report. An error means the scan couldn't start (unknown
// template, list failure); per-form parse failures land in the report
// as IssueUnreadable.
func (s *Service) Analyze(templateFilename string) (Report, error) {
	return s.m.AnalyzeTemplate(templateFilename)
}

// Fix applies plan to the template's stored forms and returns a per-
// form outcome bundle. ScannedAfter on the result is the residual
// issue count from a fresh analyze pass, so the frontend can render
// "X repaired, Y remain" without a second round-trip.
func (s *Service) Fix(templateFilename string, plan FixPlan) (FixResult, error) {
	return s.m.FixTemplate(templateFilename, plan)
}
