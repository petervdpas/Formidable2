package wiki

import (
	"errors"
	"fmt"
)

// Service is the Wails-exposed surface for runtime control of the
// wiki HTTP server. The About workspace toggle calls these methods;
// future monitoring (request log etc.) will hang off the same
// service.
//
// External actions (open in system browser, open in-app webview
// window) are delegated to function hooks the composition root
// installs at construction time. Keeps the wiki module free of any
// `os/exec` or wails dependency — testable in pure Go.
type Service struct {
	m           *Manager
	port        func() int             // resolves the configured port at call time
	openBrowser func(url string) error // nil → not supported on this build
	openWindow  func(url string) error // nil → not supported on this build
}

// NewService builds the service. Panics on nil manager — that's a
// composition-root bug and must surface immediately, not later in a
// rare branch. `port` is invoked on each StartServer so a config
// change between starts picks up the new value without reconstruction.
// `openBrowser` and `openWindow` may both be nil; callers see a
// clean error rather than a panic.
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

// InstallWindowOpener installs the function used by OpenInternalWiki
// to spawn an in-app webview window. main.go calls this after the
// Wails application is built (the application pointer doesn't exist
// when the composition root constructs this service). Pass nil to
// clear.
//
// Defined as a package-level function rather than a Service method so
// Wails' binding generator doesn't expose it to the frontend — the
// hook is purely a wiring concern between main.go and the service,
// not part of the Wails-callable API surface.
func InstallWindowOpener(s *Service, fn func(url string) error) {
	if s == nil {
		return
	}
	s.openWindow = fn
}

// StartServer boots the HTTP listener on the currently-configured
// port. Already-running → returns Manager's "already running" error.
func (s *Service) StartServer() error {
	return s.m.Start(s.port())
}

// StopServer gracefully shuts the listener down. No-op when idle.
func (s *Service) StopServer() error {
	return s.m.Stop()
}

// GetServerStatus snapshots the current state. Cheap; safe to poll.
func (s *Service) GetServerStatus() ServerStatus {
	return s.m.Status()
}

// OpenInBrowser asks the host platform's default browser to load the
// wiki root URL. Requires the server to be running so the URL
// actually responds. The opener function is platform-specific (xdg-
// open / open / cmd start) and lives in the composition root.
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

// OpenInternalWiki spawns a new Wails webview window at the wiki
// root URL. Equivalent to "open in browser" but stays inside the
// app — the user gets a dedicated window without leaving Formidable.
// Wails 3 alpha 84 supports this via application.NewWebviewWindow;
// the composition root supplies the bound function.
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

// rootURL builds the loopback URL for the running server. Returns
// an error when the server is not running — opening a URL that
// would 502 isn't useful UX.
func (s *Service) rootURL() (string, error) {
	st := s.m.Status()
	if !st.Running {
		return "", errors.New("wiki: server is not running")
	}
	return fmt.Sprintf("http://127.0.0.1:%d/", st.Port), nil
}
