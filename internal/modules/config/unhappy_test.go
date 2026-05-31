package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// unhappy_test.go covers malformed input, missing/odd lookups, invalid
// enabled_templates values, and concurrency for the config module.

// ----- Malformed / truncated config on read --------------------------

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

// TestProfileDisplayName_MalformedFileShowsUnknown verifies a corrupt
// sibling profile does not break the picker: it surfaces as "(unknown)"
// and the good profile still lists with its real name.
func TestProfileDisplayName_MalformedFileShowsUnknown(t *testing.T) {
	m, sys, _ := newTestManager(t)
	if err := sys.SaveFile("config/broken.json", "{not valid json"); err != nil {
		t.Fatalf("seed broken: %v", err)
	}

	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		t.Fatalf("ListAvailableProfiles: %v", err)
	}
	got := map[string]string{}
	for _, p := range profiles {
		got[p.Value] = p.Display
	}
	if got["broken.json"] != "(unknown)" {
		t.Errorf("broken.json display = %q, want (unknown)", got["broken.json"])
	}
	if got["user.json"] != "Default Profile" {
		t.Errorf("user.json display = %q, want Default Profile", got["user.json"])
	}
}

// ----- Missing profile name / invalid names --------------------------

func TestSwitchUserProfile_EmptyNameRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SwitchUserProfile("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
	if !strings.Contains(err.Error(), "invalid profile filename") {
		t.Errorf("err = %v, want invalid-filename message", err)
	}
	if got := m.CurrentProfileFilename(); got != defaultProfileName {
		t.Errorf("active profile changed to %q after failed switch, want %q", got, defaultProfileName)
	}
}

func TestSwitchUserProfile_DotPrefixRejectedBeforeFormatCheck(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SwitchUserProfile(".boot.json")
	if err == nil {
		t.Fatal("expected error for dot-prefixed profile")
	}
	if !strings.Contains(err.Error(), "cannot start with '.'") {
		t.Errorf("err = %v, want dot-prefix rejection", err)
	}
}

func TestDeleteUserProfile_EmptyNameMissingFilenameCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.DeleteUserProfile("")
	if r.Success {
		t.Fatal("empty delete must fail")
	}
	if r.Code != "missing_filename" {
		t.Errorf("code = %q, want missing_filename", r.Code)
	}
}

func TestDeleteUserProfile_MissingFileNotFoundCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.DeleteUserProfile("ghost.json")
	if r.Success {
		t.Fatal("deleting a non-existent profile must fail")
	}
	if r.Code != "not_found" {
		t.Errorf("code = %q, want not_found", r.Code)
	}
}

func TestExportUserProfile_MissingArgsNoCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.ExportUserProfile("", "/tmp/whatever.json", true)
	if r.Success {
		t.Fatal("empty profileFilename export must fail")
	}
	if r.Code != "" {
		t.Errorf("code = %q, want empty (validation case sets only Error)", r.Code)
	}
	if r.Error != "Missing profileFilename or targetPath." {
		t.Errorf("Error = %q", r.Error)
	}

	r = m.ExportUserProfile("user.json", "", true)
	if r.Success || r.Error != "Missing profileFilename or targetPath." {
		t.Errorf("empty targetPath: got %+v", r)
	}
}

func TestImportUserProfile_EmptySourceRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.ImportUserProfile("", "dest.json", false)
	if r.Success {
		t.Fatal("empty sourcePath import must fail")
	}
	if r.Error != "Missing sourcePath." {
		t.Errorf("Error = %q, want Missing sourcePath.", r.Error)
	}
}

// TestImportUserProfile_MalformedSourceUndoesCopy exercises the rollback:
// a copied-but-unparseable source must be deleted and reported as
// invalid_config, leaving no orphan in config/.
func TestImportUserProfile_MalformedSourceUndoesCopy(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "bad-source.json")
	if err := os.WriteFile(src, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("seed bad source: %v", err)
	}

	r := m.ImportUserProfile(src, "imp.json", false)
	if r.Success {
		t.Fatal("malformed source import must fail")
	}
	if r.Code != "invalid_config" {
		t.Errorf("code = %q, want invalid_config", r.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "imp.json")); !os.IsNotExist(err) {
		t.Errorf("orphan target must be removed, stat err = %v", err)
	}
}

// ----- Invalid enabled_templates values ------------------------------

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

// TestIsTemplateEnabled_MalformedConfigReturnsFalse asserts the tolerant
// read path: a load failure means "nothing is enabled" rather than a panic
// or stale truth.
func TestIsTemplateEnabled_MalformedConfigReturnsFalse(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "a.yaml")
	if !m.IsTemplateEnabled("a.yaml") {
		t.Fatal("a.yaml should be enabled before corruption")
	}
	if err := os.WriteFile(filepath.Join(root, "config", "user.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	m.InvalidateConfigCache()
	if m.IsTemplateEnabled("a.yaml") {
		t.Error("load failure must report no template as enabled")
	}
}

// TestFilterEnabled_MalformedConfigPassesInputThrough documents the
// asymmetry with IsTemplateEnabled: FilterEnabled returns its input
// unchanged on a load error (fail-open) rather than scoping to none.
func TestFilterEnabled_MalformedConfigPassesInputThrough(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "a.yaml")
	if err := os.WriteFile(filepath.Join(root, "config", "user.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	m.InvalidateConfigCache()
	in := []string{"a.yaml", "b.yaml"}
	got := m.FilterEnabled(in)
	if len(got) != 2 || got[0] != "a.yaml" || got[1] != "b.yaml" {
		t.Errorf("FilterEnabled on load error = %v, want input unchanged %v", got, in)
	}
}

func TestSetTemplateEnabled_EmptyFilenameRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SetTemplateEnabled("", true)
	if err == nil {
		t.Fatal("expected error for empty filename")
	}
	if !strings.Contains(err.Error(), "empty filename") {
		t.Errorf("err = %v, want empty-filename message", err)
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

// ----- VFS lookups for missing / odd paths ---------------------------

func TestGetTemplateStorageInfo_EmptyFilenameNil(t *testing.T) {
	m, _, _ := newTestManagerRootContext(t)
	if info := m.GetTemplateStorageInfo(""); info != nil {
		t.Errorf("empty filename must return nil, got %+v", info)
	}
}

func TestGetTemplateStorageInfo_MissingTemplateNil(t *testing.T) {
	m, _, _ := newTestManagerRootContext(t)
	if info := m.GetTemplateStorageInfo("nope.yaml"); info != nil {
		t.Errorf("unknown template must return nil, got %+v", info)
	}
}

// TestGetTemplateStorageInfo_ExtensionlessMatchesByStem confirms the lookup
// strips templateExt before keying, so "basic" resolves the same record as
// "basic.yaml".
func TestGetTemplateStorageInfo_ExtensionlessMatchesByStem(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/basic.yaml", "name: Basic"); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	m.DirtyVirtualStructure()
	if _, err := m.GetVirtualStructure(); err != nil {
		t.Fatalf("build vfs: %v", err)
	}

	withExt := m.GetTemplateStorageInfo("basic.yaml")
	noExt := m.GetTemplateStorageInfo("basic")
	if withExt == nil || noExt == nil {
		t.Fatalf("both lookups should resolve: withExt=%v noExt=%v", withExt, noExt)
	}
	if withExt.Name != "basic" || noExt.Name != "basic" {
		t.Errorf("names = %q / %q, want basic", withExt.Name, noExt.Name)
	}
	if withExt.Filename != "basic.yaml" || noExt.Filename != "basic.yaml" {
		t.Errorf("Filename = %q / %q, want basic.yaml", withExt.Filename, noExt.Filename)
	}
	// Stem lookup must resolve the SAME record as the full-filename lookup.
	if withExt.Path != noExt.Path {
		t.Errorf("paths diverge: withExt=%q noExt=%q", withExt.Path, noExt.Path)
	}
}

// TestGetVirtualStructure_IgnoresNonTemplateFiles seeds a stray .txt next to
// a real .yaml; only the .yaml becomes a storage folder.
func TestGetVirtualStructure_IgnoresNonTemplateFiles(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/real.yaml", "name: Real"); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	if err := sys.SaveFile("templates/readme.txt", "ignore me"); err != nil {
		t.Fatalf("seed txt: %v", err)
	}
	m.DirtyVirtualStructure()
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("GetVirtualStructure: %v", err)
	}
	if len(vfs.TemplateStorageFolders) != 1 {
		t.Fatalf("expected exactly 1 folder, got %d (%v)", len(vfs.TemplateStorageFolders), vfs.TemplateStorageFolders)
	}
	if _, ok := vfs.TemplateStorageFolders["real"]; !ok {
		t.Error("real.yaml must yield a storage folder")
	}
	if _, ok := vfs.TemplateStorageFolders["readme"]; ok {
		t.Error("readme.txt must not yield a storage folder")
	}
}

// TestGetTemplateStorageInfo_TraversalNameNil confirms an odd lookup key (path
// traversal, slashes, dot-prefix) resolves to nil rather than matching a real
// stem or escaping the keyed map. The VFS keys on bare stems only.
func TestGetTemplateStorageInfo_TraversalNameNil(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	if err := sys.SaveFile("templates/basic.yaml", "name: Basic"); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	m.DirtyVirtualStructure()
	if _, err := m.GetVirtualStructure(); err != nil {
		t.Fatalf("build vfs: %v", err)
	}
	for _, odd := range []string{"../basic", "../basic.yaml", "storage/basic", "./basic", ".basic"} {
		if info := m.GetTemplateStorageInfo(odd); info != nil {
			t.Errorf("odd key %q must not resolve, got %+v", odd, info)
		}
	}
	// The clean stem still resolves: proves the odd keys failed on the key,
	// not because the VFS was empty.
	if info := m.GetTemplateStorageInfo("basic"); info == nil {
		t.Error("clean stem basic must still resolve")
	}
}

// TestImportUserProfile_NonObjectSourceUndoesCopy covers a source that is valid
// JSON but the wrong shape (an array). parseUserConfig's probe-decode into a map
// rejects it, so the copy is rolled back and no orphan remains.
func TestImportUserProfile_NonObjectSourceUndoesCopy(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "array-source.json")
	if err := os.WriteFile(src, []byte(`[1,2,3]`), 0o644); err != nil {
		t.Fatalf("seed array source: %v", err)
	}
	r := m.ImportUserProfile(src, "arr.json", false)
	if r.Success {
		t.Fatal("array-shaped source import must fail")
	}
	if r.Code != "invalid_config" {
		t.Errorf("code = %q, want invalid_config", r.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "arr.json")); !os.IsNotExist(err) {
		t.Errorf("orphan target must be removed, stat err = %v", err)
	}
}

// TestGetContextPath_EmptyContextFallsBackToRoot covers the odd-config case:
// an empty context_folder must not error; it resolves to the workspace root.
func TestGetContextPath_EmptyContextFallsBackToRoot(t *testing.T) {
	m, _, root := newTestManagerRootContext(t)
	if _, err := m.UpdateUserConfig(map[string]any{"context_folder": ""}); err != nil {
		t.Fatalf("set empty context: %v", err)
	}
	got, err := m.GetContextPath()
	if err != nil {
		t.Fatalf("GetContextPath: %v", err)
	}
	if got != root {
		t.Errorf("GetContextPath = %q, want root %q", got, root)
	}
}

// ----- Concurrent profile writes under -race -------------------------

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

// TestConcurrent_SetTemplateEnabledNoCorruption hammers SetTemplateEnabled with
// concurrent toggles of N distinct filenames against an explicit-empty baseline.
//
// CURRENT BEHAVIOR (asserted here): SetTemplateEnabled reads cfg.EnabledTemplates
// via LoadUserConfig BEFORE taking updateMu (enabled_templates.go:91), then hands
// the rebuilt slice to UpdateUserConfig which takes updateMu only for the write.
// The read-modify-write is therefore NOT atomic across the lock, so concurrent
// togglers each start from a stale baseline and overwrite each other. The final
// count collapses to roughly 1 instead of N. See suspectedBugs.
//
// What the structure still guarantees, and what this test pins: the file always
// parses, every surviving entry is a valid known filename, there are no
// duplicates and no torn (partial) entries, and at least one toggle survives.
// We do NOT assert len == N because that would assert a fix that does not exist.
func TestConcurrent_SetTemplateEnabledNoCorruption(t *testing.T) {
	t.Parallel()
	m, _, root := newTestManager(t)
	withEnabled(t, m) // seed an explicit empty (curation mode on)

	const N = 20
	valid := map[string]bool{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range N {
		name := "f" + string(rune('a'+i%26)) + ".yaml"
		mu.Lock()
		valid[name] = true
		mu.Unlock()
		wg.Go(func() {
			if _, err := m.SetTemplateEnabled(name, true); err != nil {
				t.Errorf("SetTemplateEnabled(%s): %v", name, err)
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
		t.Fatalf("on-disk profile is corrupt after concurrent toggles: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range disk.EnabledTemplates {
		if !valid[e] {
			t.Errorf("unexpected entry %q in enabled_templates", e)
		}
		if seen[e] {
			t.Errorf("duplicate entry %q in enabled_templates", e)
		}
		seen[e] = true
	}
	// At least one writer must survive; an empty result would mean total loss
	// or corruption, which is worse than the known lost-update behavior.
	if len(seen) < 1 {
		t.Errorf("enabled count = %d, want at least 1 surviving toggle", len(seen))
	}
	if len(disk.EnabledTemplates) > N {
		t.Errorf("on-disk slice len = %d exceeds N=%d (impossible without corruption)", len(disk.EnabledTemplates), N)
	}
	// Cache and disk must agree on the final slice regardless of how many
	// updates were lost.
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != len(disk.EnabledTemplates) {
		t.Errorf("cache/disk diverge: cache len=%d disk len=%d", len(cfg.EnabledTemplates), len(disk.EnabledTemplates))
	}
}

// TestSetTemplateEnabled_SequentialAccumulatesAll proves the lost updates in the
// concurrent test above are purely a concurrency artifact, not a logic error in
// the toggle: run serially, every distinct on-toggle accumulates so the final
// slice holds all N names in insertion order.
func TestSetTemplateEnabled_SequentialAccumulatesAll(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m)

	const N = 5
	want := make([]string, N)
	for i := range N {
		name := "s" + string(rune('a'+i)) + ".yaml"
		want[i] = name
		got, err := m.SetTemplateEnabled(name, true)
		if err != nil {
			t.Fatalf("SetTemplateEnabled(%s): %v", name, err)
		}
		if len(got) != i+1 || got[i] != name {
			t.Fatalf("after adding %s: got %v, want %d entries ending in %s", name, got, i+1, name)
		}
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != N {
		t.Fatalf("final count = %d, want %d (%v)", len(cfg.EnabledTemplates), N, cfg.EnabledTemplates)
	}
	for i, w := range want {
		if cfg.EnabledTemplates[i] != w {
			t.Errorf("entry %d = %q, want %q (insertion order)", i, cfg.EnabledTemplates[i], w)
		}
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
