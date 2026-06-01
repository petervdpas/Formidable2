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
// template / stats / facets mocks the formstats plugin needs, plus a
// RunChartOut capture so tests can assert the chart spec the plugin
// pushes via formidable.run.chart.
func managerWithStats(t *testing.T) (*Manager, string, *[]RunChartEvent, *[]RunOptionsEvent) {
	t.Helper()
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	charts := &[]RunChartEvent{}
	options := &[]RunOptionsEvent{}
	m := NewManager(ManagerDeps{
		PluginsDir:    pluginsDir,
		KV:            NewKV(kvTestFS{}, filepath.Join(pluginsDir, ".kv")),
		Template:      &mockTemplate{all: demoTemplate()},
		Stats:         &mockStats{},
		Facets:        &mockFacets{},
		StatObject:    &mockStatObject{},
		RunChartOut:   func(e RunChartEvent) { *charts = append(*charts, e) },
		RunOptionsOut: func(e RunOptionsEvent) { *options = append(*options, e) },
	})
	return m, pluginsDir, charts, options
}

// TestFormstats_DrawPushesChartSpec drives the REAL Manager.Run path
// against the shipped formstats files: it discovers the plugin, runs
// the `draw` form-button command with {template, object, shape}, and
// asserts the plugin STEERS the chart widget by pushing a spec through
// formidable.run.chart (not by returning it). Guards both the fn
// binding ("draw") and the run.chart contract the widget consumes.
func TestFormstats_DrawPushesChartSpec(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir, charts, _ := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	res, err := m.Run("formstats", "draw", map[string]any{
		"workspace": "storage",
		"template":  "demo.yaml",
		"object":    "by-status",
		"shape":     "bar",
	})
	if err != nil {
		t.Fatalf("Run(draw): %v", err)
	}
	if out, _ := res.Value.(map[string]any); out["ok"] != true {
		t.Fatalf("want ok=true, got %v", res.Value)
	}
	if len(*charts) != 1 {
		t.Fatalf("want 1 run.chart event, got %d", len(*charts))
	}
	spec := (*charts)[0].Spec
	if spec["type"] != "bar" {
		t.Fatalf("spec.type = %v, want bar", spec["type"])
	}
	if spec["result"] == nil {
		t.Fatal("spec.result missing")
	}
}

// TestFormstats_RefreshSteersShapeOptions drives the on-change command:
// it evaluates the picked object (rank-1 from the mock) and steers the
// "shape" field's options via formidable.run.options to bar/pie.
func TestFormstats_RefreshSteersShapeOptions(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir, _, options := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "refresh", map[string]any{
		"template": "demo.yaml",
		"object":   "by-status",
		"changed":  "object",
	})
	if err != nil {
		t.Fatalf("Run(refresh): %v", err)
	}
	if out, _ := res.Value.(map[string]any); out["ok"] != true {
		t.Fatalf("want ok=true, got %v", res.Value)
	}
	if len(*options) != 1 {
		t.Fatalf("want 1 run.options event, got %d", len(*options))
	}
	ev := (*options)[0]
	if ev.Field != "shape" {
		t.Fatalf("field = %q, want shape", ev.Field)
	}
	if len(ev.Options) != 2 {
		t.Fatalf("rank-1 should offer 2 shapes (bar/pie), got %d: %+v", len(ev.Options), ev.Options)
	}
}

// TestFormstats_RefreshOnOpenFillsObjectOptions drives the on-open kick
// (changed == "", no object yet): refresh must fill the "object" dropdown
// from the template's named statistical objects (formidable.statistical.list),
// so object keys are never hardcoded into form.json. The empty-label object
// falls back to its name.
func TestFormstats_RefreshOnOpenFillsObjectOptions(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir, _, options := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "refresh", map[string]any{
		"template": "demo.yaml",
		"object":   "",
		"changed":  "",
	})
	if err != nil {
		t.Fatalf("Run(refresh): %v", err)
	}
	if out, _ := res.Value.(map[string]any); out["ok"] != true {
		t.Fatalf("want ok=true, got %v", res.Value)
	}
	if len(*options) != 1 {
		t.Fatalf("want 1 run.options event (object), got %d: %+v", len(*options), *options)
	}
	ev := (*options)[0]
	if ev.Field != "object" {
		t.Fatalf("field = %q, want object", ev.Field)
	}
	// Catalog has 4 objects but one is a scaling, which must be excluded.
	if len(ev.Options) != 3 {
		t.Fatalf("want 3 object options (scaling excluded), got %d: %+v", len(ev.Options), ev.Options)
	}
	for _, o := range ev.Options {
		om, _ := o.(map[string]any)
		if om["value"] == "urgency-weight" {
			t.Fatalf("scaling object must not appear in the dropdown: %+v", om)
		}
	}
	first, _ := ev.Options[0].(map[string]any)
	if first["value"] != "by-status" || first["label"] != "By status" {
		t.Fatalf("first option = %+v, want by-status/By status", first)
	}
	second, _ := ev.Options[1].(map[string]any)
	if second["value"] != "raw" || second["label"] != "raw" {
		t.Fatalf("empty-label object must fall back to name; got %+v", second)
	}
}

// TestFormstats_DrawNoTemplateReturnsNotOk verifies the empty-ctx path:
// drawing with no selected template returns ok=false and pushes no
// chart.
func TestFormstats_DrawNoTemplateReturnsNotOk(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir, charts, _ := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "draw", map[string]any{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out, _ := res.Value.(map[string]any); out["ok"] != false {
		t.Fatalf("want ok=false, got %v", res.Value)
	}
	if len(*charts) != 0 {
		t.Fatalf("want no chart pushed, got %d", len(*charts))
	}
}

// TestFormstats_DrawNoObjectReturnsOkNoChart verifies the
// template-but-no-object path: ok=true, no chart pushed, so the widget
// keeps waiting for the user to pick an object.
func TestFormstats_DrawNoObjectReturnsOkNoChart(t *testing.T) {
	manifest, main := readFormstats(t)
	m, pluginsDir, charts, _ := managerWithStats(t)
	writePlugin(t, pluginsDir, "formstats", manifest, main)
	if err := m.Refresh(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	res, err := m.Run("formstats", "draw", map[string]any{"template": "demo.yaml"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out, _ := res.Value.(map[string]any); out["ok"] != true {
		t.Fatalf("want ok=true, got %v", res.Value)
	}
	if len(*charts) != 0 {
		t.Fatalf("want no chart pushed, got %d", len(*charts))
	}
}
