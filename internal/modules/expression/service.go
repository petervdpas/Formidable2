package expression

// Service is the Wails-bound facade for the expression module. Vue
// calls Evaluate for one-off expressions (e.g. plugin commands or a
// hypothetical preview pane in the template editor) and
// EvaluateSidebar to populate the Storage workspace's per-row
// sub-labels.
type Service struct{ m *Manager }

// NewService wraps a Manager. Service stays thin so all behaviour
// (cache, helpers, narrow-context defence) lives on Manager — the
// Wails surface adds nothing beyond IDL-style passthrough.
func NewService(m *Manager) *Service { return &Service{m: m} }

// Evaluate runs one expression against an arbitrary context. Returns
// a normalised SidebarItem so callers get the same shape whether the
// expression returns a string, list, or struct.
func (s *Service) Evaluate(src string, ctx map[string]any) (SidebarItem, error) {
	return s.m.Evaluate(src, ctx)
}

// EvaluateSidebar renders the sub-label for every record in a
// template's storage list. Returns ErrNoExpression when the template
// has no sidebar_expression configured — the frontend should hide
// the sub-label entirely in that case rather than render anything.
func (s *Service) EvaluateSidebar(templateName string) ([]SidebarItem, error) {
	return s.m.EvaluateSidebar(templateName)
}
