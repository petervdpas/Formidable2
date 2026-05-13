package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// config_io.go owns the read/write/merge cycle for the active profile
// JSON. Mirrors `Formidable/controls/configManager.js` loadUserConfig +
// saveUserConfig + updateUserConfig + invalidateConfigCache, with two
// changes from the JS:
//
//   - Cache invalidation lives in domain.go (already there).
//   - The read-modify-write cycle is serialized via Manager.updateMu so
//     concurrent UpdateUserConfig + SwitchUserProfile callers can't both
//     read the same baseline (lost-update prevention — covered by
//     TestUpdateUserConfig_NoLostUpdatesUnderConcurrency and
//     TestSwitchUserProfile_SerializedAgainstUpdate).

// configJSONKeys is the set of JSON tag names declared on Config. Used
// by parseUserConfig to detect "missing keys" → changed=true. Cached
// because reflection isn't free and this is called from list-profiles
// in a loop.
var configJSONKeys = func() []string {
	var c Config
	t := reflect.TypeOf(c)
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		keys = append(keys, tag)
	}
	return keys
}()

// parseUserConfig decodes a profile JSON, fills missing fields with
// defaults, and reports whether anything was filled in. Mirrors
// `schemas/config.schema.js` sanitize.
func parseUserConfig(raw string) (Config, bool, error) {
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return Config{}, false, err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, false, err
	}
	clampChanged := clampNumericSettings(&cfg)
	for _, k := range configJSONKeys {
		if _, ok := probe[k]; !ok {
			return cfg, true, nil
		}
	}
	return cfg, clampChanged, nil
}

// clampNumericSettings enforces backend bounds on numeric Config
// fields whose UX has a fixed range (ToastTimeout). Returns true when
// any value was coerced, so the load path can rewrite the profile and
// the next read sees the sanitised value.
func clampNumericSettings(cfg *Config) bool {
	changed := false
	if cfg.ToastTimeout < ToastTimeoutMin {
		cfg.ToastTimeout = ToastTimeoutMin
		changed = true
	} else if cfg.ToastTimeout > ToastTimeoutMax {
		cfg.ToastTimeout = ToastTimeoutMax
		changed = true
	}
	return changed
}

// writeJSON marshals v with indentation and writes it through fs.SaveFile
// (which goes via system.Manager.SaveFile — atomic temp+fsync+rename and
// journal-aware). Used by every file-producing method in this module.
func (m *Manager) writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return m.fs.SaveFile(path, string(b))
}

// LoadUserConfig returns the active profile's config, loading it from
// disk on first access (and on cache invalidation). If the on-disk
// file has missing fields it is repaired and rewritten — same shape
// the JS app had via schema.sanitize.
func (m *Manager) LoadUserConfig() (*Config, error) {
	m.mu.RLock()
	cached := m.cached
	path := m.configPath
	m.mu.RUnlock()

	if cached != nil {
		c := *cached
		return &c, nil
	}
	if path == "" {
		return nil, fmt.Errorf("LoadUserConfig: no active profile path")
	}

	raw, err := m.fs.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg, changed, err := parseUserConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if changed {
		if err := m.writeJSON(path, cfg); err != nil {
			return nil, fmt.Errorf("repair %s: %w", path, err)
		}
	}

	m.mu.Lock()
	m.cached = &cfg
	prevContext := ""
	if m.virtualStructure != nil {
		prevContext = m.virtualStructure.Context
	}
	if prevContext != "" && prevContext != m.fs.ResolvePath(cfg.ContextFolder) {
		m.virtualStructure = nil
		m.virtualStructureBuilt = time.Time{}
	}
	m.mu.Unlock()

	m.ensureContextFolder(&cfg)
	m.syncJournal(&cfg)

	out := cfg
	return &out, nil
}

// ensureContextFolder materialises the active profile's context_folder
// on disk. Best-effort: a failure is logged but does not abort the
// load/persist path — the user can still edit the path from the
// Locations panel and retry. Empty folder values are skipped.
func (m *Manager) ensureContextFolder(cfg *Config) {
	if cfg == nil || strings.TrimSpace(cfg.ContextFolder) == "" {
		return
	}
	if err := m.fs.EnsureDirectory(cfg.ContextFolder); err != nil {
		m.log.Warn("config: ensure context folder failed",
			"context_folder", cfg.ContextFolder, "err", err)
	}
}

// UpdateUserConfig merges a partial map into the active profile,
// persists, updates cache, and re-syncs the journal. Held under
// updateMu so concurrent updates don't lose each other's writes.
//
// The partial values can be primitives, JSON-tagged structs (e.g.
// WindowBounds), or map[string]any — they are normalized through a
// JSON round-trip so callers don't have to think about it.
func (m *Manager) UpdateUserConfig(partial map[string]any) (*Config, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	base, err := m.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	merged := *base
	if len(partial) > 0 {
		patch, err := json.Marshal(partial)
		if err != nil {
			return nil, fmt.Errorf("marshal partial: %w", err)
		}
		if err := json.Unmarshal(patch, &merged); err != nil {
			return nil, fmt.Errorf("merge partial: %w", err)
		}
	}

	if err := m.persistConfig(&merged); err != nil {
		return nil, err
	}
	out := merged
	return &out, nil
}

// persistConfig writes the config to disk, swaps the cache, and re-syncs
// the journal. If the context_folder changed, the VFS cache is also
// dropped so the next GetVirtualStructure rebuilds against the new tree.
//
// Caller is expected to hold updateMu (or be inside a method that does).
func (m *Manager) persistConfig(cfg *Config) error {
	m.mu.RLock()
	path := m.configPath
	prev := m.cached
	m.mu.RUnlock()
	if path == "" {
		return fmt.Errorf("persistConfig: no active profile path")
	}

	if err := m.writeJSON(path, cfg); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	contextChanged := prev != nil && prev.ContextFolder != cfg.ContextFolder
	stored := *cfg

	m.mu.Lock()
	m.cached = &stored
	if contextChanged {
		m.virtualStructure = nil
		m.virtualStructureBuilt = time.Time{}
	}
	m.mu.Unlock()

	m.ensureContextFolder(cfg)
	m.syncJournal(cfg)
	return nil
}

// syncJournal forwards the relevant config fields to the journal hook.
// No-op when no journal is wired. The journal's Configure() is the only
// thing called — initialization and baseline seeding are the composition
// root's responsibility (see types.go on JournalConfigurer).
func (m *Manager) syncJournal(cfg *Config) {
	if cfg == nil {
		return
	}
	m.mu.RLock()
	j := m.journal
	m.mu.RUnlock()
	if j == nil {
		return
	}
	if err := j.Configure(cfg.ContextFolder, cfg.RemoteBackend); err != nil {
		m.log.Warn("config: journal Configure failed", "err", err)
	}
}
