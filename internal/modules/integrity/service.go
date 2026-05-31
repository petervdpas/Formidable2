package integrity

import "github.com/petervdpas/formidable2/internal/event"

// Service is the Wails surface for the Cleanup Storage dialog.
type Service struct {
	m    *Manager
	emit event.Emitter
}

func NewService(m *Manager, emit event.Emitter) *Service { return &Service{m: m, emit: emit} }

// Analyze returns the drift report for templateFilename's forms.
func (s *Service) Analyze(templateFilename string) (Report, error) {
	return s.m.AnalyzeTemplate(templateFilename)
}

// Fix applies plan to the template's forms and returns the per-form outcome bundle.
// When it actually writes forms it emits storage:changed so the frontend reloads them.
func (s *Service) Fix(templateFilename string, plan FixPlan) (FixResult, error) {
	res, err := s.m.FixTemplate(templateFilename, plan)
	if err == nil && res.FormsSaved > 0 {
		event.Emit(s.emit, "storage:changed", templateFilename)
	}
	return res, err
}
