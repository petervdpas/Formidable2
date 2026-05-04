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

	ctx.Step(`^boot.json's active_profile is "([^"]*)"$`, func(want string) error {
		raw, err := os.ReadFile(filepath.Join(w.tmp, "config", "boot.json"))
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
}

func profileValues(profiles []ProfileEntry) []string {
	out := make([]string, len(profiles))
	for i, p := range profiles {
		out[i] = p.Value
	}
	return out
}

func vfsTemplates(v *VirtualStructure) []string {
	out := make([]string, 0, len(v.TemplateStorageFolders))
	for k := range v.TemplateStorageFolders {
		out = append(out, k)
	}
	return out
}
