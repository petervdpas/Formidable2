package render

// Service is the Wails-bound facade for the render module. Vue calls
// these to drive the Storage workspace's Render button.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// RenderForm - Handlebars markdown + sanitized HTML for one
// (template, datafile) pair. Empty datafile renders defaults.
func (s *Service) RenderForm(templateName, datafile string) (*Result, error) {
	return s.m.RenderForm(templateName, datafile)
}

// RenderMarkdown - Handlebars stage only. Useful when Vue wants to let
// the user edit the intermediate markdown before HTML rendering.
func (s *Service) RenderMarkdown(templateName, datafile string) (string, error) {
	return s.m.RenderMarkdown(templateName, datafile)
}

// RenderHTML - re-render arbitrary markdown (Vue's editor preview).
func (s *Service) RenderHTML(markdown string) (string, error) {
	return s.m.RenderHTMLOnly(markdown)
}

// RenderFullHTML - self-contained HTML document (DOCTYPE + head + body)
// with the formidable-prose stylesheet inlined. Used by the storage
// workspace's "Copy HTML" action so what the user pastes into a .html
// file renders identically to the in-app preview.
func (s *Service) RenderFullHTML(templateName, datafile string) (string, error) {
	return s.m.RenderFullHTML(templateName, datafile)
}

// ListHelpers - the catalog of every Handlebars helper this module
// registers, for the Information panel's "Render helpers reference".
// Static data (no Manager state read); returned as a fresh slice the
// frontend can sort/filter freely.
func (s *Service) ListHelpers() []HelperDescriptor {
	return Catalog()
}
