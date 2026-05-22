package about

// Service is the Wails-facing surface. Stateless on purpose - the
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

// GetLibraries returns the canonical credits list - the source of
// truth for the About panel's "Special thanks to" section. Frontend
// renders one row per entry and looks up the per-locale description
// via i18n. Returns a fresh slice so the bound caller can't mutate
// the package-level Libraries.
func (s *Service) GetLibraries() []Library {
	out := make([]Library, len(Libraries))
	copy(out, Libraries)
	return out
}
