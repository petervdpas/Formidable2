package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseUserConfig_NonObjectJSONRejected(t *testing.T) {
	cases := []string{`[1,2,3]`, `"a string"`, `42`, `true`}
	for _, raw := range cases {
		_, _, err := parseUserConfig(raw)
		if err == nil {
			t.Errorf("parseUserConfig(%q): expected error for non-object JSON, got nil", raw)
		}
	}
}

// TestParseUserConfig_JSONNullYieldsDefaults documents the tolerant edge:
// the literal null unmarshals into a nil map without error, so the result
// is a fully-defaulted config flagged changed (every key reads as missing).
func TestParseUserConfig_JSONNullYieldsDefaults(t *testing.T) {
	cfg, changed, err := parseUserConfig(`null`)
	if err != nil {
		t.Fatalf("parseUserConfig(null): %v", err)
	}
	if !changed {
		t.Error("null input must flag changed=true so the profile is rewritten with defaults")
	}
	if cfg.Theme != "light" || cfg.InternalServerPort != 8383 {
		t.Errorf("null must yield defaults, got theme=%q port=%d", cfg.Theme, cfg.InternalServerPort)
	}
}

func TestParseUserConfig_TruncatedJSONRejected(t *testing.T) {
	_, _, err := parseUserConfig(`{"theme":"dark"`)
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Errorf("err = %v, want unexpected-end-of-JSON", err)
	}
}

func TestParseUserConfig_EmptyStringRejected(t *testing.T) {
	_, _, err := parseUserConfig("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Errorf("err = %v, want unexpected-end-of-JSON", err)
	}
}

// TestLoadUserConfig_MalformedDiskSurfacesParseError mutates the active
// profile to garbage behind the cache then forces a reload. The error must
// name the path and the JSON failure so the caller can act, not silently
// fall back to defaults.
func TestLoadUserConfig_MalformedDiskSurfacesParseError(t *testing.T) {
	m, _, root := newTestManager(t)
	if _, err := m.LoadUserConfig(); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	path := filepath.Join(root, "config", "user.json")
	if err := os.WriteFile(path, []byte("{truncated"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}
	m.InvalidateConfigCache()

	_, err := m.LoadUserConfig()
	if err == nil {
		t.Fatal("expected error loading malformed profile")
	}
	if !strings.Contains(err.Error(), "parse") || !strings.Contains(err.Error(), "user.json") {
		t.Errorf("err = %v, want a parse error naming user.json", err)
	}
}

// TestLoadUserConfig_NonObjectDiskSurfacesError covers an on-disk profile
// that is valid JSON but the wrong shape (an array). The probe-decode into
// map[string]any must reject it rather than feeding defaults.
func TestLoadUserConfig_NonObjectDiskSurfacesError(t *testing.T) {
	m, _, root := newTestManager(t)
	if _, err := m.LoadUserConfig(); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	path := filepath.Join(root, "config", "user.json")
	if err := os.WriteFile(path, []byte(`["not","an","object"]`), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}
	m.InvalidateConfigCache()

	if _, err := m.LoadUserConfig(); err == nil {
		t.Fatal("expected error for array-shaped profile on disk")
	}
}

// TestUpdateUserConfig_EnabledTemplatesWrongTypeRejected feeds a scalar
// where the schema wants []string. The merge must fail with a typed JSON
// error and leave the persisted slice untouched (still nil/all-enabled).
func TestUpdateUserConfig_EnabledTemplatesStringRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.UpdateUserConfig(map[string]any{"enabled_templates": "basic.yaml"})
	if err == nil {
		t.Fatal("expected error assigning a string to enabled_templates")
	}
	if !strings.Contains(err.Error(), "merge partial") {
		t.Errorf("err = %v, want merge-partial wrap", err)
	}
	cfg, lerr := m.LoadUserConfig()
	if lerr != nil {
		t.Fatalf("LoadUserConfig: %v", lerr)
	}
	if cfg.EnabledTemplates != nil {
		t.Errorf("failed update must not mutate slice, got %v", cfg.EnabledTemplates)
	}
}

func TestUpdateUserConfig_EnabledTemplatesNumberElementsRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.UpdateUserConfig(map[string]any{"enabled_templates": []int{1, 2}})
	if err == nil {
		t.Fatal("expected error: numeric elements are not valid template filenames")
	}
	if !strings.Contains(err.Error(), "merge partial") {
		t.Errorf("err = %v, want merge-partial wrap", err)
	}
}

func TestUpdateUserConfig_EnabledTemplatesNullClearsToNil(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "a.yaml", "b.yaml")
	if _, err := m.UpdateUserConfig(map[string]any{"enabled_templates": nil}); err != nil {
		t.Fatalf("UpdateUserConfig(nil): %v", err)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.EnabledTemplates != nil {
		t.Errorf("explicit null must clear back to nil (all enabled), got %v", cfg.EnabledTemplates)
	}
}

// TestUpdateUserConfig_EnabledTemplatesEmptyStringElementRoundTrips covers the
// tolerant-on-read edge: an empty-string element is a valid JSON string, so the
// merge accepts it and it persists verbatim. The lookup helpers still treat the
// empty name as not-enabled, so no real template gets accidentally scoped in.
func TestUpdateUserConfig_EnabledTemplatesEmptyStringElementRoundTrips(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"a.yaml", ""},
	}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != 2 || cfg.EnabledTemplates[0] != "a.yaml" || cfg.EnabledTemplates[1] != "" {
		t.Errorf("empty-string element must round-trip verbatim, got %v", cfg.EnabledTemplates)
	}
	if m.IsTemplateEnabled("") {
		t.Error("empty name must never read as enabled even when present in the slice")
	}
	if !m.IsTemplateEnabled("a.yaml") {
		t.Error("the real entry a.yaml must still read as enabled")
	}
}

// TestUpdateUserConfig_EnabledTemplatesObjectElementsRejected feeds objects
// where the schema wants strings. The merge must fail with the typed JSON wrap
// and not mutate the persisted slice.
func TestUpdateUserConfig_EnabledTemplatesObjectElementsRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []map[string]any{{"name": "x.yaml"}},
	})
	if err == nil {
		t.Fatal("expected error: object elements are not valid template filenames")
	}
	if !strings.Contains(err.Error(), "merge partial") {
		t.Errorf("err = %v, want merge-partial wrap", err)
	}
	cfg, lerr := m.LoadUserConfig()
	if lerr != nil {
		t.Fatalf("LoadUserConfig: %v", lerr)
	}
	if cfg.EnabledTemplates != nil {
		t.Errorf("failed update must not mutate slice, got %v", cfg.EnabledTemplates)
	}
}

// TestUpdateUserConfig_ConcurrentEnabledTemplatesNoTornState fires N
// writers that each set a distinct single-element enabled_templates slice.
// Under updateMu the final on-disk slice must be exactly one of the inputs
// (length 1), never a torn merge of two writers.
func TestUpdateUserConfig_ConcurrentEnabledTemplatesNoTornState(t *testing.T) {
	t.Parallel()
	m, _, root := newTestManager(t)

	const N = 24
	names := make([]string, N)
	for i := range names {
		names[i] = "tpl" + string(rune('a'+i%26)) + ".yaml"
	}

	var wg sync.WaitGroup
	for i := range N {
		wg.Go(func() {
			if _, err := m.UpdateUserConfig(map[string]any{
				"enabled_templates": []string{names[i]},
			}); err != nil {
				t.Errorf("UpdateUserConfig: %v", err)
			}
		})
	}
	wg.Wait()

	raw, err := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if err != nil {
		t.Fatalf("read user.json: %v", err)
	}
	var disk Config
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("parse user.json: %v", err)
	}
	if len(disk.EnabledTemplates) != 1 {
		t.Fatalf("torn state: enabled_templates len = %d, want 1 (%v)", len(disk.EnabledTemplates), disk.EnabledTemplates)
	}
	winner := disk.EnabledTemplates[0]
	found := false
	for _, n := range names {
		if n == winner {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("winning value %q is not one of the writer inputs", winner)
	}

	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != 1 || cfg.EnabledTemplates[0] != winner {
		t.Errorf("cache diverges from disk: cache=%v disk=%q", cfg.EnabledTemplates, winner)
	}
}

// TestConcurrent_LoadInvalidateAndVFSNoDataRace hammers the read path
// (LoadUserConfig + GetVirtualStructure) against cache invalidation under -race.
// The point is the race detector: no torn read of m.cached / m.virtualStructure.
// We also assert every successful load returns the known seeded theme so a
// nil-deref or partial swap can't pass as "ran".
func TestConcurrent_LoadInvalidateAndVFSNoDataRace(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestManagerRootContext(t)
	if _, err := m.UpdateUserConfig(map[string]any{"theme": "racecheck"}); err != nil {
		t.Fatalf("seed theme: %v", err)
	}

	const N = 32
	var wg sync.WaitGroup
	for range N {
		wg.Go(func() {
			cfg, err := m.LoadUserConfig()
			if err != nil {
				t.Errorf("LoadUserConfig: %v", err)
				return
			}
			if cfg.Theme != "racecheck" {
				t.Errorf("theme = %q, want racecheck", cfg.Theme)
			}
		})
		wg.Go(func() { m.InvalidateConfigCache() })
		wg.Go(func() {
			if _, err := m.GetVirtualStructure(); err != nil {
				t.Errorf("GetVirtualStructure: %v", err)
			}
		})
	}
	wg.Wait()

	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("final LoadUserConfig: %v", err)
	}
	if cfg.Theme != "racecheck" {
		t.Errorf("final theme = %q, want racecheck (invalidation must not lose persisted state)", cfg.Theme)
	}
}
