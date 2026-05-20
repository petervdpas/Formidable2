package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MessagesForLocale walks every discovered plugin and merges per-
// plugin locale files into a single flat map keyed by
// `plugin.<id>.<key>`. The auto-prefix is the namespace contract —
// plugin authors never write the prefix themselves, and collisions
// between plugins are impossible by construction.
//
// A plugin without an `i18n/` folder or without a file for `locale`
// contributes zero keys; that is the common case and not an error.
// One plugin with a malformed locale file is logged and skipped so
// it can't take down the whole locale-fetch path the frontend uses
// on every locale change.
func (m *Manager) MessagesForLocale(locale string) map[string]string {
	out := map[string]string{}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.plugins {
		msgs, err := loadPluginI18n(p.Dir, locale)
		if err != nil {
			m.log.Warn("plugin: skip bad i18n file",
				"plugin", p.Manifest.ID, "locale", locale, "err", err)
			continue
		}
		prefix := "plugin." + p.Manifest.ID + "."
		for k, v := range msgs {
			out[prefix+k] = v
		}
	}
	return out
}

// loadPluginI18n reads <pluginDir>/i18n/<locale>.json as a flat
// {key: value} map. Returns (empty, nil) when the file doesn't
// exist — a plugin with no translations is a valid plugin.
func loadPluginI18n(pluginDir, locale string) (map[string]string, error) {
	path := filepath.Join(pluginDir, "i18n", locale+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("plugin i18n: read %s: %w", path, err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("plugin i18n: parse %s: %w", path, err)
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────
// Editor surface — per-locale CRUD on <plugin>/i18n/<locale>.json.
// All writes route through Editor (system.Manager) so they're
// atomic+fsync'd; reads go through the same fs surface for symmetry
// and to keep tests on a single shim.
// ─────────────────────────────────────────────────────────────────

// GetI18nFile returns the raw key→value map stored on disk for one
// plugin/locale pair. Empty map (not error) when the file is missing
// so the editor UI can render an empty key/value table for a fresh
// locale without a special "first save" branch. Returns
// ErrPluginNotFound when the plugin folder is missing,
// ErrManifestInvalid for malformed ids.
func (m *Manager) GetI18nFile(pluginID, locale string) (map[string]string, error) {
	if !validID(pluginID) {
		return nil, fmt.Errorf("%w: bad id %q", ErrManifestInvalid, pluginID)
	}
	if m.deps.Editor == nil {
		return nil, fmt.Errorf("plugin: editor fs not configured")
	}
	pluginDir := filepath.Join(m.deps.PluginsDir, pluginID)
	if !m.deps.Editor.FileExists(pluginDir) {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	path := filepath.Join(pluginDir, "i18n", locale+".json")
	if !m.deps.Editor.FileExists(path) {
		return map[string]string{}, nil
	}
	raw, err := m.deps.Editor.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin i18n: read %s: %w", path, err)
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("plugin i18n: parse %s: %w", path, err)
	}
	return out, nil
}

// SaveI18nFile writes the canonical JSON shape (sorted keys, 2-space
// indent, trailing newline) to <plugin>/i18n/<locale>.json. Creates
// the i18n/ folder on first save. Locale is validated to keep
// path-traversal ("../escape") and absolute-path inputs out of the
// plugin tree — only short ascii ids are accepted.
func (m *Manager) SaveI18nFile(pluginID, locale string, msgs map[string]string) error {
	if !validID(pluginID) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, pluginID)
	}
	if !validLocaleID(locale) {
		return fmt.Errorf("%w: bad locale %q", ErrManifestInvalid, locale)
	}
	if m.deps.Editor == nil {
		return fmt.Errorf("plugin: editor fs not configured")
	}
	pluginDir := filepath.Join(m.deps.PluginsDir, pluginID)
	if !m.deps.Editor.FileExists(pluginDir) {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	i18nDir := filepath.Join(pluginDir, "i18n")
	if err := m.deps.Editor.EnsureDirectory(i18nDir); err != nil {
		return fmt.Errorf("plugin i18n: ensure %s: %w", i18nDir, err)
	}
	raw, err := marshalI18nFile(msgs)
	if err != nil {
		return err
	}
	path := filepath.Join(i18nDir, locale+".json")
	if err := m.deps.Editor.SaveFile(path, string(raw)); err != nil {
		return fmt.Errorf("plugin i18n: write %s: %w", path, err)
	}
	return nil
}

// DeleteI18nFile removes <plugin>/i18n/<locale>.json. Missing file
// is a silent no-op so the editor UI doesn't have to branch on
// "does this locale exist on disk yet?" before sending the delete.
func (m *Manager) DeleteI18nFile(pluginID, locale string) error {
	if !validID(pluginID) {
		return fmt.Errorf("%w: bad id %q", ErrManifestInvalid, pluginID)
	}
	if !validLocaleID(locale) {
		return fmt.Errorf("%w: bad locale %q", ErrManifestInvalid, locale)
	}
	if m.deps.Editor == nil {
		return fmt.Errorf("plugin: editor fs not configured")
	}
	pluginDir := filepath.Join(m.deps.PluginsDir, pluginID)
	if !m.deps.Editor.FileExists(pluginDir) {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	path := filepath.Join(pluginDir, "i18n", locale+".json")
	if !m.deps.Editor.FileExists(path) {
		return nil
	}
	if err := m.deps.Editor.DeleteFile(path); err != nil {
		return fmt.Errorf("plugin i18n: delete %s: %w", path, err)
	}
	return nil
}

// ListI18nLocales returns the locale ids the plugin has files for,
// sorted alphabetically. Empty slice (not error) when the plugin has
// no `i18n/` folder at all so the editor UI can show "no
// translations yet" without a stat-error branch.
func (m *Manager) ListI18nLocales(pluginID string) ([]string, error) {
	if !validID(pluginID) {
		return nil, fmt.Errorf("%w: bad id %q", ErrManifestInvalid, pluginID)
	}
	if m.deps.Editor == nil {
		return nil, fmt.Errorf("plugin: editor fs not configured")
	}
	pluginDir := filepath.Join(m.deps.PluginsDir, pluginID)
	if !m.deps.Editor.FileExists(pluginDir) {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, pluginID)
	}
	i18nDir := filepath.Join(pluginDir, "i18n")
	entries, err := m.deps.Editor.ListDir(i18nDir)
	if err != nil {
		return nil, fmt.Errorf("plugin i18n: list %s: %w", i18nDir, err)
	}
	out := make([]string, 0, len(entries))
	for _, name := range entries {
		if before, ok := strings.CutSuffix(name, ".json"); ok {
			out = append(out, before)
		}
	}
	sort.Strings(out)
	return out, nil
}

// marshalI18nFile produces the canonical on-disk shape: sorted
// flat keys, 2-space indent, trailing newline. Deterministic output
// means file diffs only show real changes, not key-iteration noise.
func marshalI18nFile(msgs map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(msgs))
	for k := range msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	// json.Marshal on a map already alpha-sorts string keys, but
	// being explicit keeps the contract obvious from the body.
	for _, k := range keys {
		ordered[k] = msgs[k]
	}
	raw, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("plugin i18n: marshal: %w", err)
	}
	return append(raw, '\n'), nil
}

// validLocaleID enforces the same ascii-id discipline as plugin ids
// so a locale string can never escape the plugin's i18n folder via
// path tricks. vue-i18n / unix locale codes ("en", "nl", "pt-BR")
// all fit; anything with separators or whitespace doesn't.
func validLocaleID(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
