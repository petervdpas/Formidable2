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

// MessagesForLocale merges every plugin's locale file into one flat map keyed `plugin.<id>.<key>`.
// The auto-prefix is the namespace contract (authors never write it; cross-plugin collisions are impossible).
// A malformed locale file is logged and skipped so it can't take down the whole locale-fetch path.
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

// loadPluginI18n reads <pluginDir>/i18n/<locale>.json as a flat map, returning (empty, nil) when the file is absent.
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

// Editor surface: per-locale CRUD on <plugin>/i18n/<locale>.json, writes routed through Editor for atomic+fsync.

// GetI18nFile returns the on-disk key map for one plugin/locale pair, empty map when the file is missing.
// Errors: ErrPluginNotFound, ErrManifestInvalid.
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

// SaveI18nFile writes the canonical JSON (sorted keys, 2-space indent, trailing newline) to <plugin>/i18n/<locale>.json.
// Locale is validated to keep path-traversal and absolute-path inputs out of the plugin tree.
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

// DeleteI18nFile removes <plugin>/i18n/<locale>.json; a missing file is a silent no-op.
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

// ListI18nLocales returns the plugin's locale ids sorted, empty slice when it has no i18n/ folder.
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

// marshalI18nFile produces the canonical on-disk shape (sorted keys, 2-space indent, trailing newline) so diffs show only real changes.
func marshalI18nFile(msgs map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(msgs))
	for k := range msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(keys))
	for _, k := range keys {
		ordered[k] = msgs[k]
	}
	raw, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("plugin i18n: marshal: %w", err)
	}
	return append(raw, '\n'), nil
}

// validLocaleID enforces ascii-id discipline so a locale can't escape the i18n folder via path tricks; codes like "en", "pt-BR" fit.
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
