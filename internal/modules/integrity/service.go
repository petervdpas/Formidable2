package integrity

// Service is the Wails-facing surface. Phase 1 exposes only Analyze;
// phase 2 will add a Fix endpoint that takes a Report (or a filtered
// subset) and rewrites the affected forms.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Analyze inspects every form under templateFilename and returns the
// drift report. An error means the scan couldn't start (unknown
// template, list failure); per-form parse failures land in the report
// as IssueUnreadable.
func (s *Service) Analyze(templateFilename string) (Report, error) {
	return s.m.AnalyzeTemplate(templateFilename)
}
