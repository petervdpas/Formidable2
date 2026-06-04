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
