package mermaid

// Service is the Wails facade for Mermaid validation. Stateless: validation is
// a pure function over the source string.
type Service struct{}

func NewService() *Service { return &Service{} }

// Validate parses Mermaid source and reports its canonical diagram type plus
// any positioned issues. Used by the field editor (live) and on save.
func (s *Service) Validate(source string) Result { return Validate(source) }
