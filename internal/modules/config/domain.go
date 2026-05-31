// Package config owns the user configuration, boot profile, and the derived
// Virtual File System (VFS) view of the on-disk Formidable data layout.
// Wails-only, no HTTP handlers: raw config is too sensitive even for the
// loopback API.
//
// The package is split across several files by concern:
//
//	domain.go     Manager type, lifecycle (NewManager, Set*),
//	              cache invalidation primitives.
//	boot.go       .boot.json read/repair + active profile pointer.
//	config_io.go  LoadUserConfig / UpdateUserConfig / persist / parse,
//	              plus the JSON marshal helpers (writeJSON, syncJournal).
//	vfs.go        Virtual File System view of the context folder
//	              (templates + storage tree, per-template lookups).
//	profiles.go   Profile management: switch / list / current,
//	              import / export / delete + filename normalisation.
package config

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// fs is the filesystem surface this module needs.
type fs interface {
	ResolvePath(segments ...string) string
	JoinPath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	CopyFile(from, to string, overwrite bool) error
	ListFiles(dir string) ([]string, error)
}

const (
	configDirName      = "config"
	bootFileName       = ".boot.json"
	legacyBootFileName = "boot.json"
	defaultProfileName = "user.json"
	templatesDirName   = "templates"
	storageDirName     = "storage"
	imagesDirName      = "images"
	templateExt        = ".yaml"
	formExt            = ".meta.json"
	defaultVFSCacheTTL = 2 * time.Second
)

// Manager owns config + VFS state; all exported methods are safe for
// concurrent use.
type Manager struct {
	fs        fs
	log       *slog.Logger
	journal   JournalConfigurer
	tplLister TemplateLister

	mu                    sync.RWMutex
	configPath            string // absolute path to active profile JSON
	cached                *Config
	virtualStructure      *VirtualStructure
	virtualStructureBuilt time.Time
	ttl                   time.Duration
	nowFn                 func() time.Time

	// updateMu serializes the read-modify-write cycle of UpdateUserConfig and
	// SwitchUserProfile so concurrent callers can't both read the same baseline
	// (or switch into a stale path) and clobber each other. Held independently
	// of mu so cache reads stay non-blocking during a long update.
	updateMu sync.Mutex
}

// NewManager constructs and initializes the config manager: resolves the boot
// profile, ensures config dir + user.json exist, loads the active profile.
func NewManager(filesystem fs, log *slog.Logger) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{
		fs:    filesystem,
		log:   log,
		ttl:   defaultVFSCacheTTL,
		nowFn: time.Now,
	}
	if err := m.initialize(); err != nil {
		return nil, fmt.Errorf("config init: %w", err)
	}
	return m, nil
}

// SetJournal wires the journal hook (nil disables journal sync) and re-applies
// the current config to it. Safe to call before or after init.
func (m *Manager) SetJournal(j JournalConfigurer) {
	m.mu.Lock()
	m.journal = j
	cfg := m.cached
	m.mu.Unlock()
	if cfg != nil {
		m.syncJournal(cfg)
	}
}

// SetTTL overrides the virtual-structure cache TTL. Mainly for tests.
func (m *Manager) SetTTL(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttl = d
}

// SetNowFn injects a clock for tests.
func (m *Manager) SetNowFn(fn func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nowFn = fn
}

// initialize does not touch the journal; that happens lazily on first Load.
func (m *Manager) initialize() error {
	if err := m.fs.EnsureDirectory(configDirName); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}
	profile, err := m.resolveBootProfile()
	if err != nil {
		return err
	}
	m.setConfigPath(profile)
	// Seed the active profile file so listing/export works before the first Load.
	return m.ensureUserConfigFile()
}

// InvalidateConfigCache forgets the cached config and VFS; next access reloads.
func (m *Manager) InvalidateConfigCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cached = nil
	m.virtualStructure = nil
	m.virtualStructureBuilt = time.Time{}
}

// DirtyVirtualStructure marks the VFS stale without dropping the cached config,
// so the next GetVirtualStructure rescans. Called after FS mutations under the
// context folder.
func (m *Manager) DirtyVirtualStructure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.virtualStructureBuilt = time.Time{}
}
