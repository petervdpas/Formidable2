package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

// ── Happy paths (against the embedded tree) ─────────────────────────

func TestNewManager_LoadsEmbeddedLocales(t *testing.T) {
	t.Parallel()
	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !m.HasLocale("en") {
		t.Errorf("en locale missing")
	}
	if !m.HasLocale("nl") {
		t.Errorf("nl locale missing")
	}
}

func TestLoadBundle_ReturnsKnownKey(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	b, err := m.LoadBundle("en")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	v, ok := b["status.ready"]
	if !ok {
		t.Fatalf("missing status.ready in en bundle")
	}
	if s, _ := v.(string); s == "" {
		t.Errorf("status.ready empty")
	}
}

func TestLoadBundle_MergesAcrossNamespaceFiles(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	b, _ := m.LoadBundle("en")
	// One key from each namespace file. If merge fails, at least one
	// is missing.
	for _, k := range []string{
		"ribbon.templates",      // shell.json
		"settings.title",        // settings.json
		"menu.file.openTemplateFolder", // menus.json
		"status.ready",          // status.json
		"paste.process",         // modals.json
		"error.template.invalid", // errors.json
	} {
		if _, ok := b[k]; !ok {
			t.Errorf("merged en bundle missing key %q", k)
		}
	}
}

func TestLoadBundle_DutchKeyDiffersFromEnglish(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	en, _ := m.LoadBundle("en")
	nl, _ := m.LoadBundle("nl")
	differs := false
	for k, vEn := range en {
		if vNl, ok := nl[k]; ok && vNl != vEn {
			differs = true
			break
		}
	}
	if !differs {
		t.Fatalf("en and nl bundles look identical - likely an upstream bug")
	}
}

func TestAvailableLocales_SortedAndContains(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	locs := m.AvailableLocales()
	if len(locs) < 2 {
		t.Fatalf("expected at least 2 locales, got %v", locs)
	}
	for i := 1; i < len(locs); i++ {
		if locs[i] < locs[i-1] {
			t.Fatalf("not sorted: %v", locs)
		}
	}
	has := map[string]bool{}
	for _, l := range locs {
		has[l] = true
	}
	if !has["en"] || !has["nl"] {
		t.Errorf("missing en/nl in %v", locs)
	}
}

func TestDefaultLocale_IsEn(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	if got := m.DefaultLocale(); got != "en" {
		t.Errorf("DefaultLocale = %q, want en", got)
	}
}

func TestLoadBundle_ReturnsCopy(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	a, _ := m.LoadBundle("en")
	a["status.ready"] = "MUTATED"
	b, _ := m.LoadBundle("en")
	if got, _ := b["status.ready"].(string); got == "MUTATED" {
		t.Errorf("LoadBundle returned shared (mutable) reference")
	}
}

// ── Unhappy paths (against synthetic FS) ─────────────────────────────

func TestLoadBundle_UnknownLocaleErrors(t *testing.T) {
	t.Parallel()
	m, _ := NewManager(nil)
	_, err := m.LoadBundle("klingon")
	if err == nil {
		t.Fatalf("expected error for unknown locale")
	}
}

func TestNewManagerFromFS_MalformedJSONErrors(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"en/shell.json": &fstest.MapFile{Data: []byte("{not json")},
	}
	_, err := newManagerFromFS(fsys, nil)
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
}

func TestNewManagerFromFS_MissingDefaultLocaleErrors(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"fr/shell.json": &fstest.MapFile{Data: []byte(`{"hello":"bonjour"}`)},
	}
	_, err := newManagerFromFS(fsys, nil)
	if err == nil {
		t.Fatalf("expected error when default locale missing")
	}
	if !strings.Contains(err.Error(), "default locale") {
		t.Errorf("error %q does not mention 'default locale'", err)
	}
}

func TestNewManagerFromFS_IgnoresStrayFilesAtRoot(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"README.md":     &fstest.MapFile{Data: []byte("not a locale dir")},
		"en/shell.json": &fstest.MapFile{Data: []byte(`{"k":"v"}`)},
	}
	m, err := newManagerFromFS(fsys, nil)
	if err != nil {
		t.Fatalf("newManagerFromFS: %v", err)
	}
	if locs := m.AvailableLocales(); len(locs) != 1 || locs[0] != "en" {
		t.Errorf("locales = %v, want [en]", locs)
	}
}

func TestNewManagerFromFS_IgnoresNonJSONInLocaleDir(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"en/shell.json": &fstest.MapFile{Data: []byte(`{"k":"v"}`)},
		"en/notes.txt":  &fstest.MapFile{Data: []byte("ignored")},
	}
	m, err := newManagerFromFS(fsys, nil)
	if err != nil {
		t.Fatalf("newManagerFromFS: %v", err)
	}
	b, err := m.LoadBundle("en")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if got, _ := b["k"].(string); got != "v" {
		t.Errorf("bundle = %v, want {k: v}", b)
	}
}

func TestNewManagerFromFS_MergesNamespaceFiles(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"en/shell.json":    &fstest.MapFile{Data: []byte(`{"a":"A"}`)},
		"en/settings.json": &fstest.MapFile{Data: []byte(`{"b":"B"}`)},
	}
	m, err := newManagerFromFS(fsys, nil)
	if err != nil {
		t.Fatalf("newManagerFromFS: %v", err)
	}
	b, _ := m.LoadBundle("en")
	if got, _ := b["a"].(string); got != "A" {
		t.Errorf("missing a: %v", b)
	}
	if got, _ := b["b"].(string); got != "B" {
		t.Errorf("missing b: %v", b)
	}
}

func TestNewManagerFromFS_DuplicateKeyAcrossNamespacesAlphaLastWins(t *testing.T) {
	t.Parallel()
	// Files merge in sorted-name order, so the alphabetically later
	// file's value wins on collision. "shell" sorts after "settings".
	fsys := fstest.MapFS{
		"en/settings.json": &fstest.MapFile{Data: []byte(`{"k":"FROM_SETTINGS"}`)},
		"en/shell.json":    &fstest.MapFile{Data: []byte(`{"k":"FROM_SHELL"}`)},
	}
	m, _ := newManagerFromFS(fsys, nil)
	b, _ := m.LoadBundle("en")
	if got, _ := b["k"].(string); got != "FROM_SHELL" {
		t.Errorf("dup key = %q, want FROM_SHELL (alpha-last wins)", got)
	}
}

func TestNewManagerFromFS_EmptyDirErrors(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{}
	_, err := newManagerFromFS(fsys, nil)
	if err == nil {
		t.Fatalf("expected error for empty locale dir")
	}
	if !strings.Contains(err.Error(), "default locale") {
		t.Errorf("error %q does not mention 'default locale'", err)
	}
}

func TestNewManagerFromFS_LocaleDirWithNoJSONIsSkipped(t *testing.T) {
	t.Parallel()
	// "fr" exists but has no JSON; default "en" is present.
	fsys := fstest.MapFS{
		"en/shell.json": &fstest.MapFile{Data: []byte(`{"k":"v"}`)},
		"fr/notes.txt":  &fstest.MapFile{Data: []byte("nothing here")},
	}
	m, err := newManagerFromFS(fsys, nil)
	if err != nil {
		t.Fatalf("newManagerFromFS: %v", err)
	}
	if m.HasLocale("fr") {
		t.Errorf("fr should be skipped (no JSON)")
	}
	if !m.HasLocale("en") {
		t.Errorf("en should be loaded")
	}
}
