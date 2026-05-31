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

// Manager holds parsed locale bundles in memory, one flat map[string]any per
// locale, behind an RWMutex. Values are map[string]any rather than string so
// future locale features (plural rules, nested namespaces) can extend the
// schema without changing this signature.
type Manager struct {
	log     *slog.Logger
	mu      sync.RWMutex
	bundles map[string]map[string]any
}

// NewManager loads every JSON file under the embedded locales/ tree. Fails
// fast if the default locale (`en`) is missing or any namespace file is
// malformed, both of which signal a build issue.
func NewManager(log *slog.Logger) (*Manager, error) {
	sub, err := fs.Sub(embedded, localesDir)
	if err != nil {
		return nil, fmt.Errorf("i18n locales fs.Sub: %w", err)
	}
	return newManagerFromFS(sub, log)
}

// newManagerFromFS is the testable entry point: the FS is rooted at the
// locales directory (<locale>/<file>.json) so tests can drive the loader with
// fstest.MapFS.
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
			continue
		}
		locale := e.Name()
		bundle, err := loadLocaleDir(localesFS, locale)
		if err != nil {
			return nil, fmt.Errorf("load locale %q: %w", locale, err)
		}
		if len(bundle) == 0 {
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

// LoadBundle returns the full key->value map for locale, erroring when the
// locale wasn't loaded so frontends can retry with DefaultLocale().
func (m *Manager) LoadBundle(locale string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.bundles[locale]
	if !ok {
		return nil, fmt.Errorf("unknown locale %q", locale)
	}
	// Detached copy so callers can't mutate the cached bundle.
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

// LocaleDescriptor pairs a locale id with its endonym (the language's own
// name for itself, e.g. "Nederlands"), read from the locale's own bundle under
// `language.endonym`.
type LocaleDescriptor struct {
	Code    string `json:"code"`
	Endonym string `json:"endonym"`
}

// ListLocales returns a descriptor per loaded locale, sorted by Code, with the
// raw Code as Endonym fallback when the bundle lacks `language.endonym`. Drives
// the Settings language picker from the backend's known locale set.
func (m *Manager) ListLocales() []LocaleDescriptor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	codes := make([]string, 0, len(m.bundles))
	for k := range m.bundles {
		codes = append(codes, k)
	}
	sort.Strings(codes)
	out := make([]LocaleDescriptor, 0, len(codes))
	for _, code := range codes {
		out = append(out, LocaleDescriptor{
			Code:    code,
			Endonym: lookupEndonym(m.bundles[code], code),
		})
	}
	return out
}

// lookupEndonym reads `language.endonym` from a loaded bundle (flat, since
// vue-i18n keys are dotted strings not nested maps), falling back to code.
func lookupEndonym(bundle map[string]any, code string) string {
	if v, ok := bundle["language.endonym"]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return code
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
