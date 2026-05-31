package wiki

import (
	"errors"
	"fmt"
)

// Service is the Wails surface for runtime control of the wiki HTTP server. External actions
// (open browser/window) are delegated to hooks so the module stays free of os/exec and wails deps.
type Service struct {
	m           *Manager
	port        func() int             // resolves the configured port at call time
	openBrowser func(url string) error // nil → not supported on this build
	openWindow  func(url string) error // nil → not supported on this build
}

// NewService builds the service; port is invoked per StartServer so a config change is picked up without reconstruction.
func NewService(m *Manager, port func() int, openBrowser, openWindow func(url string) error) *Service {
	if m == nil {
		panic("wiki: NewService called with nil manager")
	}
	if port == nil {
		port = func() int { return 0 }
	}
	return &Service{
		m:           m,
		port:        port,
		openBrowser: openBrowser,
		openWindow:  openWindow,
	}
}

// InstallWindowOpener installs the in-app webview opener after the Wails app is built.
// A package function, not a method, so Wails' binding generator doesn't expose it to the frontend.
func InstallWindowOpener(s *Service, fn func(url string) error) {
	if s == nil {
		return
	}
	s.openWindow = fn
}

// StartServer boots the HTTP listener on the configured port.
func (s *Service) StartServer() error {
	return s.m.Start(s.port())
}

// StopServer gracefully shuts the listener down (no-op when idle).
func (s *Service) StopServer() error {
	return s.m.Stop()
}

// GetServerStatus snapshots the current state.
func (s *Service) GetServerStatus() ServerStatus {
	return s.m.Status()
}

// OpenInBrowser loads the wiki root URL in the host's default browser; requires the server running.
func (s *Service) OpenInBrowser() error {
	url, err := s.rootURL()
	if err != nil {
		return err
	}
	if s.openBrowser == nil {
		return errors.New("wiki: OpenInBrowser not supported on this build")
	}
	return s.openBrowser(url)
}

// OpenInternalWiki spawns an in-app Wails webview window at the wiki root URL.
func (s *Service) OpenInternalWiki() error {
	url, err := s.rootURL()
	if err != nil {
		return err
	}
	if s.openWindow == nil {
		return errors.New("wiki: OpenInternalWiki not wired up")
	}
	return s.openWindow(url)
}

// OpenAPIDocsInBrowser opens the Swagger UI page in the host's default browser.
func (s *Service) OpenAPIDocsInBrowser() error {
	url, err := s.urlFor("/api/docs/")
	if err != nil {
		return err
	}
	if s.openBrowser == nil {
		return errors.New("wiki: OpenAPIDocsInBrowser not supported on this build")
	}
	return s.openBrowser(url)
}

// OpenAPIDocsInWindow spawns the in-app webview window at the Swagger UI URL.
func (s *Service) OpenAPIDocsInWindow() error {
	url, err := s.urlFor("/api/docs/")
	if err != nil {
		return err
	}
	if s.openWindow == nil {
		return errors.New("wiki: OpenAPIDocsInWindow not wired up")
	}
	return s.openWindow(url)
}

func (s *Service) rootURL() (string, error) {
	return s.urlFor("/")
}

// urlFor builds a loopback URL, erroring when the server isn't running.
func (s *Service) urlFor(path string) (string, error) {
	st := s.m.Status()
	if !st.Running {
		return "", errors.New("wiki: server is not running")
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", st.Port, path), nil
}
