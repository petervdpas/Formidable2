package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initConfigScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// configWorld is the per-scenario state.
type configWorld struct {
	tmp          string
	sys          *system.Manager
	m            *Manager
	profiles     []ProfileEntry
	deleteResult ProfileResult
	importResult ProfileResult
	exportResult ProfileResult
	vfs          *VirtualStructure
	lastErr      error
	lister       *godogTemplateLister
	enabledList  []string
}

// godogTemplateLister is the stub TemplateLister used by the
// EnabledTemplates scenarios. Configured per-scenario via the
// "the live templates folder contains" step.
type godogTemplateLister struct {
	files []string
}

func (l *godogTemplateLister) ListTemplates() ([]string, error) {
	return append([]string(nil), l.files...), nil
}

func initConfigScenario(ctx *godog.ScenarioContext) {
	w := &configWorld{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "config-godog-")
		if err != nil {
			return ctx, err
		}
		w.tmp = dir
		w.sys = system.NewManager(dir, nil)
		w.profiles = nil
		w.deleteResult = ProfileResult{}
		w.importResult = ProfileResult{}
		w.exportResult = ProfileResult{}
		w.vfs = nil
		w.lastErr = nil
		w.lister = &godogTemplateLister{}
		w.enabledList = nil
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// ── Givens ────────────────────────────────────────────────────────

	ctx.Step(`^a config manager rooted at a fresh temp directory$`, func() error {
		log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		m, err := NewManager(w.sys, log)
		if err != nil {
			return err
		}
		// Scenarios use root-relative paths (templates/..., storage/...)
		// for readability. The first-run default is "./Examples"; override
		// to "./" so the VFS scanner finds files at the repo root.
		// The "first-run uses ./Examples" promise is exercised by a
		// dedicated unit test (TestNewManager_FirstRunContextFolderIsExamples).
		if _, err := m.UpdateUserConfig(map[string]any{"context_folder": "./"}); err != nil {
			return err
		}
		// Wire the godog template lister by default so the EnabledTemplates
		// scenarios can exercise ListEnabledTemplates / Reconcile without
		// extra setup. The "Without a template lister wired" scenario
		// clears it explicitly.
		m.SetTemplateLister(w.lister)
		w.m = m
		return nil
	})

	ctx.Step(`^a profile "([^"]*)" exists with theme "([^"]*)"$`, func(filename, theme string) error {
		alt := defaultConfig()
		alt.Theme = theme
		alt.ProfileName = strings.TrimSuffix(filename, ".json")
		bytes, err := json.MarshalIndent(alt, "", "  ")
		if err != nil {
			return err
		}
		return w.sys.SaveFile(filepath.Join("config", filename), string(bytes))
	})

	ctx.Step(`^the file "([^"]*)" with content "([^"]*)"$`, func(path, content string) error {
		return w.sys.SaveFile(path, content)
	})

	ctx.Step(`^the file "([^"]*)" with content '([^']*)'$`, func(path, content string) error {
		return w.sys.SaveFile(path, content)
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I update the config with theme "([^"]*)" and font_size (\d+)$`, func(theme string, size int) error {
		_, err := w.m.UpdateUserConfig(map[string]any{
			"theme":     theme,
			"font_size": size,
		})
		w.lastErr = err
		return nil
	})

	ctx.Step(`^I switch the active profile to "([^"]*)"$`, func(filename string) error {
		_, err := w.m.SwitchUserProfile(filename)
		w.lastErr = err
		return nil
	})

	ctx.Step(`^I delete the profile "([^"]*)"$`, func(filename string) error {
		w.deleteResult = w.m.DeleteUserProfile(filename)
		return nil
	})

	ctx.Step(`^I export the profile "([^"]*)" to "([^"]*)"$`, func(filename, target string) error {
		w.exportResult = w.m.ExportUserProfile(filename, filepath.Join(w.tmp, target), false)
		return nil
	})

	ctx.Step(`^I import the profile from "([^"]*)" as "([^"]*)"$`, func(source, name string) error {
		w.importResult = w.m.ImportUserProfile(filepath.Join(w.tmp, source), name, false)
		return nil
	})

	ctx.Step(`^I load the config$`, func() error {
		_, w.lastErr = w.m.LoadUserConfig()
		return nil
	})

	ctx.Step(`^I invalidate the config cache$`, func() error {
		w.m.InvalidateConfigCache()
		return nil
	})

	ctx.Step(`^an external profile file "([^"]*)" exists with theme "([^"]*)"$`, func(filename, theme string) error {
		alt := defaultConfig()
		alt.Theme = theme
		bytes, err := json.MarshalIndent(alt, "", "  ")
		if err != nil {
			return err
		}
		return w.sys.SaveFile(filename, string(bytes))
	})

	ctx.Step(`^an external file "([^"]*)" exists with content "([^"]*)"$`, func(filename, content string) error {
		decoded := strings.ReplaceAll(content, `\n`, "\n")
		return w.sys.SaveFile(filename, decoded)
	})

	ctx.Step(`^I list available profiles$`, func() error {
		profiles, err := w.m.ListAvailableProfiles()
		if err != nil {
			return err
		}
		w.profiles = profiles
		return nil
	})

	ctx.Step(`^I request the virtual structure$`, func() error {
		vfs, err := w.m.GetVirtualStructure()
		if err != nil {
			return err
		}
		w.vfs = vfs
		return nil
	})

	ctx.Step(`^I dirty the virtual structure$`, func() error {
		w.m.DirtyVirtualStructure()
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the file "([^"]*)" exists$`, func(path string) error {
		full := filepath.Join(w.tmp, path)
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("expected file %q to exist: %v", path, err)
		}
		return nil
	})

	ctx.Step(`^the file "([^"]*)" does not exist$`, func(path string) error {
		full := filepath.Join(w.tmp, path)
		if _, err := os.Stat(full); err == nil {
			return fmt.Errorf("expected file %q to be absent", path)
		}
		return nil
	})

	ctx.Step(`^I reinitialize the config manager$`, func() error {
		log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		m, err := NewManager(w.sys, log)
		if err != nil {
			return err
		}
		w.m = m
		return nil
	})

	ctx.Step(`^the active profile filename is "([^"]*)"$`, func(want string) error {
		got := w.m.CurrentProfileFilename()
		if got != want {
			return fmt.Errorf("active profile = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the loaded config has theme "([^"]*)"$`, func(want string) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		if cfg.Theme != want {
			return fmt.Errorf("theme = %q, want %q", cfg.Theme, want)
		}
		return nil
	})

	ctx.Step(`^the loaded config has language "([^"]*)"$`, func(want string) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		if cfg.Language != want {
			return fmt.Errorf("language = %q, want %q", cfg.Language, want)
		}
		return nil
	})

	ctx.Step(`^the loaded config has internal_server_port (\d+)$`, func(want int) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		if cfg.InternalServerPort != want {
			return fmt.Errorf("internal_server_port = %d, want %d", cfg.InternalServerPort, want)
		}
		return nil
	})

	ctx.Step(`^the loaded config has font_size (\d+)$`, func(want int) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		if cfg.FontSize != want {
			return fmt.Errorf("font_size = %d, want %d", cfg.FontSize, want)
		}
		return nil
	})

	ctx.Step(`^the disk file "([^"]*)" reflects theme "([^"]*)"$`, func(path, want string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		var disk map[string]any
		if err := json.Unmarshal(raw, &disk); err != nil {
			return err
		}
		got, _ := disk["theme"].(string)
		if got != want {
			return fmt.Errorf("disk theme = %q, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^\.boot\.json's active_profile is "([^"]*)"$`, func(want string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, "config", ".boot.json"))
		if err != nil {
			return err
		}
		var boot BootConfig
		if err := json.Unmarshal(raw, &boot); err != nil {
			return err
		}
		if boot.ActiveProfile != want {
			return fmt.Errorf("boot.active_profile = %q, want %q", boot.ActiveProfile, want)
		}
		return nil
	})

	ctx.Step(`^the delete result is failure with code "([^"]*)"$`, func(code string) error {
		if w.deleteResult.Success {
			return fmt.Errorf("expected failure, got success: %+v", w.deleteResult)
		}
		if w.deleteResult.Code != code {
			return fmt.Errorf("delete code = %q, want %q", w.deleteResult.Code, code)
		}
		return nil
	})

	ctx.Step(`^the export result is failure with code "([^"]*)"$`, func(code string) error {
		if w.exportResult.Success {
			return fmt.Errorf("expected failure, got success: %+v", w.exportResult)
		}
		if w.exportResult.Code != code {
			return fmt.Errorf("export code = %q, want %q", w.exportResult.Code, code)
		}
		return nil
	})

	ctx.Step(`^the import result is success with filename "([^"]*)"$`, func(filename string) error {
		if !w.importResult.Success {
			return fmt.Errorf("expected success, got: %+v", w.importResult)
		}
		if w.importResult.Filename != filename {
			return fmt.Errorf("import filename = %q, want %q", w.importResult.Filename, filename)
		}
		return nil
	})

	ctx.Step(`^the import result is failure with code "([^"]*)"$`, func(code string) error {
		if w.importResult.Success {
			return fmt.Errorf("expected failure, got success: %+v", w.importResult)
		}
		if w.importResult.Code != code {
			return fmt.Errorf("import code = %q, want %q", w.importResult.Code, code)
		}
		return nil
	})

	ctx.Step(`^the profile list contains "([^"]*)"$`, func(filename string) error {
		for _, p := range w.profiles {
			if p.Value == filename {
				return nil
			}
		}
		return fmt.Errorf("profile %q not in list %v", filename, profileValues(w.profiles))
	})

	ctx.Step(`^the profile list does not contain "([^"]*)"$`, func(filename string) error {
		for _, p := range w.profiles {
			if p.Value == filename {
				return fmt.Errorf("profile %q should NOT be in list %v", filename, profileValues(w.profiles))
			}
		}
		return nil
	})

	ctx.Step(`^the folder "([^"]*)" exists$`, func(path string) error {
		full := filepath.Join(w.tmp, path)
		info, err := os.Stat(full)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", path)
		}
		return nil
	})

	ctx.Step(`^the virtual structure contains template "([^"]*)"$`, func(name string) error {
		if w.vfs == nil {
			return fmt.Errorf("virtual structure not requested")
		}
		if _, ok := w.vfs.TemplateStorageFolders[name]; !ok {
			return fmt.Errorf("template %q missing; have %v", name, vfsTemplates(w.vfs))
		}
		return nil
	})

	ctx.Step(`^I set status button "([^"]*)" to (on|off)$`, func(name, state string) error {
		on := state == "on"
		_, err := w.m.UpdateUserConfig(map[string]any{
			"status_buttons": map[string]any{name: on},
		})
		w.lastErr = err
		return err
	})

	ctx.Step(`^the status button "([^"]*)" is (on|off)$`, func(name, state string) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		got, ok := statusButtonField(cfg.StatusButtons, name)
		if !ok {
			return fmt.Errorf("unknown status button %q", name)
		}
		want := state == "on"
		if got != want {
			return fmt.Errorf("status_buttons.%s = %v, want %v", name, got, want)
		}
		return nil
	})

	// ── EnabledTemplates ─────────────────────────────────────────────

	splitCSV := func(s string) []string {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, strings.TrimSpace(p))
		}
		return out
	}

	ctx.Step(`^the live templates folder contains "([^"]*)"$`, func(csv string) error {
		w.lister.files = splitCSV(csv)
		return nil
	})

	ctx.Step(`^I set the enabled templates to "([^"]*)"$`, func(csv string) error {
		_, err := w.m.UpdateUserConfig(map[string]any{
			"enabled_templates": splitCSV(csv),
		})
		return err
	})

	ctx.Step(`^I clear the template lister$`, func() error {
		w.m.SetTemplateLister(nil)
		return nil
	})

	ctx.Step(`^I reconcile enabled templates$`, func() error {
		got, err := w.m.ReconcileEnabledTemplates()
		if err != nil {
			return err
		}
		w.enabledList = got
		return nil
	})

	ctx.Step(`^I list enabled templates$`, func() error {
		got, err := w.m.ListEnabledTemplates()
		if err != nil {
			return err
		}
		w.enabledList = got
		return nil
	})

	ctx.Step(`^template "([^"]*)" is enabled$`, func(name string) error {
		if !w.m.IsTemplateEnabled(name) {
			return fmt.Errorf("expected template %q to be enabled", name)
		}
		return nil
	})

	ctx.Step(`^template "([^"]*)" is not enabled$`, func(name string) error {
		if w.m.IsTemplateEnabled(name) {
			return fmt.Errorf("expected template %q to NOT be enabled", name)
		}
		return nil
	})

	ctx.Step(`^the enabled templates list is "([^"]*)"$`, func(csv string) error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		want := splitCSV(csv)
		if !slicesEqual(cfg.EnabledTemplates, want) {
			return fmt.Errorf("EnabledTemplates = %v, want %v", cfg.EnabledTemplates, want)
		}
		return nil
	})

	ctx.Step(`^the enabled templates list is empty$`, func() error {
		cfg, err := w.m.LoadUserConfig()
		if err != nil {
			return err
		}
		if len(cfg.EnabledTemplates) != 0 {
			return fmt.Errorf("EnabledTemplates = %v, want empty", cfg.EnabledTemplates)
		}
		return nil
	})

	ctx.Step(`^the listed enabled templates are "([^"]*)"$`, func(csv string) error {
		want := splitCSV(csv)
		if !slicesEqual(w.enabledList, want) {
			return fmt.Errorf("listed = %v, want %v", w.enabledList, want)
		}
		return nil
	})

	ctx.Step(`^the listed enabled templates are empty$`, func() error {
		if len(w.enabledList) != 0 {
			return fmt.Errorf("listed = %v, want empty", w.enabledList)
		}
		return nil
	})

	ctx.Step(`^the disk file "([^"]*)" contains "([^"]*)"$`, func(path, needle string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		if !strings.Contains(string(raw), needle) {
			return fmt.Errorf("file %q missing %q", path, needle)
		}
		return nil
	})

	ctx.Step(`^the disk file "([^"]*)" does not contain "([^"]*)"$`, func(path, needle string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), needle) {
			return fmt.Errorf("file %q must not contain %q", path, needle)
		}
		return nil
	})

	ctx.Step(`^the disk file "([^"]*)" reflects status button "([^"]*)" (on|off)$`, func(path, name, state string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, path))
		if err != nil {
			return err
		}
		var disk struct {
			StatusButtons map[string]any `json:"status_buttons"`
		}
		if err := json.Unmarshal(raw, &disk); err != nil {
			return err
		}
		v, ok := disk.StatusButtons[name]
		if !ok {
			return fmt.Errorf("status_buttons.%s missing on disk; have %v", name, disk.StatusButtons)
		}
		got, ok := v.(bool)
		if !ok {
			return fmt.Errorf("status_buttons.%s is %T, want bool", name, v)
		}
		want := state == "on"
		if got != want {
			return fmt.Errorf("disk status_buttons.%s = %v, want %v", name, got, want)
		}
		return nil
	})
}

func statusButtonField(b StatusButtons, name string) (bool, bool) {
	switch name {
	case "reloader":
		return b.Reloader, true
	case "charpicker":
		return b.Charpicker, true
	case "gitquick":
		return b.Gitquick, true
	case "gigotload":
		return b.Gigotload, true
	case "language":
		return b.Language, true
	}
	return false, false
}

func profileValues(profiles []ProfileEntry) []string {
	out := make([]string, len(profiles))
	for i, p := range profiles {
		out[i] = p.Value
	}
	return out
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func vfsTemplates(v *VirtualStructure) []string {
	out := make([]string, 0, len(v.TemplateStorageFolders))
	for k := range v.TemplateStorageFolders {
		out = append(out, k)
	}
	return out
}
