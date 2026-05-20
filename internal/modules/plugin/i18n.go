package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
