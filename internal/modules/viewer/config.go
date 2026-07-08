package viewer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Config holds the viewer's own preferences. It is deliberately small: the
// viewer only VIEWS exports, so nothing here edits data. It persists as JSON
// under the user config dir, separate from anything the main app owns.
type Config struct {
	Language      string   `json:"language"`       // "system" | "en" | "nl"
	Theme         string   `json:"theme"`          // "system" | "light" | "dark"
	RememberSize  bool     `json:"remember_size"`  // persist window size on close
	WindowWidth   int      `json:"window_width"`   // last size when RememberSize
	WindowHeight  int      `json:"window_height"`  //
	RecentBundles []string `json:"recent_bundles"` // most-recent-first, capped

	// Optional companion HTTP server. The webview itself serves the bundle
	// over an internal scheme (no user-facing port), but when ServeHTTP is on
	// the viewer also exposes the open bundle over real HTTP on HTTPPort so it
	// can be opened in a browser or shared on the machine.
	ServeHTTP bool `json:"serve_http"`
	HTTPPort  int  `json:"http_port"`

	// ServeAPI exposes the bundle's data as a read-only REST API for agents
	// (see datadb). Opt-in, default off, and independent of ServeHTTP: on
	// loopback whenever it is on, and additionally on the LAN only when
	// ServeHTTP is also on.
	ServeAPI bool `json:"serve_api"`

	// APIToken gates the data endpoints: a request must present it (X-API-Key
	// header, Authorization Bearer, or ?key=). The user sets it in Settings; one
	// is minted if the API is enabled while it is empty. The discovery routes
	// (docs, spec) stay open so an agent can find the API and be handed the key.
	APIToken string `json:"api_token"`
}

const (
	viewerConfigDir  = "formidable-viewer"
	viewerConfigFile = "config.json"
	maxRecentBundles = 10
	minPort          = 1024
	maxPort          = 65535
	defaultHTTPPort  = 8723
)

// DefaultConfig is the config a fresh install starts from.
func DefaultConfig() Config {
	return Config{
		Language:      "system",
		Theme:         "system",
		RememberSize:  true,
		RecentBundles: []string{},
		ServeHTTP:     false,
		HTTPPort:      defaultHTTPPort,
	}
}

// ConfigPath is the on-disk location of the viewer config
// (os.UserConfigDir()/formidable-viewer/config.json).
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, viewerConfigDir, viewerConfigFile), nil
}

// normalize clamps values into supported ranges and repairs a nil slice, so a
// hand-edited or partial file can never yield a broken config.
func (c *Config) normalize() {
	switch c.Language {
	case "system", "en", "nl":
	default:
		c.Language = "system"
	}
	switch c.Theme {
	case "system", "light", "dark":
	default:
		c.Theme = "system"
	}
	if c.RecentBundles == nil {
		c.RecentBundles = []string{}
	}
	if len(c.RecentBundles) > maxRecentBundles {
		c.RecentBundles = c.RecentBundles[:maxRecentBundles]
	}
	if c.HTTPPort < minPort || c.HTTPPort > maxPort {
		c.HTTPPort = defaultHTTPPort
	}
}

// addRecent pushes path to the front, de-duplicated, capped at maxRecentBundles.
func (c *Config) addRecent(path string) {
	if path == "" {
		return
	}
	next := make([]string, 0, maxRecentBundles)
	next = append(next, path)
	for _, p := range c.RecentBundles {
		if p == path {
			continue
		}
		next = append(next, p)
		if len(next) == maxRecentBundles {
			break
		}
	}
	c.RecentBundles = next
}

// SaverFunc writes bytes to a path atomically. Injected at the composition
// root (wired to system.Manager.SaveBytes) so this module stays free of the
// system dependency and is trivially testable.
type SaverFunc func(path string, data []byte) error

// ConfigStore loads and saves Config. Saves are serialized by a mutex so
// concurrent writers (e.g. a settings save racing a window-close size save)
// never interleave.
type ConfigStore struct {
	path  string
	saver SaverFunc
	mu    sync.Mutex
}

// NewConfigStore builds a store writing to path via saver.
func NewConfigStore(path string, saver SaverFunc) *ConfigStore {
	return &ConfigStore{path: path, saver: saver}
}

// Load reads the config, returning DefaultConfig() when the file is missing or
// unreadable (tolerant on read: a broken file must not brick the viewer).
func (s *ConfigStore) Load() Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return cfg // missing or unreadable -> defaults
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	cfg.normalize()
	return cfg
}

// Save normalizes and persists cfg atomically.
func (s *ConfigStore) Save(cfg Config) error {
	if s.saver == nil {
		return errors.New("viewer: config store has no saver")
	}
	cfg.normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saver(s.path, data)
}

// AddRecent records path as the most-recently-opened bundle and persists.
func (s *ConfigStore) AddRecent(path string) error {
	cfg := s.Load()
	cfg.addRecent(path)
	return s.Save(cfg)
}
