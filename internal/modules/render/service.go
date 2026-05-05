package render

// Service is the Wails-bound facade for the render module. Vue calls
// these to drive the Storage workspace's Render button.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// RenderForm — Handlebars markdown + sanitized HTML for one
// (template, datafile) pair. Empty datafile renders defaults.
func (s *Service) RenderForm(templateName, datafile string) (*Result, error) {
	return s.m.RenderForm(templateName, datafile)
}

// RenderMarkdown — Handlebars stage only. Useful when Vue wants to let
// the user edit the intermediate markdown before HTML rendering.
func (s *Service) RenderMarkdown(templateName, datafile string) (string, error) {
	return s.m.RenderMarkdown(templateName, datafile)
}

// RenderHTML — re-render arbitrary markdown (Vue's editor preview).
func (s *Service) RenderHTML(markdown string) (string, error) {
	return s.m.RenderHTMLOnly(markdown)
}
