package viewer

import (
	"fmt"
	"os"
	"path/filepath"
)

// BundleInfo summarizes the currently-open bundle for the UI.
type BundleInfo struct {
	Loaded bool   `json:"loaded"`
	Name   string `json:"name"`
}

// RecentInfo is a remembered bundle path with display metadata.
type RecentInfo struct {
	Path   string `json:"path"`
	Name   string `json:"name"`   // base name for display
	Exists bool   `json:"exists"` // file still present on disk
}

// BundleChangedEventName is emitted from the composition root whenever the open
// bundle changes; the Vue shell listens for it to refresh. Must stay in sync
// with the frontend's BundleChangedEvent constant.
const BundleChangedEventName = "viewer:bundle-changed"

// ServerStatus reflects the optional LAN HTTP server.
type ServerStatus struct {
	Running bool     `json:"running"`
	Port    int      `json:"port"`
	URLs    []string `json:"urls"`
}

// Service is the viewer's bound Wails surface. The Vue shell calls it to open
// bundles, read and write config, list recents, and control the LAN server.
// Wails-specific behavior (the native open dialog, retitle + reload after a
// swap) is injected from the composition root so this stays testable.
type Service struct {
	store  *ConfigStore
	server *Server
	http   *HTTPServer
	frame  *HTTPServer            // always-on loopback server the iframe loads from
	open   func() (string, error) // native open-file dialog; "" == cancelled
	onSwap func()                 // called after a bundle swaps
}

// NewService wires the store, the current-bundle server, and the optional LAN
// server together.
func NewService(store *ConfigStore, server *Server, httpSrv *HTTPServer) *Service {
	return &Service{store: store, server: server, http: httpSrv}
}

// SetOpenFunc injects the native open-file dialog (returns "" when cancelled).
func (s *Service) SetOpenFunc(f func() (string, error)) { s.open = f }

// SetSwapHook injects the after-swap callback (retitle + reload the webview).
func (s *Service) SetSwapHook(f func()) { s.onSwap = f }

// SetFrameServer wires the loopback server the shell's iframe loads the bundle
// from. Serving over a real http:// origin avoids the WebKitGTK limitation
// where the app's custom URI scheme will not render inside a sub-frame.
func (s *Service) SetFrameServer(h *HTTPServer) { s.frame = h }

// BundleURL is the URL the shell points its iframe at. It is the loopback frame
// server's address, falling back to the in-app /bundle/ mount if that server is
// not running.
func (s *Service) BundleURL() string {
	if s.frame != nil {
		if p := s.frame.Port(); p != 0 {
			return fmt.Sprintf("http://127.0.0.1:%d/", p)
		}
	}
	return "/bundle/"
}

// GetConfig returns the persisted config.
func (s *Service) GetConfig() Config { return s.store.Load() }

// SetConfig persists cfg and applies it (starting or stopping the LAN server),
// returning the normalized, applied config.
func (s *Service) SetConfig(cfg Config) (Config, error) {
	if err := s.store.Save(cfg); err != nil {
		return s.store.Load(), err
	}
	applied := s.store.Load()
	if err := s.applyServer(applied); err != nil {
		return applied, err
	}
	return applied, nil
}

// Apply reflects the persisted config at startup (e.g. auto-starts the LAN
// server when enabled).
func (s *Service) Apply() error { return s.applyServer(s.store.Load()) }

func (s *Service) applyServer(cfg Config) error {
	if s.http == nil {
		return nil
	}
	if cfg.ServeHTTP {
		return s.http.Start(cfg.HTTPPort)
	}
	return s.http.Stop()
}

// Languages lists the supported UI languages (English first).
func (s *Service) Languages() []string { return SupportedLanguages }

// EffectiveLanguage resolves the configured language ("system" -> OS locale).
func (s *Service) EffectiveLanguage() string { return ResolveLanguage(s.store.Load().Language) }

// Messages returns the UI strings for lang, or for the effective language when
// lang is empty.
func (s *Service) Messages(lang string) map[string]string {
	if lang == "" {
		lang = s.EffectiveLanguage()
	}
	return Messages(lang)
}

// Recents lists remembered bundles, newest first, flagged by on-disk existence.
func (s *Service) Recents() []RecentInfo {
	cfg := s.store.Load()
	out := make([]RecentInfo, 0, len(cfg.RecentBundles))
	for _, p := range cfg.RecentBundles {
		out = append(out, RecentInfo{Path: p, Name: filepath.Base(p), Exists: fileExists(p)})
	}
	return out
}

// OpenDialog shows the native file picker and opens the chosen bundle. A
// cancelled dialog leaves the current bundle unchanged.
func (s *Service) OpenDialog() (BundleInfo, error) {
	if s.open == nil {
		return s.Current(), nil
	}
	path, err := s.open()
	if err != nil || path == "" {
		return s.Current(), err
	}
	return s.OpenPath(path)
}

// OpenPath opens the bundle at path, swaps it in, records it as recent, and
// fires the swap hook. Used by the Open dialog, recents, and file drops.
func (s *Service) OpenPath(path string) (BundleInfo, error) {
	b, err := OpenBundle(path)
	if err != nil {
		return s.Current(), err
	}
	if prev := s.server.SetBundle(b); prev != nil {
		_ = prev.Close()
	}
	_ = s.store.AddRecent(path)
	if s.onSwap != nil {
		s.onSwap()
	}
	return s.Current(), nil
}

// Current returns the open-bundle summary.
func (s *Service) Current() BundleInfo {
	b := s.server.Current()
	if b == nil {
		return BundleInfo{}
	}
	return BundleInfo{Loaded: true, Name: b.Name()}
}

// ServerStatus reports the LAN server state.
func (s *Service) ServerStatus() ServerStatus {
	if s.http == nil {
		return ServerStatus{}
	}
	return ServerStatus{Running: s.http.Running(), Port: s.http.Port(), URLs: s.http.URLs()}
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
