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

// configFields is the single source of truth for Config's top-level JSON keys,
// reflected once: name -> required (required == not omitempty). It drives both
// auto-sanitize checks in parseUserConfig: a missing required key and an unknown
// (dead) key both force a clean rewrite. Cached because parse runs per-profile.
var configFields = func() map[string]bool {
	t := reflect.TypeFor[Config]()
	out := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, rest, _ := strings.Cut(tag, ",")
		out[name] = !strings.Contains(rest, "omitempty")
	}
	return out
}()

// parseUserConfig decodes a profile JSON, fills missing fields with defaults,
// and reports changed=true when the on-disk file should be rewritten: a missing
// required key (fill the default), an unknown/dead key (strip it), or a clamped
// numeric. The rewrite goes through the Config struct, so dropped fields vanish.
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
	migrated := migrateFlatCollaboration(probe, &cfg)
	for name, required := range configFields {
		if _, present := probe[name]; required && !present {
			return cfg, true, nil
		}
	}
	for k := range probe {
		if _, known := configFields[k]; !known {
			return cfg, true, nil
		}
	}
	return cfg, clampChanged || migrated, nil
}

// migrateFlatCollaboration moves legacy top-level collaboration keys
// (git_branch, git_self_cloned, gigot_base_url, gigot_repo_name, gigot_token)
// into the nested git/gigot blocks, so an old profile keeps its values when the
// flat keys are stripped on the auto-sanitize rewrite. Reports whether it moved
// anything. The dropped git_root/gigot_root carry no value to migrate.
func migrateFlatCollaboration(probe map[string]any, cfg *Config) bool {
	moved := false
	str := func(k string) (string, bool) { v, ok := probe[k].(string); return v, ok }
	if v, ok := str("git_branch"); ok {
		cfg.Git.Branch = v
		moved = true
	}
	if v, ok := probe["git_self_cloned"].(bool); ok {
		cfg.Git.SelfCloned = v
		moved = true
	}
	if v, ok := str("gigot_base_url"); ok {
		cfg.Gigot.BaseURL = v
		moved = true
	}
	if v, ok := str("gigot_repo_name"); ok {
		cfg.Gigot.RepoName = v
		moved = true
	}
	if v, ok := str("gigot_token"); ok {
		cfg.Gigot.Token = v
		moved = true
	}
	return moved
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
