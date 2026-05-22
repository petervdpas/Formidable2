package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func newTestManager(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	m, err := NewManager(sys, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, sys, root
}

// newTestManagerRootContext is like newTestManager but overrides the
// first-run "./Examples" default to "./" so legacy tests using
// root-relative paths (templates/..., storage/...) keep working.
// Use this for VFS / journal / placement tests that don't care about
// the default context folder. Use newTestManager for first-run / default
// behavior tests.
func newTestManagerRootContext(t *testing.T) (*Manager, *system.Manager, string) {
	t.Helper()
	m, sys, root := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{"context_folder": "./"}); err != nil {
		t.Fatalf("override context_folder: %v", err)
	}
	return m, sys, root
}

// ----- Initialization & defaults -------------------------------------

func TestNewManager_SeedsBootAndUserConfig(t *testing.T) {
	m, _, root := newTestManager(t)

	bootPath := filepath.Join(root, "config", ".boot.json")
	if _, err := os.Stat(bootPath); err != nil {
		t.Fatalf(".boot.json should exist: %v", err)
	}
	legacy := filepath.Join(root, "config", "boot.json")
	if _, err := os.Stat(legacy); err == nil {
		t.Errorf("legacy boot.json must not be seeded on first run")
	}

	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.Theme != "light" || cfg.Language != "en" || cfg.InternalServerPort != 8383 {
		t.Errorf("defaults wrong: theme=%q lang=%q port=%d", cfg.Theme, cfg.Language, cfg.InternalServerPort)
	}
	if cfg.History.Index != -1 {
		t.Errorf("History.Index default = %d, want -1", cfg.History.Index)
	}
	if cfg.WindowBounds.Width != 1024 {
		t.Errorf("WindowBounds.Width default = %d, want 1024", cfg.WindowBounds.Width)
	}
}

// TestNewManager_MigratesLegacyBootJSON exercises the rename-on-read
// migration. An existing install will have config/boot.json on disk;
// after upgrade we must adopt that pointer (preserving active_profile)
// and rewrite it to .boot.json so the new naming wins. The legacy file
// must be removed so subsequent ListAvailableProfiles doesn't surface
// it to the picker.
func TestNewManager_MigratesLegacyBootJSON(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)

	if err := sys.EnsureDirectory("config"); err != nil {
		t.Fatalf("ensure config: %v", err)
	}
	if err := sys.SaveFile("config/boot.json", `{"active_profile":"work.json"}`); err != nil {
		t.Fatalf("seed legacy boot.json: %v", err)
	}
	work := defaultConfig()
	work.ProfileName = "Work"
	work.Theme = "dark"
	wb, _ := json.Marshal(work)
	if err := sys.SaveFile("config/work.json", string(wb)); err != nil {
		t.Fatalf("seed work.json: %v", err)
	}

	m, err := NewManager(sys, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "config", ".boot.json")); err != nil {
		t.Fatalf(".boot.json must exist after migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "boot.json")); err == nil {
		t.Errorf("legacy boot.json should have been removed")
	}

	if got := m.CurrentProfileFilename(); got != "work.json" {
		t.Errorf("CurrentProfileFilename = %q, want work.json (active_profile preserved across rename)", got)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Errorf("active profile content lost: theme=%q", cfg.Theme)
	}
}

// TestNewManager_PrefersDotbootWhenBothExist guards the ambiguous case:
// if both files exist (legacy upgrade interrupted, user copied an old
// backup, etc.) the new file wins and the legacy is cleaned up.
func TestNewManager_PrefersDotbootWhenBothExist(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	_ = sys.EnsureDirectory("config")
	_ = sys.SaveFile("config/boot.json", `{"active_profile":"stale.json"}`)
	_ = sys.SaveFile("config/.boot.json", `{"active_profile":"user.json"}`)

	m, err := NewManager(sys, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := m.CurrentProfileFilename(); got != "user.json" {
		t.Errorf("CurrentProfileFilename = %q, want user.json (.boot.json wins)", got)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "boot.json")); err == nil {
		t.Errorf("stale legacy boot.json should be removed when .boot.json wins")
	}
}

// TestNewManager_MigratesMalformedLegacyBoot ensures the unhappy path:
// a legacy boot.json that's invalid JSON still gets the migration -
// sanitizeBoot fills in defaults and the result is written to the new
// path so the user lands on a working config rather than a hard fail.
func TestNewManager_MigratesMalformedLegacyBoot(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	_ = sys.EnsureDirectory("config")
	_ = sys.SaveFile("config/boot.json", "not json {[}")

	m, err := NewManager(sys, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "config", ".boot.json")); err != nil {
		t.Errorf(".boot.json must exist after migration of malformed legacy: %v", err)
	}
	if got := m.CurrentProfileFilename(); got != defaultProfileName {
		t.Errorf("CurrentProfileFilename = %q, want default %q", got, defaultProfileName)
	}
}

func TestNewManager_FirstRunContextFolderIsExamples(t *testing.T) {
	m, _, _ := newTestManager(t)
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.ContextFolder != "./Examples" {
		t.Errorf("first-run context_folder = %q, want %q", cfg.ContextFolder, "./Examples")
	}
}

func TestParseUserConfig_FillsMissingFields(t *testing.T) {
	raw := `{"theme":"dark","language":"nl"}`
	cfg, changed, err := parseUserConfig(raw)
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for partial input")
	}
	if cfg.Theme != "dark" || cfg.Language != "nl" {
		t.Errorf("present fields lost: %+v", cfg)
	}
	if cfg.InternalServerPort != 8383 {
		t.Errorf("default not filled: port=%d", cfg.InternalServerPort)
	}
}

func TestParseUserConfig_NoChangeWhenComplete(t *testing.T) {
	full := defaultConfig()
	bytes, _ := json.Marshal(full)
	_, changed, err := parseUserConfig(string(bytes))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if changed {
		t.Error("expected changed=false for complete config")
	}
}

func TestParseUserConfig_DefaultToastTimeoutFilledWhenMissing(t *testing.T) {
	raw := `{"theme":"dark"}`
	cfg, _, err := parseUserConfig(raw)
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.ToastTimeout != ToastTimeoutDefault {
		t.Errorf("default not filled: got %d, want %d", cfg.ToastTimeout, ToastTimeoutDefault)
	}
}

func TestParseUserConfig_ToastTimeoutBelowMinClampsToMin(t *testing.T) {
	full := defaultConfig()
	full.ToastTimeout = 0
	bytes, _ := json.Marshal(full)
	cfg, changed, err := parseUserConfig(string(bytes))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.ToastTimeout != ToastTimeoutMin {
		t.Errorf("toast_timeout=%d, want clamped to %d", cfg.ToastTimeout, ToastTimeoutMin)
	}
	if !changed {
		t.Error("clamping must trigger changed=true so the file is rewritten with the sanitised value")
	}
}

func TestParseUserConfig_ToastTimeoutAboveMaxClampsToMax(t *testing.T) {
	full := defaultConfig()
	full.ToastTimeout = 999
	bytes, _ := json.Marshal(full)
	cfg, changed, err := parseUserConfig(string(bytes))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.ToastTimeout != ToastTimeoutMax {
		t.Errorf("toast_timeout=%d, want clamped to %d", cfg.ToastTimeout, ToastTimeoutMax)
	}
	if !changed {
		t.Error("clamping must trigger changed=true")
	}
}

func TestParseUserConfig_ToastTimeoutNegativeClampsToMin(t *testing.T) {
	full := defaultConfig()
	full.ToastTimeout = -7
	bytes, _ := json.Marshal(full)
	cfg, _, err := parseUserConfig(string(bytes))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.ToastTimeout != ToastTimeoutMin {
		t.Errorf("negative toast_timeout should clamp to min, got %d", cfg.ToastTimeout)
	}
}

func TestParseUserConfig_ToastTimeoutInRangePassesThrough(t *testing.T) {
	full := defaultConfig()
	full.ToastTimeout = 8
	bytes, _ := json.Marshal(full)
	cfg, changed, err := parseUserConfig(string(bytes))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.ToastTimeout != 8 {
		t.Errorf("in-range value lost: got %d, want 8", cfg.ToastTimeout)
	}
	if changed {
		t.Error("in-range value must not flip changed")
	}
}

func TestNormalizeProfileFilename(t *testing.T) {
	cases := map[string]string{
		"User.json":          "user.json",
		"My Profile.JSON":    "my-profile.json",
		"weird !! name":      "weird-name.json",
		"---trim---.json":    "trim.json",
		"":                   "",
		"---":                "",
		"/path/to/Foo.json":  "foo.json",
	}
	for in, want := range cases {
		if got := normalizeProfileFilename(in); got != want {
			t.Errorf("normalizeProfileFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

// ----- Load / Update / cache invalidation ----------------------------

// TestUpdateUserConfig_WindowBoundsRoundTrip exercises the path
// main.go uses to persist window geometry on close: pass a populated
// WindowBounds via the partial map; expect the values back on reload.
// Pointer fields (X, Y) must survive the JSON round-trip intact.
func TestUpdateUserConfig_WindowBoundsRoundTrip(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestManager(t)

	x, y := 137, 42
	bounds := WindowBounds{
		Width:     1280,
		Height:    900,
		X:         &x,
		Y:         &y,
		Maximized: false,
	}
	if _, err := m.UpdateUserConfig(map[string]any{
		"window_bounds": bounds,
	}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}

	m.InvalidateConfigCache()
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.WindowBounds.Width != 1280 || cfg.WindowBounds.Height != 900 {
		t.Errorf("size = %dx%d, want 1280x900", cfg.WindowBounds.Width, cfg.WindowBounds.Height)
	}
	if cfg.WindowBounds.X == nil || *cfg.WindowBounds.X != 137 {
		t.Errorf("X = %v, want 137", cfg.WindowBounds.X)
	}
	if cfg.WindowBounds.Y == nil || *cfg.WindowBounds.Y != 42 {
		t.Errorf("Y = %v, want 42", cfg.WindowBounds.Y)
	}
}

func TestUpdateUserConfig_PartialMerge(t *testing.T) {
	m, _, root := newTestManager(t)

	if _, err := m.UpdateUserConfig(map[string]any{
		"theme": "dark",
		"font_size": 16,
	}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}

	cfg, _ := m.LoadUserConfig()
	if cfg.Theme != "dark" || cfg.FontSize != 16 {
		t.Errorf("update not applied: %+v", cfg)
	}
	// untouched defaults survive
	if cfg.Language != "en" {
		t.Errorf("untouched field changed: %q", cfg.Language)
	}

	// Persisted to disk
	raw, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))
	var disk map[string]any
	_ = json.Unmarshal(raw, &disk)
	if disk["theme"] != "dark" || disk["font_size"] != float64(16) {
		t.Errorf("disk not updated: %v", disk)
	}
}

// TestUpdateUserConfig_NoLostUpdatesUnderConcurrency fires N writers
// that each set a DISTINCT top-level field. Without serialization, two
// writers can both load the same baseline and the later writer's
// merge-and-write silently undoes the earlier one ("lost update"). With
// updateMu, every writer's change must be present at the end.
func TestUpdateUserConfig_NoLostUpdatesUnderConcurrency(t *testing.T) {
	t.Parallel()
	m, _, _ := newTestManager(t)

	type write struct{ field, value string }
	writes := []write{
		{"profile_name",     "Goro_PN"},
		{"theme",            "purplish"},
		{"language",         "nl"},
		{"author_name",      "Goro_AN"},
		{"author_email",     "goro@example.com"},
		{"gigot_base_url",   "https://goro.example/u"},
		{"gigot_repo_name",  "Goro_REPO"},
		{"gigot_token",      "Goro_TOKEN"},
		{"git_root",         "/goro/root"},
		{"git_branch",       "goro-branch"},
		{"remote_backend",   "git"},
		{"selected_data_file", "goro.meta.json"},
	}

	var wg sync.WaitGroup
	for _, w := range writes {
		wg.Go(func() {
			if _, err := m.UpdateUserConfig(map[string]any{w.field: w.value}); err != nil {
				t.Errorf("UpdateUserConfig(%s): %v", w.field, err)
			}
		})
	}
	wg.Wait()

	final, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	b, _ := json.Marshal(final)
	finalMap := map[string]any{}
	_ = json.Unmarshal(b, &finalMap)

	for _, w := range writes {
		got, ok := finalMap[w.field]
		if !ok {
			t.Errorf("field %s missing from final config", w.field)
			continue
		}
		if got != w.value {
			t.Errorf("field %s = %v, want %q (lost update)", w.field, got, w.value)
		}
	}
}

func TestInvalidateConfigCache_ReloadsFromDisk(t *testing.T) {
	m, _, root := newTestManager(t)
	_, _ = m.LoadUserConfig()

	// Mutate the file directly (simulate external edit).
	cfgPath := filepath.Join(root, "config", "user.json")
	raw, _ := os.ReadFile(cfgPath)
	var d map[string]any
	_ = json.Unmarshal(raw, &d)
	d["theme"] = "purplish"
	out, _ := json.MarshalIndent(d, "", "  ")
	_ = os.WriteFile(cfgPath, out, 0o644)

	// Without invalidation, cache wins.
	cfg, _ := m.LoadUserConfig()
	if cfg.Theme != "light" {
		t.Errorf("expected stale cache, got theme=%q", cfg.Theme)
	}

	m.InvalidateConfigCache()
	cfg, _ = m.LoadUserConfig()
	if cfg.Theme != "purplish" {
		t.Errorf("expected fresh load, got theme=%q", cfg.Theme)
	}
}

// ----- Virtual structure --------------------------------------------

func TestVirtualStructure_AutoCreatesContextDirs(t *testing.T) {
	m, sys, root := newTestManagerRootContext(t)
	_, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("GetVirtualStructure: %v", err)
	}

	if !sys.FileExists(filepath.Join(root, "templates")) {
		t.Error("templates dir not created")
	}
	if !sys.FileExists(filepath.Join(root, "storage")) {
		t.Error("storage dir not created")
	}
}

func TestVirtualStructure_ScansTemplatesAndCreatesStorageFolders(t *testing.T) {
	m, sys, root := newTestManagerRootContext(t)

	// Seed two templates.
	_ = sys.SaveFile("templates/basic.yaml", "name: Basic\nfields: []\n")
	_ = sys.SaveFile("templates/people.yaml", "name: People\nfields: []\n")

	m.DirtyVirtualStructure()
	vfs, err := m.GetVirtualStructure()
	if err != nil {
		t.Fatalf("GetVirtualStructure: %v", err)
	}
	if len(vfs.TemplateStorageFolders) != 2 {
		t.Fatalf("expected 2 storage folders, got %d", len(vfs.TemplateStorageFolders))
	}
	for _, name := range []string{"basic", "people"} {
		info := vfs.TemplateStorageFolders[name]
		if info.Filename != name+".yaml" {
			t.Errorf("entry %q has filename %q", name, info.Filename)
		}
		if !sys.FileExists(filepath.Join(root, "storage", name)) {
			t.Errorf("storage folder for %q not created", name)
		}
		if !sys.FileExists(filepath.Join(root, "storage", name, "images")) {
			t.Errorf("images folder for %q not created", name)
		}
	}
}

func TestVirtualStructure_ListsMetaAndImageFiles(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	_ = sys.SaveFile("templates/basic.yaml", "name: Basic")
	// Ensure folders exist via first call
	_, _ = m.GetVirtualStructure()
	_ = sys.SaveFile("storage/basic/form-1.meta.json", `{"data":{}}`)
	_ = sys.SaveFile("storage/basic/form-2.meta.json", `{"data":{}}`)
	_ = sys.SaveFile("storage/basic/notes.txt", "stray") // should be ignored
	_ = sys.SaveFile("storage/basic/images/pic.png", "fake")

	m.DirtyVirtualStructure()
	vfs, _ := m.GetVirtualStructure()
	info := vfs.TemplateStorageFolders["basic"]
	if len(info.MetaFiles) != 2 {
		t.Errorf("MetaFiles got %d, want 2 (got %v)", len(info.MetaFiles), info.MetaFiles)
	}
	if len(info.ImageFiles) != 1 || info.ImageFiles[0] != "pic.png" {
		t.Errorf("ImageFiles = %v", info.ImageFiles)
	}
}

func TestVirtualStructure_TTLRebuild(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)

	clock := time.Date(2026, 5, 4, 22, 0, 0, 0, time.UTC)
	m.SetNowFn(func() time.Time { return clock })
	m.SetTTL(5 * time.Second)

	_ = sys.SaveFile("templates/basic.yaml", "name: Basic")
	// Force the very first GetVirtualStructure to rebuild against the
	// frozen clock - without this, the lastBuilt timestamp comes from
	// real time.Now() at construction, which throws off the staleness
	// math under the injected clock.
	m.DirtyVirtualStructure()
	v1, _ := m.GetVirtualStructure()
	if _, ok := v1.TemplateStorageFolders["basic"]; !ok {
		t.Fatal("basic missing from initial VFS")
	}

	// Add another template; cache should hide it within TTL.
	_ = sys.SaveFile("templates/extra.yaml", "name: Extra")
	v2, _ := m.GetVirtualStructure()
	if _, ok := v2.TemplateStorageFolders["extra"]; ok {
		t.Error("extra should not appear within TTL")
	}

	// Advance the clock past TTL → rebuild.
	clock = clock.Add(10 * time.Second)
	v3, _ := m.GetVirtualStructure()
	if _, ok := v3.TemplateStorageFolders["extra"]; !ok {
		t.Error("extra should appear after TTL elapses")
	}
}

func TestDirtyVirtualStructure_ForcesRebuild(t *testing.T) {
	m, sys, _ := newTestManagerRootContext(t)
	_ = sys.SaveFile("templates/basic.yaml", "name: Basic")
	_, _ = m.GetVirtualStructure()
	_ = sys.SaveFile("templates/added.yaml", "name: Added")

	m.DirtyVirtualStructure()
	vfs, _ := m.GetVirtualStructure()
	if _, ok := vfs.TemplateStorageFolders["added"]; !ok {
		t.Error("DirtyVirtualStructure did not force rebuild")
	}
}

// ----- Profiles ------------------------------------------------------

func TestSwitchUserProfile(t *testing.T) {
	m, sys, root := newTestManager(t)

	// Seed an alternate profile on disk.
	alt := defaultConfig()
	alt.ProfileName = "Work"
	alt.Theme = "dark"
	bytes, _ := json.MarshalIndent(alt, "", "  ")
	_ = sys.SaveFile("config/work.json", string(bytes))

	cfg, err := m.SwitchUserProfile("work.json")
	if err != nil {
		t.Fatalf("SwitchUserProfile: %v", err)
	}
	if cfg.Theme != "dark" || cfg.ProfileName != "Work" {
		t.Errorf("loaded profile wrong: %+v", cfg)
	}
	if m.CurrentProfileFilename() != "work.json" {
		t.Errorf("CurrentProfileFilename = %q", m.CurrentProfileFilename())
	}

	// .boot.json should now point at work.json.
	bootRaw, _ := os.ReadFile(filepath.Join(root, "config", ".boot.json"))
	var boot BootConfig
	_ = json.Unmarshal(bootRaw, &boot)
	if boot.ActiveProfile != "work.json" {
		t.Errorf("boot.active_profile = %q", boot.ActiveProfile)
	}
}

// TestSwitchUserProfile_SerializedAgainstUpdate fires N updates and N
// switches concurrently and verifies the final state is internally
// consistent: the active config equals one of the profile files on
// disk, and that file's content matches the active config. Without
// updateMu, an Update could run halfway through a Switch and write
// merged data into the wrong file.
func TestSwitchUserProfile_SerializedAgainstUpdate(t *testing.T) {
	t.Parallel()
	m, sys, root := newTestManager(t)

	// Seed three alternates plus the default.
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		seed := defaultConfig()
		seed.ProfileName = "P-" + name
		buf, _ := json.Marshal(seed)
		_ = sys.SaveFile(filepath.Join("config", name), string(buf))
	}

	const N = 30
	profiles := []string{"user.json", "a.json", "b.json", "c.json"}
	var wg sync.WaitGroup

	// Switch storm.
	for i := range N {
		wg.Go(func() {
			_, _ = m.SwitchUserProfile(profiles[i%len(profiles)])
		})
	}

	// Update storm - every goroutine touches a different field so a
	// lost merge becomes detectable as a missing field after settling.
	tweaks := []string{"author_name", "author_email", "git_root", "gigot_repo_name"}
	for i := range N {
		wg.Go(func() {
			_, _ = m.UpdateUserConfig(map[string]any{tweaks[i%len(tweaks)]: "v" + string(rune('A'+i%26))})
		})
	}
	wg.Wait()

	// After the storm: the on-disk active profile and the in-memory
	// cache must agree.
	curName := m.CurrentProfileFilename()
	if curName == "" {
		t.Fatalf("no current profile after storm")
	}
	cur, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}

	diskRaw, err := os.ReadFile(filepath.Join(root, "config", curName))
	if err != nil {
		t.Fatalf("read active profile from disk: %v", err)
	}
	var disk Config
	if err := json.Unmarshal(diskRaw, &disk); err != nil {
		t.Fatalf("parse active profile: %v", err)
	}
	if disk.AuthorName != cur.AuthorName ||
		disk.AuthorEmail != cur.AuthorEmail ||
		disk.GitRoot != cur.GitRoot ||
		disk.GigotRepoName != cur.GigotRepoName {
		t.Errorf("on-disk %q diverges from cache after concurrent storm:\n disk=%+v\n  mem=%+v",
			curName, disk, cur)
	}
}

func TestListAvailableProfiles_OmitsBoot(t *testing.T) {
	m, sys, _ := newTestManager(t)

	alt := defaultConfig()
	alt.ProfileName = "Work"
	bytes, _ := json.MarshalIndent(alt, "", "  ")
	_ = sys.SaveFile("config/work.json", string(bytes))

	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		t.Fatalf("ListAvailableProfiles: %v", err)
	}
	for _, p := range profiles {
		if p.Value == ".boot.json" || p.Value == "boot.json" {
			t.Errorf("boot pointer must be omitted, got %q", p.Value)
		}
	}
	if len(profiles) != 2 {
		t.Errorf("got %d profiles (expect user.json + work.json)", len(profiles))
	}
}

func TestSwitchUserProfile_RejectsInvalidNames(t *testing.T) {
	m, _, _ := newTestManager(t)

	invalid := []string{
		".boot.json",
		".pdf-state.json",
		"Work.json",
		"work profile.json",
		"work",
		"work.txt",
		"work_profile.json",
		"../escape.json",
		"",
	}
	for _, name := range invalid {
		if _, err := m.SwitchUserProfile(name); err == nil {
			t.Errorf("SwitchUserProfile(%q): expected error, got nil", name)
		}
	}

	for _, name := range []string{"work.json", "user.json", "my-profile.json", "abc123.json"} {
		if _, err := m.SwitchUserProfile(name); err != nil {
			t.Errorf("SwitchUserProfile(%q): unexpected error: %v", name, err)
		}
	}
}

func TestImportUserProfile_RejectsInvalidNames(t *testing.T) {
	m, _, root := newTestManager(t)

	src := filepath.Join(root, "src.json")
	if err := os.WriteFile(src, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	cases := []struct {
		name string
		want string
	}{
		{".pdf-state.json", "boot_forbidden"},
		{".boot.json", "boot_forbidden"},
		{"Work.json", "invalid_name"},
		{"work profile.json", "invalid_name"},
		{"work_profile.json", "invalid_name"},
		{"work", "invalid_name"},
	}
	for _, tc := range cases {
		got := m.ImportUserProfile(src, tc.name, false)
		if got.Success {
			t.Errorf("ImportUserProfile(%q): expected failure", tc.name)
			continue
		}
		if got.Code != tc.want {
			t.Errorf("ImportUserProfile(%q): code=%q, want %q", tc.name, got.Code, tc.want)
		}
	}
}

func TestIsValidProfileFilename(t *testing.T) {
	good := []string{"user.json", "work.json", "my-profile.json", "abc123.json", "a.json"}
	for _, n := range good {
		if !IsValidProfileFilename(n) {
			t.Errorf("IsValidProfileFilename(%q) = false, want true", n)
		}
	}
	bad := []string{
		"", ".json", ".boot.json", "User.json", "user.JSON",
		"my_profile.json", "my profile.json", "user", "user.txt",
		"../user.json", "sub/user.json",
	}
	for _, n := range bad {
		if IsValidProfileFilename(n) {
			t.Errorf("IsValidProfileFilename(%q) = true, want false", n)
		}
	}
}

func TestListAvailableProfiles_OmitsDotFiles(t *testing.T) {
	m, sys, _ := newTestManager(t)

	_ = sys.SaveFile("config/.pdf-state.json", `{"activated":true}`)
	_ = sys.SaveFile("config/.private-cache.json", `{}`)

	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		t.Fatalf("ListAvailableProfiles: %v", err)
	}
	for _, p := range profiles {
		if strings.HasPrefix(p.Value, ".") {
			t.Errorf("dot-file leaked into profiles: %q", p.Value)
		}
	}
}

// TestIoCollectionOnly_DefaultFalseAndPersists exercises the toggle
// that gates the Storage workspace's CSV Import/Export menu against
// templates with enable_collection: false. Default is OFF (the menu
// shows for every template); flipping ON via UpdateUserConfig must
// round-trip on disk and surface through the accessor.
func TestIoCollectionOnly_DefaultFalseAndPersists(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m.IoCollectionOnly() {
		t.Errorf("default IoCollectionOnly = true, want false")
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.IoCollectionOnly {
		t.Errorf("default Config.IoCollectionOnly = true, want false")
	}

	if _, err := m.UpdateUserConfig(map[string]any{"io_collection_only": true}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}
	if !m.IoCollectionOnly() {
		t.Errorf("after Update, IoCollectionOnly = false")
	}

	m.InvalidateConfigCache()
	cfg2, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if !cfg2.IoCollectionOnly {
		t.Errorf("io_collection_only did not round-trip on disk")
	}
}

// TestIoCollectionOnly_UncachedReturnsFalse mirrors GitSelfCloned's
// early-boot guarantee: callers that hit the accessor before the first
// LoadUserConfig must see false, not panic.
func TestIoCollectionOnly_UncachedReturnsFalse(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.InvalidateConfigCache()
	if m.IoCollectionOnly() {
		t.Errorf("expected false when cache is empty")
	}
}

// Accessor methods read out the gigot-related profile fields used by
// gigot.Service to build a Connection. Mirrors IoCollectionOnly's
// "uncached returns zero value" contract so the gigot Service can ask
// without special-casing early-boot.

func TestGigotAccessors_DefaultEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.GigotBaseURL(); got != "" {
		t.Errorf("GigotBaseURL default = %q, want \"\"", got)
	}
	if got := m.GigotRepoName(); got != "" {
		t.Errorf("GigotRepoName default = %q, want \"\"", got)
	}
}

func TestGigotAccessors_ReadFromCachedConfig(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"gigot_base_url":  "https://gigot.example",
		"gigot_repo_name": "addresses",
	}); err != nil {
		t.Fatal(err)
	}
	if got := m.GigotBaseURL(); got != "https://gigot.example" {
		t.Errorf("GigotBaseURL = %q", got)
	}
	if got := m.GigotRepoName(); got != "addresses" {
		t.Errorf("GigotRepoName = %q", got)
	}
}

func TestGigotAccessors_UncachedReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.InvalidateConfigCache()
	if got := m.GigotBaseURL(); got != "" {
		t.Errorf("expected empty when cache is empty, got %q", got)
	}
	if got := m.GigotRepoName(); got != "" {
		t.Errorf("expected empty when cache is empty, got %q", got)
	}
}

func TestAuthorAccessors_ReadFromCachedConfig(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"author_name":  "Alice",
		"author_email": "alice@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if m.AuthorName() != "Alice" {
		t.Errorf("AuthorName = %q", m.AuthorName())
	}
	if m.AuthorEmail() != "alice@example.com" {
		t.Errorf("AuthorEmail = %q", m.AuthorEmail())
	}
}

func TestAuthorAccessors_UncachedReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.InvalidateConfigCache()
	if m.AuthorName() != "" || m.AuthorEmail() != "" {
		t.Errorf("expected empty when cache is empty")
	}
}

func TestContextFolder_ReadsFromCachedConfig(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"context_folder": "/some/where",
	}); err != nil {
		t.Fatal(err)
	}
	if got := m.ContextFolder(); got != "/some/where" {
		t.Errorf("ContextFolder = %q", got)
	}
}

func TestContextFolder_UncachedReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.InvalidateConfigCache()
	if got := m.ContextFolder(); got != "" {
		t.Errorf("expected empty when cache is empty, got %q", got)
	}
}

func TestHasUserProfiles_TrueAfterSeed(t *testing.T) {
	// newTestManager seeds user.json by default → at least one
	// profile exists.
	m, _, _ := newTestManager(t)
	if !m.HasUserProfiles() {
		t.Error("expected true after first-run seed")
	}
}

func TestHasUserProfiles_FalseWhenOnlyBoot(t *testing.T) {
	m, sys, _ := newTestManager(t)
	// Remove the seeded user.json so only the boot pointer remains.
	_ = sys.DeleteFile(sys.JoinPath("config", "user.json"))
	if m.HasUserProfiles() {
		t.Error("expected false when only boot pointer exists")
	}
}

func TestExportUserProfile(t *testing.T) {
	m, _, root := newTestManager(t)
	target := filepath.Join(root, "exports", "out.json")
	r := m.ExportUserProfile("user.json", target, true)
	if !r.Success {
		t.Fatalf("Export failed: %+v", r)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("export missing: %v", err)
	}
}

func TestExportUserProfile_NotFound(t *testing.T) {
	m, _, root := newTestManager(t)
	r := m.ExportUserProfile("nope.json", filepath.Join(root, "x.json"), true)
	if r.Success || r.Code != "not_found" {
		t.Errorf("expected not_found, got %+v", r)
	}
}

func TestDeleteUserProfile_RejectsBootAndActive(t *testing.T) {
	m, _, _ := newTestManager(t)

	r := m.DeleteUserProfile(".boot.json")
	if r.Success || r.Code != "boot_forbidden" {
		t.Errorf(".boot.json delete should be forbidden: %+v", r)
	}
	r = m.DeleteUserProfile("user.json")
	if r.Success || r.Code != "active_profile" {
		t.Errorf("active profile delete should be forbidden: %+v", r)
	}
}

func TestDeleteUserProfile_RemovesFile(t *testing.T) {
	m, sys, _ := newTestManager(t)
	alt := defaultConfig()
	bytes, _ := json.MarshalIndent(alt, "", "  ")
	_ = sys.SaveFile("config/work.json", string(bytes))

	r := m.DeleteUserProfile("work.json")
	if !r.Success {
		t.Fatalf("delete failed: %+v", r)
	}
	if sys.FileExists(sys.ResolvePath("config", "work.json")) {
		t.Error("file should be gone")
	}
}

func TestImportUserProfile(t *testing.T) {
	m, _, root := newTestManager(t)

	// Source file outside config dir.
	src := filepath.Join(root, "import-source.json")
	alt := defaultConfig()
	alt.ProfileName = "Imported"
	bytes, _ := json.MarshalIndent(alt, "", "  ")
	_ = os.WriteFile(src, bytes, 0o644)

	r := m.ImportUserProfile(src, "", false)
	if !r.Success {
		t.Fatalf("Import failed: %+v", r)
	}
	if r.Filename != "import-source.json" {
		t.Errorf("expected normalized filename, got %q", r.Filename)
	}
	if _, err := os.Stat(filepath.Join(root, "config", r.Filename)); err != nil {
		t.Errorf("imported file missing: %v", err)
	}
}

func TestImportUserProfile_RejectsBootJSON(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "x.json")
	_ = os.WriteFile(src, []byte("{}"), 0o644)
	r := m.ImportUserProfile(src, ".boot.json", false)
	if r.Success || r.Code != "boot_forbidden" {
		t.Errorf(".boot.json import should be forbidden: %+v", r)
	}
}

func TestImportUserProfile_ExistsWithoutOverwrite(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "import-source.json")
	alt := defaultConfig()
	bytes, _ := json.MarshalIndent(alt, "", "  ")
	_ = os.WriteFile(src, bytes, 0o644)

	// First import succeeds.
	if r := m.ImportUserProfile(src, "alt.json", false); !r.Success {
		t.Fatalf("first import: %+v", r)
	}
	// Second without overwrite is rejected.
	r := m.ImportUserProfile(src, "alt.json", false)
	if r.Success || r.Code != "exists" {
		t.Errorf("expected exists code, got %+v", r)
	}
}

// ----- EnabledTemplates: field, round-trip, defaults -----------------

func TestEnabledTemplates_DefaultIsNil(t *testing.T) {
	cfg := defaultConfig()
	if cfg.EnabledTemplates != nil {
		t.Errorf("default EnabledTemplates = %v, want nil (nil/empty = all enabled)", cfg.EnabledTemplates)
	}
}

func TestEnabledTemplates_JSONRoundTripPreservesSlice(t *testing.T) {
	in := defaultConfig()
	in.EnabledTemplates = []string{"basic.yaml", "report.yaml"}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, _, err := parseUserConfig(string(raw))
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if len(out.EnabledTemplates) != 2 ||
		out.EnabledTemplates[0] != "basic.yaml" ||
		out.EnabledTemplates[1] != "report.yaml" {
		t.Errorf("round-trip lost slice: %v", out.EnabledTemplates)
	}
}

func TestEnabledTemplates_OmitEmpty(t *testing.T) {
	in := defaultConfig()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "enabled_templates") {
		t.Errorf("nil slice must omit the JSON key, got: %s", raw)
	}
}

func TestEnabledTemplates_LegacyJSONLoadsAsNil(t *testing.T) {
	raw := `{"theme":"dark","language":"en"}`
	cfg, _, err := parseUserConfig(raw)
	if err != nil {
		t.Fatalf("parseUserConfig: %v", err)
	}
	if cfg.EnabledTemplates != nil {
		t.Errorf("legacy profile (no field) must deserialise as nil, got %v", cfg.EnabledTemplates)
	}
}

func TestEnabledTemplates_PersistViaUpdateUserConfig(t *testing.T) {
	m, _, root := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"alpha.yaml", "beta.yaml"},
	}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != 2 ||
		cfg.EnabledTemplates[0] != "alpha.yaml" ||
		cfg.EnabledTemplates[1] != "beta.yaml" {
		t.Errorf("update did not persist: %v", cfg.EnabledTemplates)
	}

	raw, err := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if err != nil {
		t.Fatalf("read user.json: %v", err)
	}
	if !strings.Contains(string(raw), `"enabled_templates"`) {
		t.Errorf("on-disk profile missing field: %s", raw)
	}
}

// TestEnabledTemplates_EmptySliceFromUpdateRoundTrip exercises the
// "user just disabled the last template" edge - the slice is empty,
// not nil, after the update. We accept either nil or [] coming back
// from LoadUserConfig because JSON omitempty turns []string{} into
// nil on re-read; semantically both mean "all enabled", which is what
// the doc says.
func TestEnabledTemplates_EmptySliceFromUpdateRoundTrip(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{},
	}); err != nil {
		t.Fatalf("UpdateUserConfig: %v", err)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != 0 {
		t.Errorf("expected empty/nil slice, got %v", cfg.EnabledTemplates)
	}
}

// ----- Journal hook --------------------------------------------------

type stubJournal struct {
	configures int
	lastCtx    string
	lastBack   string
}

func (s *stubJournal) Configure(ctx, backend string) error {
	s.configures++
	s.lastCtx = ctx
	s.lastBack = backend
	return nil
}

func TestJournalSyncOnLoad(t *testing.T) {
	m, _, _ := newTestManager(t)
	j := &stubJournal{}
	m.SetJournal(j)

	if _, err := m.LoadUserConfig(); err != nil {
		t.Fatal(err)
	}
	// SetJournal already calls syncJournal once (config was cached); LoadUserConfig
	// hits the cache so doesn't re-sync. Either way, at least one Configure call.
	if j.configures < 1 {
		t.Errorf("expected at least one Configure call, got %d", j.configures)
	}
	if j.lastCtx != "./Examples" {
		t.Errorf("ctx forwarded = %q, want \"./Examples\"", j.lastCtx)
	}
	if j.lastBack != "none" {
		t.Errorf("backend forwarded = %q, want \"none\"", j.lastBack)
	}
}
