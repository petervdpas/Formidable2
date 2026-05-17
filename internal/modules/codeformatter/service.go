package codeformatter

// Service is the Wails-bound surface for the code formatter. One
// method: Format(lang, src). The frontend CodeEditor calls this on its
// Format button instead of running prettier in the webview, so the
// YAML inside markdown frontmatter blocks gets a real parser pass
// (yaml.v3) and doesn't depend on what survived the paste.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Format reformats src according to lang. Errors are returned as
// strings via Wails — the frontend toasts ErrMalformed cases so the
// user can fix the source rather than silently shipping broken YAML.
func (s *Service) Format(lang, src string) (string, error) {
	return s.m.Format(lang, src)
}
