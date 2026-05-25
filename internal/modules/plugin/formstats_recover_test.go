package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

const formstatsDir = "../../../plugins/formstats"

// readFormstats returns the real plugin.json and main.lua, skipping the
// test if the plugin isn't present (it lives in the gitignored AppRoot
// plugins dir on a real install; the repo ships a copy under plugins/).
func readFormstats(t *testing.T) (manifest, main string) {
	t.Helper()
	man, err := os.ReadFile(filepath.Join(formstatsDir, "plugin.json"))
	if err != nil {
		t.Skipf("formstats plugin not present: %v", err)
	}
	lua, err := os.ReadFile(filepath.Join(formstatsDir, "main.lua"))
	if err != nil {
		t.Skipf("formstats main.lua not present: %v", err)
	}
	return string(man), string(lua)
}

// demoTemplate is a realistic template definition the mock template
// adapter serves to the plugin: two facets, a numeric / date / dropdown
// field, a free-text field (must be skipped), and a table with one
// column of each chartable kind.
func demoTemplate() map[string]map[string]any {
	return map[string]map[string]any{
		"demo.yaml": {
			"filename": "demo.yaml",
			"name":     "Demo",
			"facets": []any{
				map[string]any{"key": "priority"},
				map[string]any{"key": "stage"},
			},
			"fields": []any{
				map[string]any{"key": "amount", "type": "number", "label": "Amount"},
				map[string]any{"key": "due", "type": "date", "label": "Due"},
				map[string]any{"key": "status", "type": "dropdown", "label": "Status"},
				map[string]any{"key": "notes", "type": "textarea", "label": "Notes"},
				map[string]any{"key": "rows", "type": "table", "label": "Rows", "options": []any{
					map[string]any{"value": "qty", "type": "number", "label": "Qty"},
					map[string]any{"value": "when", "type": "date", "label": "When"},
					map[string]any{"value": "tag", "type": "string", "label": "Tag"},
				}},
			},
		},
	}
}

// managerWithStats mirrors newTestManager but additionally wires the
// template / stats / facets mocks the formstats plugin needs.
func managerWithStats(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m := NewManager(ManagerDeps{
		PluginsDir: pluginsDir,
		KV:         NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		Template:   &mockTemplate{all: demoTemplate()},
		Stats:      &mockStats{},
		Facets:     &mockFacets{},
		StatObject: &mockStatObject{},
	})
	return m, pluginsDir
}

// TestFormstats_DrawProducesChart drives the REAL Manager.Run path
// against the shipped formstats files: it discovers the plugin from
// disk, resolves the command's function via FnNameFor, and runs the
// `draw` command the chart widget calls with {template, object, shape}.
// Guards the fn binding (command id "draw" maps to global `draw` via
// "fn": "draw") and the chart-envelope return shape the widget renders.
func TestFormstats_DrawProducesChart(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	plugins := m.List()
	if len(plugins) != 1 || plugins[0].Manifest.ID != "formstats" {
		t.Fatalf("discovered %+v", plugins)
	}
	cmdID := plugins[0].Manifest.Commands[0].ID

	res, err := m.Run("formstats", cmdID, map[string]any{
		"workspace": "storage",
		"template":  "demo.yaml",
		"object":    "by-status",
		"shape":     "bar",
	})
	if err != nil {
		t.Fatalf("Run(%q): %v", cmdID, err)
	}

	out, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("return not a map: %T = %v", res.Value, res.Value)
	}
	chart, ok := out["chart"].(map[string]any)
	if !ok {
		t.Fatalf("chart not a map: %T = %v (full=%v)", out["chart"], out["chart"], out)
	}
	if chart["type"] != "bar" {
		t.Fatalf("chart.type = %v, want bar", chart["type"])
	}
	if chart["result"] == nil {
		t.Fatal("chart.result missing")
	}
}

// TestFormstats_DrawNoTemplateReturnsNotOk verifies the empty-ctx path:
// drawing with no selected template returns ok=false without erroring
// (the widget simply shows nothing).
func TestFormstats_DrawNoTemplateReturnsNotOk(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "draw", map[string]any{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, _ := res.Value.(map[string]any)
	if out["ok"] != false {
		t.Fatalf("want ok=false, got %v", out["ok"])
	}
}

// TestFormstats_DrawNoObjectReturnsOkNoChart verifies the
// template-but-no-object path: ok=true with no chart, so the widget
// waits for the user to pick an object.
func TestFormstats_DrawNoObjectReturnsOkNoChart(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "draw", map[string]any{"template": "demo.yaml"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, _ := res.Value.(map[string]any)
	if out["ok"] != true {
		t.Fatalf("want ok=true, got %v", out["ok"])
	}
	if out["chart"] != nil {
		t.Fatalf("want no chart, got %v", out["chart"])
	}
}
