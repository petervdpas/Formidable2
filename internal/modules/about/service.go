package about

import "errors"

// Service is the Wails-facing surface. The identity values are
// compile-time constants; the only state is the injected browser
// opener used by OpenWebsite, supplied by the composition root so the
// module stays free of os/exec (same pattern as wiki.Service).
type Service struct {
	openBrowser func(string) error
}

func NewService(openBrowser func(string) error) *Service {
	return &Service{openBrowser: openBrowser}
}

func (s *Service) GetInfo() Info {
	return Info{
		Name:    Name,
		Version: Version,
		Tagline: Tagline,
		Author:  Author,
		Website: Website,
	}
}

// OpenWebsite asks the host platform's default browser to load the
// project homepage. The opener is platform-specific and lives in the
// composition root.
func (s *Service) OpenWebsite() error {
	if s.openBrowser == nil {
		return errors.New("about: OpenWebsite not supported on this build")
	}
	return s.openBrowser(Website)
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
