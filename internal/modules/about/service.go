package about

// Service is the Wails-facing surface. Stateless on purpose — the
// values are compile-time constants.
type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) GetInfo() Info {
	return Info{
		Name:    Name,
		Version: Version,
		Tagline: Tagline,
		Author:  Author,
	}
}
