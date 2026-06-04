package storage

import (
	"context"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fakeFormulaFiller returns fixed formula values, standing in for the real
// expression-engine harvester the composition root wires in.
type fakeFormulaFiller struct{ vals map[string]any }

func (f fakeFormulaFiller) FormulaValues(_ *template.Template, _ *Form) map[string]any {
	return f.vals
}

func formulaFieldTemplate(trigger string) *template.Template {
	return &template.Template{
		Formulas: []template.Formula{{Key: "total", Type: "number", Expression: `F["a"] + F["b"]`}},
		Fields: []template.Field{
			{Key: "a", Type: "number"},
			{Key: "b", Type: "number"},
			{Key: "out", Type: "number"},
			{Key: "calc", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: trigger},
		},
	}
}

func TestSaveForm_FormulaFieldWritesTargetOnSave(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	if err := tplM.SaveTemplate("basic.yaml", formulaFieldTemplate("save")); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	m.SetFormulaFiller(fakeFormulaFiller{vals: map[string]any{"total": 42.0}})

	if r := m.SaveForm(context.Background(), "basic.yaml", "rec1", map[string]any{"a": 1.0, "b": 2.0}); !r.Success {
		t.Fatalf("SaveForm failed: %s", r.Error)
	}
	got := m.LoadForm("basic.yaml", "rec1")
	if got == nil {
		t.Fatal("LoadForm returned nil")
	}
	if got.Data["out"] != 42.0 {
		t.Errorf("target out = %#v, want 42 (formula stamped on save)", got.Data["out"])
	}
}

func TestLoadForm_FormulaFieldWritesTargetOnLoad(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	if err := tplM.SaveTemplate("basic.yaml", formulaFieldTemplate("load")); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	m.SetFormulaFiller(fakeFormulaFiller{vals: map[string]any{"total": 42.0}})

	// Trigger is "load", so the save path leaves out at its default; the load path fills it.
	if r := m.SaveForm(context.Background(), "basic.yaml", "rec1", map[string]any{"a": 1.0, "b": 2.0}); !r.Success {
		t.Fatalf("SaveForm failed: %s", r.Error)
	}
	got := m.LoadForm("basic.yaml", "rec1")
	if got == nil {
		t.Fatal("LoadForm returned nil")
	}
	if got.Data["out"] != 42.0 {
		t.Errorf("target out = %#v, want 42 (formula stamped on load)", got.Data["out"])
	}
}

func TestLoadForm_FormulaSaveTriggerNotAppliedOnLoad(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	if err := tplM.SaveTemplate("basic.yaml", formulaFieldTemplate("save")); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	// Write a record verbatim (escape hatch does not apply formulas), seeding out=7.
	exact := Form{Data: map[string]any{"a": 1.0, "b": 2.0, "out": 7.0}}
	if r := m.SaveFormExact(context.Background(), "basic.yaml", "rec1", exact); !r.Success {
		t.Fatalf("SaveFormExact failed: %s", r.Error)
	}
	m.SetFormulaFiller(fakeFormulaFiller{vals: map[string]any{"total": 42.0}})

	got := m.LoadForm("basic.yaml", "rec1")
	if got == nil {
		t.Fatal("LoadForm returned nil")
	}
	if got.Data["out"] != 7.0 {
		t.Errorf("target out = %#v, want 7 (save-trigger must not fire on load)", got.Data["out"])
	}
}

func TestLoadForm_FormulaFieldNilFillerIsNoop(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	if err := tplM.SaveTemplate("basic.yaml", formulaFieldTemplate("load")); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	// No filler installed.
	if r := m.SaveForm(context.Background(), "basic.yaml", "rec1", map[string]any{"a": 1.0, "b": 2.0}); !r.Success {
		t.Fatalf("SaveForm failed: %s", r.Error)
	}
	if got := m.LoadForm("basic.yaml", "rec1"); got == nil {
		t.Fatal("LoadForm returned nil with no filler")
	}
}
