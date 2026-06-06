package relation

// Service is the Wails-facing surface over Manager: get and set a template's relations.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Cardinalities returns the cardinality picker options (value + label key) for
// the editor, so the frontend keeps no value->label mapping of its own.
func (s *Service) Cardinalities() []CardinalityOption {
	return CardinalityOptions()
}

// Reconcile runs a self-heal pass: recreate missing counterparts and report cardinality conflicts.
func (s *Service) Reconcile() (ReconcileReport, error) {
	return s.m.Reconcile()
}

// GetRelations returns the relations declared by a template.
func (s *Service) GetRelations(template string) ([]Relation, error) {
	return s.m.GetRelations(template)
}

// SetRelations replaces and persists a template's relations.
func (s *Service) SetRelations(template string, relations []Relation) error {
	return s.m.SetRelations(template, relations)
}

// AddEdge links two records through the relation from template to `to`.
func (s *Service) AddEdge(template, to string, edge Edge) error {
	return s.m.AddEdge(template, to, edge)
}

// RemoveEdge unlinks two records from the relation from template to `to`.
func (s *Service) RemoveEdge(template, to string, edge Edge) error {
	return s.m.RemoveEdge(template, to, edge)
}
