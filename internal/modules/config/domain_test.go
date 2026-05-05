package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
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

	bootPath := filepath.Join(root, "config", "boot.json")
	if _, err := os.Stat(bootPath); err != nil {
		t.Fatalf("boot.json should exist: %v", err)
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
		{"encryption_key",   "Goro_EK"},
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
	// frozen clock — without this, the lastBuilt timestamp comes from
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

	// boot.json should now point at work.json.
	bootRaw, _ := os.ReadFile(filepath.Join(root, "config", "boot.json"))
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

	// Update storm — every goroutine touches a different field so a
	// lost merge becomes detectable as a missing field after settling.
	tweaks := []string{"author_name", "author_email", "git_root", "encryption_key"}
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
		disk.EncryptionKey != cur.EncryptionKey {
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
		if p.Value == "boot.json" {
			t.Error("boot.json must be omitted")
		}
	}
	if len(profiles) != 2 {
		t.Errorf("got %d profiles (expect user.json + work.json)", len(profiles))
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

	r := m.DeleteUserProfile("boot.json")
	if r.Success || r.Code != "boot_forbidden" {
		t.Errorf("boot.json delete should be forbidden: %+v", r)
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
	r := m.ImportUserProfile(src, "boot.json", false)
	if r.Success || r.Code != "boot_forbidden" {
		t.Errorf("boot.json import should be forbidden: %+v", r)
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
