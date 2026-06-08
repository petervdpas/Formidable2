package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func newFormulaServiceForTest(t *testing.T) (*FormulaService, *storage.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})

	tpl := &template.Template{
		Name: "apps", Filename: "apps.yaml",
		Formulas: []template.Formula{
			{Key: "total", Type: "number", Expression: `F["a"] + F["b"]`},
		},
		Fields: []template.Field{
			{Key: "a", Type: "number"},
			{Key: "b", Type: "number"},
			{Key: "out", Type: "number"},
			{Key: "calc", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "live"},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	return NewFormulaService(tplM, stoM, exprM), stoM
}

func TestFormulaService_ComputeField_FromSavedRecord(t *testing.T) {
	svc, sto := newFormulaServiceForTest(t)
	if r := sto.SaveForm(context.Background(), "apps.yaml", "rec1", map[string]any{"a": 1.0, "b": 2.0}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}
	got, err := svc.ComputeField("apps.yaml", "rec1", "calc")
	if err != nil {
		t.Fatalf("ComputeField: %v", err)
	}
	if got.TargetKey != "out" {
		t.Errorf("TargetKey = %q, want out", got.TargetKey)
	}
	if got.Value != 3.0 {
		t.Errorf("Value = %#v, want 3", got.Value)
	}
}

func TestFormulaService_ComputeField_UnknownFieldErrors(t *testing.T) {
	svc, sto := newFormulaServiceForTest(t)
	if r := sto.SaveForm(context.Background(), "apps.yaml", "rec1", map[string]any{"a": 1.0}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}
	if _, err := svc.ComputeField("apps.yaml", "rec1", "ghost"); err == nil {
		t.Error("expected an error computing an unknown formula field")
	}
}

func TestFormulaService_ComputeField_MissingRecordErrors(t *testing.T) {
	svc, _ := newFormulaServiceForTest(t)
	if _, err := svc.ComputeField("apps.yaml", "nope", "calc"); err == nil {
		t.Error("expected an error computing against a missing record")
	}
}

// newFormulaServiceWithRawTemplate builds a FormulaService and writes templateYAML
// directly to disk (bypassing SaveTemplate's validation), so ComputeField can be
// driven against field shapes that template validation would otherwise reject.
// LoadTemplate does not re-validate, so these defensive guards are reachable.
func newFormulaServiceWithRawTemplate(t *testing.T, filename, templateYAML string) (*FormulaService, *storage.Manager) {
	t.Helper()
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})
	if err := sys.SaveFile(sys.JoinPath("templates", filename), templateYAML); err != nil {
		t.Fatalf("write template yaml: %v", err)
	}
	return NewFormulaService(tplM, stoM, exprM), stoM
}

func TestFormulaService_ComputeField_MissingTemplateErrors(t *testing.T) {
	svc, _ := newFormulaServiceForTest(t)
	// No nonexistent.yaml on disk: LoadTemplate fails before any field lookup.
	if _, err := svc.ComputeField("nonexistent.yaml", "rec1", "calc"); err == nil {
		t.Error("expected an error computing against a missing template")
	}
}

func TestFormulaService_ComputeField_EmptyBindingErrors(t *testing.T) {
	// A formula field with no formula_key/target_key binding. SaveTemplate would
	// reject this, so the YAML is written raw to reach the binding guard.
	yaml := `name: apps
filename: apps.yaml
fields:
  - key: calc
    type: formula
    trigger: live
`
	svc, sto := newFormulaServiceWithRawTemplate(t, "apps.yaml", yaml)
	if r := sto.SaveForm(context.Background(), "apps.yaml", "rec1", map[string]any{}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}
	if _, err := svc.ComputeField("apps.yaml", "rec1", "calc"); err == nil {
		t.Error("expected an error for a formula field with no source/target binding")
	}
}

func TestFormulaService_ComputeField_FormulaDidNotEvaluateErrors(t *testing.T) {
	// The formula field binds formula_key "total", but no such formula is
	// declared, so EvaluateFormulas yields no value for it. SaveTemplate would
	// reject the dangling reference, so the YAML is written raw.
	yaml := `name: apps
filename: apps.yaml
fields:
  - key: out
    type: number
  - key: calc
    type: formula
    formula_key: total
    target_key: out
    trigger: live
`
	svc, sto := newFormulaServiceWithRawTemplate(t, "apps.yaml", yaml)
	if r := sto.SaveForm(context.Background(), "apps.yaml", "rec1", map[string]any{}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}
	if _, err := svc.ComputeField("apps.yaml", "rec1", "calc"); err == nil {
		t.Error("expected an error when the bound formula does not evaluate")
	}
}
