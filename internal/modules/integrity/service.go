package integrity

// Service is the Wails surface for the Cleanup Storage dialog.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Analyze returns the drift report for templateFilename's forms.
func (s *Service) Analyze(templateFilename string) (Report, error) {
	return s.m.AnalyzeTemplate(templateFilename)
}

// Fix applies plan to the template's forms and returns the per-form outcome bundle.
func (s *Service) Fix(templateFilename string, plan FixPlan) (FixResult, error) {
	return s.m.FixTemplate(templateFilename, plan)
}
