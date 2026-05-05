package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync"
)

const (
	localesDir    = "locales"
	defaultLocale = "en"
)

// Manager holds parsed locale bundles in memory. Each locale lives as
// a directory of namespace JSON files (shell.json, settings.json, …);
// they are merged into a single flat map[string]any per locale at load
// time. Per-call lookup is then a single map read under an RWMutex.
//
// Bundle values are typed as map[string]any rather than map[string]string
// so future locale features (plural rules, nested namespaces, rich text)
// can extend the schema without changing this signature.
type Manager struct {
	log     *slog.Logger
	mu      sync.RWMutex
	bundles map[string]map[string]any
}

// NewManager loads every JSON file under the embedded locales/ tree.
// Fails fast if the default locale (`en`) is missing or any namespace
// file is malformed — both signal a build issue rather than a runtime
// condition.
func NewManager(log *slog.Logger) (*Manager, error) {
	sub, err := fs.Sub(embedded, localesDir)
	if err != nil {
		return nil, fmt.Errorf("i18n locales fs.Sub: %w", err)
	}
	return newManagerFromFS(sub, log)
}

// newManagerFromFS is the testable entry point — accepts an arbitrary
// fs.FS so tests can drive the loader with fstest.MapFS to cover
// happy + unhappy paths without touching the embedded tree. The FS is
// expected to be rooted at the locales directory: <locale>/<file>.json.
func newManagerFromFS(localesFS fs.FS, log *slog.Logger) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{log: log, bundles: map[string]map[string]any{}}

	entries, err := fs.ReadDir(localesFS, ".")
	if err != nil {
		return nil, fmt.Errorf("read locales root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue // stray files at the root are ignored
		}
		locale := e.Name()
		bundle, err := loadLocaleDir(localesFS, locale)
		if err != nil {
			return nil, fmt.Errorf("load locale %q: %w", locale, err)
		}
		if len(bundle) == 0 {
			// A locale dir with no namespace files contributes nothing.
			// Skipping (rather than erroring) lets the layout grow lazily.
			continue
		}
		m.bundles[locale] = bundle
		m.log.Debug("i18n loaded locale", "locale", locale, "keys", len(bundle))
	}

	if _, ok := m.bundles[defaultLocale]; !ok {
		return nil, fmt.Errorf("default locale %q missing from bundles", defaultLocale)
	}
	return m, nil
}

// loadLocaleDir parses every <locale>/*.json into one merged map. Files
// own disjoint key namespaces by convention; on any duplicate key, the
// alphabetically-later file wins (deterministic and visible in tests).
func loadLocaleDir(localesFS fs.FS, locale string) (map[string]any, error) {
	entries, err := fs.ReadDir(localesFS, locale)
	if err != nil {
		return nil, err
	}
	// Sort so merge order is deterministic across filesystems.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := map[string]any{}
	for _, name := range names {
		raw, err := fs.ReadFile(localesFS, locale+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read %s/%s: %w", locale, name, err)
		}
		ns := map[string]any{}
		if err := json.Unmarshal(raw, &ns); err != nil {
			return nil, fmt.Errorf("parse %s/%s: %w", locale, name, err)
		}
		maps.Copy(out, ns)
	}
	return out, nil
}

// LoadBundle returns the full key→value map for locale. Returns an
// error (not a nil map) when the locale wasn't loaded — frontends can
// fall back to the default by retrying with DefaultLocale().
func (m *Manager) LoadBundle(locale string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bundles[locale]
	if !ok {
		return nil, fmt.Errorf("unknown locale %q", locale)
	}
	// Copy to detach from internal state so callers can't mutate the
	// cached bundle.
	out := make(map[string]any, len(b))
	maps.Copy(out, b)
	return out, nil
}

// AvailableLocales returns the sorted list of loaded locale ids.
func (m *Manager) AvailableLocales() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.bundles))
	for k := range m.bundles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultLocale returns the canonical fallback locale id.
func (m *Manager) DefaultLocale() string { return defaultLocale }

// HasLocale reports whether locale was loaded into the manager.
func (m *Manager) HasLocale(locale string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.bundles[locale]
	return ok
}
