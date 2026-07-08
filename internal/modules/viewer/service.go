package viewer

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/datadb"
)

// BundleInfo summarizes a bundle for the UI. When Loaded is false but Name is
// set, it describes a pack that has been peeked (manifest read) but not opened,
// e.g. an encrypted one awaiting its password.
type BundleInfo struct {
	Loaded      bool   `json:"loaded"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Created     string `json:"created"`
	Encrypted   bool   `json:"encrypted"`
	HasData     bool   `json:"hasData"` // carries a queryable data image (agent API)
}

// OpenResult is the outcome of an open attempt. When the pack is encrypted and
// no (or a wrong) password was supplied, Info describes it from the cleartext
// manifest (so the UI can title the unlock prompt) and the flags say what to do;
// Path is the file to retry with the password (empty for a byte drop). On
// success Info.Loaded is true and the bundle is serving.
type OpenResult struct {
	Info          BundleInfo `json:"info"`
	NeedsPassword bool       `json:"needsPassword"`
	WrongPassword bool       `json:"wrongPassword"`
	Path          string     `json:"path"`
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

// APIStatus reflects the read-only agent API: whether it is enabled, whether a
// bundle with data is open to serve, and the base URLs to call.
type APIStatus struct {
	Enabled   bool     `json:"enabled"`
	Available bool     `json:"available"`
	URLs      []string `json:"urls"`
	Token     string   `json:"token"` // the key agents must present; shown only when enabled
}

// Service is the viewer's bound Wails surface. The Vue shell calls it to open
// bundles, read and write config, list recents, and control the LAN server.
// Wails-specific behavior (the native open dialog, retitle + reload after a
// swap) is injected from the composition root so this stays testable.
type Service struct {
	store   *ConfigStore
	server  *Server
	http    *HTTPServer
	frame   *HTTPServer            // always-on loopback server the iframe loads from
	open    func() (string, error) // native open-file dialog; "" == cancelled
	onSwap  func()                 // called after a bundle swaps
	pending string                 // argv "open with" path, claimed once by the UI
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

// SetPendingOpen records an argv / "open with" path to be opened once the UI is
// ready. The shell claims it on mount (TakePendingOpen) so the open, and any
// password prompt for an encrypted pack, flows through the normal UI path.
func (s *Service) SetPendingOpen(path string) { s.pending = path }

// TakePendingOpen returns the pending argv path and clears it, so it opens only
// once. Empty when there is none.
func (s *Service) TakePendingOpen() string {
	p := s.pending
	s.pending = ""
	return p
}

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

// SetConfig persists cfg and applies it (starting or stopping the LAN server,
// minting the API token on first enable), returning the applied config.
func (s *Service) SetConfig(cfg Config) (Config, error) {
	if err := s.store.Save(cfg); err != nil {
		return s.store.Load(), err
	}
	return s.applyServer(s.store.Load())
}

// Apply reflects the persisted config at startup (e.g. auto-starts the LAN
// server when enabled, restores the API token).
func (s *Service) Apply() error {
	_, err := s.applyServer(s.store.Load())
	return err
}

// applyServer reflects cfg onto the running servers and returns the config that
// was actually applied. Enabling the API mints a token on first use and
// persists it, so the returned config carries the live token.
func (s *Service) applyServer(cfg Config) (Config, error) {
	if cfg.ServeAPI && cfg.APIToken == "" {
		cfg.APIToken = generateToken()
		if err := s.store.Save(cfg); err != nil {
			return cfg, err
		}
	}
	s.server.SetAPIToken(cfg.APIToken)
	s.server.SetAPIEnabled(cfg.ServeAPI) // gates /api/ on both loopback and LAN

	if s.http == nil {
		return cfg, nil
	}
	if cfg.ServeHTTP {
		return cfg, s.http.Start(cfg.HTTPPort)
	}
	return cfg, s.http.Stop()
}

// RegenerateAPIToken mints a fresh API token (invalidating the old one) and
// returns the updated API status. Enables nothing on its own; the API stays off
// unless ServeAPI is on.
func (s *Service) RegenerateAPIToken() (APIStatus, error) {
	cfg := s.store.Load()
	cfg.APIToken = generateToken()
	if err := s.store.Save(cfg); err != nil {
		return s.APIStatus(), err
	}
	if _, err := s.applyServer(cfg); err != nil {
		return s.APIStatus(), err
	}
	return s.APIStatus(), nil
}

// generateToken returns a random 32-hex-char (16-byte) API token.
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
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

// OpenDialog shows the native file picker and opens the chosen pack with no
// password. If the pack is encrypted the result says so (NeedsPassword) and
// carries its Path, and the UI re-calls OpenPath with the password. A cancelled
// dialog leaves the current bundle unchanged.
func (s *Service) OpenDialog() (OpenResult, error) {
	if s.open == nil {
		return OpenResult{Info: s.Current()}, nil
	}
	path, err := s.open()
	if err != nil || path == "" {
		return OpenResult{Info: s.Current()}, err
	}
	return s.OpenPath(path, "")
}

// OpenPath opens the pack at path with the given password (empty for an
// unencrypted pack). On success it swaps the bundle in, records the path as
// recent, and fires the swap hook. When the pack is encrypted and the password
// is missing or wrong, nothing is loaded and the result flags say so. The
// password is used only to decrypt; it is never stored (recents hold the path
// only, so reopening re-prompts).
func (s *Service) OpenPath(path string, password string) (OpenResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return OpenResult{}, err
	}
	return s.openRaw(raw, filepath.Base(path), path, password)
}

// OpenBytes opens a pack from raw bytes (base64-encoded), for the shell's HTML5
// file drop, where the webview exposes file contents but not a path. No recents
// entry is recorded because there is no path to reopen later; on a wrong or
// missing password the UI retries by re-submitting the same bytes.
func (s *Service) OpenBytes(name string, dataB64 string, password string) (OpenResult, error) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return OpenResult{}, err
	}
	return s.openRaw(data, name, "", password)
}

// openRaw is the shared open path: decode raw file bytes into a servable zip and
// its manifest (transparently handling encrypted .bundle, plain .bundle, and a
// legacy bare .zip), then swap the bundle in. path is "" for byte drops.
func (s *Service) openRaw(raw []byte, name, path, password string) (OpenResult, error) {
	zipBytes, man, needPw, wrongPw, err := decodeBundle(raw, password)
	if err != nil {
		return OpenResult{}, err
	}
	if needPw || wrongPw {
		info := infoFromManifest(name, man)
		return OpenResult{Info: info, NeedsPassword: needPw, WrongPassword: wrongPw, Path: path}, nil
	}

	b, err := BundleFromBytes(zipBytes, name)
	if err != nil {
		return OpenResult{}, err
	}
	b.manifest = man
	if prev := s.server.SetBundle(b); prev != nil {
		_ = prev.Close()
	}
	if path != "" {
		_ = s.store.AddRecent(path)
	}
	if s.onSwap != nil {
		s.onSwap()
	}
	return OpenResult{Info: s.Current()}, nil
}

// decodeBundle resolves raw file bytes to the servable zip plus its manifest.
// The Viewer opens Formidable bundles only: a file without the bundle container
// (ErrNotBundle) is rejected, not served as a bare zip. An encrypted bundle with
// a missing password sets needPw; a wrong password sets wrongPw. Neither is a
// hard error, so the UI can prompt.
func decodeBundle(raw []byte, password string) (zipBytes []byte, man bundle.Manifest, needPw, wrongPw bool, err error) {
	man, mErr := bundle.ReadManifest(raw)
	if mErr != nil {
		return nil, bundle.Manifest{}, false, false, mErr
	}
	if man.Encrypted && password == "" {
		return nil, man, true, false, nil
	}
	zipBytes, uErr := bundle.Unpack(raw, password)
	switch {
	case errors.Is(uErr, bundle.ErrEmptyPassword):
		return nil, man, true, false, nil
	case errors.Is(uErr, bundle.ErrDecrypt):
		return nil, man, false, true, nil
	case uErr != nil:
		return nil, man, false, false, uErr
	}
	return zipBytes, man, false, false, nil
}

func infoFromManifest(name string, man bundle.Manifest) BundleInfo {
	return BundleInfo{
		Name:        name,
		Title:       man.Title,
		Description: man.Description,
		Author:      man.Author,
		Created:     man.Created,
		Encrypted:   man.Encrypted,
	}
}

// Graph returns the open bundle's record-relations graph, read straight from
// the bundle (no agent API or token needed: this is the recipient viewing their
// own open bundle). Empty when nothing with data is open.
func (s *Service) Graph() (datadb.Graph, error) {
	b := s.server.Current()
	if b == nil || !b.HasData() {
		return datadb.Graph{Nodes: []datadb.GraphNode{}, Edges: []datadb.GraphEdge{}}, nil
	}
	return b.Graph()
}

// GraphRecord returns one record's detail for the graph's side panel. An unknown
// guid (or no open data) yields an empty record, not an error.
func (s *Service) GraphRecord(guid string) (datadb.RecordFull, error) {
	b := s.server.Current()
	if b == nil || !b.HasData() {
		return datadb.RecordFull{}, nil
	}
	r, _, err := b.Record(guid)
	return r, err
}

// Current returns the open-bundle summary.
func (s *Service) Current() BundleInfo {
	b := s.server.Current()
	if b == nil {
		return BundleInfo{}
	}
	info := infoFromManifest(b.Name(), b.manifest)
	info.Loaded = true
	info.HasData = b.HasData()
	return info
}

// APIStatus reports the agent API state for the UI: whether it is enabled, is
// serving (a bundle with data is open), and the base URLs agents can call. URLs
// are only populated when it is actually serving.
func (s *Service) APIStatus() APIStatus {
	st := APIStatus{Enabled: s.server.APIEnabled()}
	if b := s.server.Current(); b != nil {
		st.Available = b.HasData()
	}
	if st.Enabled {
		st.Token = s.store.Load().APIToken
	}
	if st.Enabled && st.Available {
		st.URLs = append(st.URLs, s.BundleURL()+"api/") // loopback frame server
		if s.http != nil && s.http.Running() {
			for _, u := range s.http.URLs() {
				st.URLs = append(st.URLs, u+"/api/")
			}
		}
	}
	return st
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
