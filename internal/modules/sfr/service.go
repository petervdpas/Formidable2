package sfr

// Service is the api layer over Manager. Wails-only by design: callers supply
// a directory path joined with a base filename, too generic to expose over the
// loopback HTTP API.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) ListFiles(directory, extension string) ([]string, error) {
	return s.m.ListFiles(directory, extension)
}

func (s *Service) LoadFromBase(directory, baseFilename string, opts Options) (any, error) {
	return s.m.LoadFromBase(directory, baseFilename, opts)
}

func (s *Service) SaveFromBase(directory, baseFilename string, data any, opts Options) SaveResult {
	return s.m.SaveFromBase(directory, baseFilename, data, opts)
}

func (s *Service) DeleteFromBase(directory, baseFilename string, opts Options) error {
	return s.m.DeleteFromBase(directory, baseFilename, opts)
}
