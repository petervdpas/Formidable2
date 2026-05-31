package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// config_io.go owns the read/write/merge cycle for the active profile
// JSON. The read-modify-write cycle is serialized via Manager.updateMu so
// concurrent Update + SwitchProfile callers can't lose each other's writes.

// configJSONKeys are the required JSON tag names on Config, used to detect
// missing keys (changed=true). omitempty fields are skipped: absent is
// valid for them, so flagging them would force spurious rewrites. Cached
// because this runs per-profile in a loop.
var configJSONKeys = func() []string {
	var c Config
	t := reflect.TypeOf(c)
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, rest, _ := strings.Cut(tag, ",")
		if strings.Contains(rest, "omitempty") {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}()

// parseUserConfig decodes a profile JSON, fills missing fields with
// defaults, and reports whether anything was filled in or clamped.
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

// clampNumericSettings coerces range-bound numeric fields (ToastTimeout,
// DecimalPrecision) into bounds, returning true if any value changed.
func clampNumericSettings(cfg *Config) bool {
	changed := false
	if cfg.ToastTimeout < ToastTimeoutMin {
		cfg.ToastTimeout = ToastTimeoutMin
		changed = true
	} else if cfg.ToastTimeout > ToastTimeoutMax {
		cfg.ToastTimeout = ToastTimeoutMax
		changed = true
	}
	if cfg.DecimalPrecision < DecimalPrecisionMin {
		cfg.DecimalPrecision = DecimalPrecisionMin
		changed = true
	} else if cfg.DecimalPrecision > DecimalPrecisionMax {
		cfg.DecimalPrecision = DecimalPrecisionMax
		changed = true
	}
	return changed
}

// writeJSON marshals v indented and writes it through fs.SaveFile (atomic
// temp+fsync+rename, journal-aware).
func (m *Manager) writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return m.fs.SaveFile(path, string(b))
}

// LoadUserConfig returns the active profile's config, loading from disk on
// first access (and after cache invalidation). A file with missing fields
// is repaired and rewritten.
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

// ensureContextFolder materialises the profile's context_folder on disk.
// Best-effort: failures are logged, not fatal; empty values are skipped.
func (m *Manager) ensureContextFolder(cfg *Config) {
	if cfg == nil || strings.TrimSpace(cfg.ContextFolder) == "" {
		return
	}
	if err := m.fs.EnsureDirectory(cfg.ContextFolder); err != nil {
		m.log.Warn("config: ensure context folder failed",
			"context_folder", cfg.ContextFolder, "err", err)
	}
}

// UpdateUserConfig merges a partial map into the active profile, persists,
// and re-syncs the journal, held under updateMu against lost updates.
// Partial values normalize through a JSON round-trip, so primitives,
// JSON-tagged structs, and map[string]any all work.
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

	// Auto-clear SelectedTemplate when it falls outside EnabledTemplates;
	// runs every Update since either key could break the invariant.
	normalizeSelectedTemplate(&merged)

	if err := m.persistConfig(&merged); err != nil {
		return nil, err
	}
	out := merged
	return &out, nil
}

// mutateUserConfig applies mutate to the live config under updateMu, so the
// whole read-modify-write is atomic against concurrent updates (callers that
// load, compute a slice, then UpdateUserConfig lose updates under contention).
// mutate reports whether it changed anything; a no-op skips the write.
func (m *Manager) mutateUserConfig(mutate func(*Config) bool) (*Config, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	base, err := m.LoadUserConfig()
	if err != nil {
		return nil, err
	}
	merged := *base
	if !mutate(&merged) {
		out := *base
		return &out, nil
	}
	normalizeSelectedTemplate(&merged)
	if err := m.persistConfig(&merged); err != nil {
		return nil, err
	}
	out := merged
	return &out, nil
}

// persistConfig writes the config, swaps the cache, and re-syncs the
// journal; a changed context_folder also drops the VFS cache. Caller holds
// updateMu.
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

// syncJournal forwards config fields to the journal hook via Configure;
// no-op when no journal is wired. Init/seeding is the composition root's
// job (see JournalConfigurer in types.go).
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
