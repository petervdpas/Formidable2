package relation

// Service is the Wails-facing surface over Manager: get and set a template's relations.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// GetRelations returns the relations declared by a template.
func (s *Service) GetRelations(template string) ([]Relation, error) {
	return s.m.GetRelations(template)
}

// SetRelations replaces and persists a template's relations.
func (s *Service) SetRelations(template string, relations []Relation) error {
	return s.m.SetRelations(template, relations)
}

// AddEdge links two records through a named relation.
func (s *Service) AddEdge(template, relationName string, edge Edge) error {
	return s.m.AddEdge(template, relationName, edge)
}

// RemoveEdge unlinks two records from a named relation.
func (s *Service) RemoveEdge(template, relationName string, edge Edge) error {
	return s.m.RemoveEdge(template, relationName, edge)
}
