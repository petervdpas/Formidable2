package app

import (
	"log/slog"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// wireEnablement is the smallest replica of the composition-root wiring
// for the EnabledTemplates feature. The real wiring in app.go is much
// larger; this stub focuses on just the two modules that participate
// so the integration test stays readable.
func wireEnablement(t *testing.T) (*config.Manager, *template.Manager, *system.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfgM, err := config.NewManager(sys, log)
	if err != nil {
		t.Fatalf("config.NewManager: %v", err)
	}
	// Use root-relative paths so the templates folder is at <root>/templates.
	if _, err := cfgM.UpdateUserConfig(map[string]any{"context_folder": "./"}); err != nil {
		t.Fatalf("set context_folder: %v", err)
	}

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}

	// The two wires that make the feature work end-to-end:
	cfgM.SetTemplateLister(tplM)
	tplM.AddObserver(template.ObserverFunc(func(_ string) error {
		_, err := cfgM.ReconcileEnabledTemplates()
		return err
	}))
	return cfgM, tplM, sys
}

func saveStubTemplate(t *testing.T, m *template.Manager, name string) {
	t.Helper()
	tpl := &template.Template{
		Name:     name,
		Filename: name,
		Fields:   []template.Field{{Key: "id", Type: "guid"}, {Key: "title", Type: "text"}},
	}
	if err := m.SaveTemplate(name, tpl); err != nil {
		t.Fatalf("SaveTemplate %s: %v", name, err)
	}
}

// TestTemplateDelete_PrunesEnabledTemplates is the headline integration:
// user enables 3 templates, deletes one via the template manager, and
// expects config.EnabledTemplates to no longer reference the deleted
// one — without any explicit prune call from the caller.
func TestTemplateDelete_PrunesEnabledTemplates(t *testing.T) {
	cfgM, tplM, _ := wireEnablement(t)
	saveStubTemplate(t, tplM, "basic.yaml")
	saveStubTemplate(t, tplM, "report.yaml")
	saveStubTemplate(t, tplM, "extra.yaml")

	if _, err := cfgM.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"basic.yaml", "report.yaml", "extra.yaml"},
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := tplM.DeleteTemplate("report.yaml"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	cfg, err := cfgM.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	want := []string{"basic.yaml", "extra.yaml"}
	if !reflect.DeepEqual(cfg.EnabledTemplates, want) {
		t.Errorf("post-delete EnabledTemplates = %v, want %v", cfg.EnabledTemplates, want)
	}
}

// TestTemplateDelete_NoChangeWhenDeletedNotEnabled covers: deleting a
// template that wasn't in the enabled list must not rewrite the config.
func TestTemplateDelete_NoChangeWhenDeletedNotEnabled(t *testing.T) {
	cfgM, tplM, _ := wireEnablement(t)
	saveStubTemplate(t, tplM, "kept.yaml")
	saveStubTemplate(t, tplM, "ignored.yaml")

	if _, err := cfgM.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"kept.yaml"},
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := tplM.DeleteTemplate("ignored.yaml"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	cfg, _ := cfgM.LoadUserConfig()
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"kept.yaml"}) {
		t.Errorf("EnabledTemplates should remain [kept.yaml], got %v", cfg.EnabledTemplates)
	}
}

// TestTemplateDelete_EmptyEnabledListIsLeftAlone covers the "opt-in not
// used" path: a profile with empty EnabledTemplates means "all enabled".
// Deleting a template must not introduce a populated list. The file
// should be byte-identical post-delete.
func TestTemplateDelete_EmptyEnabledListIsLeftAlone(t *testing.T) {
	cfgM, tplM, sys := wireEnablement(t)
	saveStubTemplate(t, tplM, "gone.yaml")

	// Warm load so any first-load sanitization is committed before we
	// stat the file for byte-identity.
	if _, err := cfgM.LoadUserConfig(); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	preBytes, err := sys.LoadFile("config/user.json")
	if err != nil {
		t.Fatalf("read pre: %v", err)
	}

	if err := tplM.DeleteTemplate("gone.yaml"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	postBytes, err := sys.LoadFile("config/user.json")
	if err != nil {
		t.Fatalf("read post: %v", err)
	}
	if preBytes != postBytes {
		t.Error("delete with empty enabled list must not rewrite config")
	}
}

// TestTemplateDelete_ClearsSelectedTemplate exercises the full chain: a
// template that's currently selected is deleted via the template
// manager → the observer fires reconcile → reconcile prunes the entry
// out of EnabledTemplates → normalizeSelectedTemplate clears
// SelectedTemplate + SelectedDataFile. End-to-end, no manual reset.
func TestTemplateDelete_ClearsSelectedTemplate(t *testing.T) {
	cfgM, tplM, _ := wireEnablement(t)
	saveStubTemplate(t, tplM, "active.yaml")
	saveStubTemplate(t, tplM, "other.yaml")
	if _, err := cfgM.UpdateUserConfig(map[string]any{
		"enabled_templates":  []string{"active.yaml", "other.yaml"},
		"selected_template":  "active.yaml",
		"selected_data_file": "alpha.meta.json",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := tplM.DeleteTemplate("active.yaml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	cfg, err := cfgM.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if cfg.SelectedTemplate != "" {
		t.Errorf("SelectedTemplate must be cleared after delete, got %q", cfg.SelectedTemplate)
	}
	if cfg.SelectedDataFile != "" {
		t.Errorf("SelectedDataFile must be cleared too, got %q", cfg.SelectedDataFile)
	}
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"other.yaml"}) {
		t.Errorf("EnabledTemplates = %v, want [other.yaml]", cfg.EnabledTemplates)
	}
}

// TestListEnabledTemplates_AfterRename covers the user's stated concern:
// after templates change on disk (we simulate a rename via delete+save),
// ListEnabledTemplates reflects the live folder state.
func TestListEnabledTemplates_AfterRename(t *testing.T) {
	cfgM, tplM, _ := wireEnablement(t)
	saveStubTemplate(t, tplM, "old-name.yaml")
	saveStubTemplate(t, tplM, "alpha.yaml")

	if _, err := cfgM.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"old-name.yaml", "alpha.yaml"},
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Simulate a rename: delete the old, save the new.
	if err := tplM.DeleteTemplate("old-name.yaml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	saveStubTemplate(t, tplM, "new-name.yaml")

	got, err := cfgM.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("ListEnabledTemplates: %v", err)
	}
	sort.Strings(got)
	// new-name.yaml is on disk but not in the enabled list — invisible
	// in the picker. alpha.yaml stays. old-name was pruned.
	want := []string{"alpha.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("post-rename listed = %v, want %v", got, want)
	}

	// Confirm new-name.yaml is on disk (template manager sees it),
	// just not enabled.
	all, _ := tplM.ListTemplates()
	sort.Strings(all)
	if !reflect.DeepEqual(all, []string{"alpha.yaml", "new-name.yaml"}) {
		t.Errorf("all templates on disk = %v, want [alpha.yaml new-name.yaml]", all)
	}
}
