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

// AddEdge links a source record to a target record (and mirrors the reversed edge).
func (s *Service) AddEdge(source, target string, edge Edge) error {
	return s.m.AddEdge(source, target, edge)
}

// RemoveEdge unlinks a source record from a target record (and removes the mirror).
func (s *Service) RemoveEdge(source, target string, edge Edge) error {
	return s.m.RemoveEdge(source, target, edge)
}
