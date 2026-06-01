package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TestFormulaFields_BecomeDatacoreFields proves a template formula is computed
// per record in the loader and is then an ordinary datacore field: numeric
// formulas aggregate, a formula referencing an earlier formula resolves in
// declared order, and a text formula over a facet works as a dimension.
func TestFormulaFields_BecomeDatacoreFields(t *testing.T) {
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
		Fields: []template.Field{
			{Key: "name", Type: "text"},
			{Key: "amount", Type: "number", UseInStatistics: true},
		},
		Facets: []template.Facet{
			{Key: "tier", Icon: "fa-flag", Options: []template.FacetOption{
				{Label: "GOLD", Color: "amber"}, {Label: "SILVER", Color: "blue"},
			}},
		},
		Formulas: []template.Formula{
			{Key: "marge", Type: "number", Expression: `F["amount"] * 0.5`},
			{Key: "weighted", Type: "number", Expression: `F["marge"] + 1`}, // refs earlier formula
			{Key: "kind", Type: "text", Expression: `F["tier"] == "GOLD" ? "premium" : "standard"`},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	saves := []struct {
		filename string
		data     map[string]any
	}{
		{"a.meta.json", map[string]any{"name": "A", "amount": float64(100),
			"meta": map[string]any{"facets": map[string]any{"tier": map[string]any{"set": true, "selected": "GOLD"}}}}},
		{"b.meta.json", map[string]any{"name": "B", "amount": float64(40),
			"meta": map[string]any{"facets": map[string]any{"tier": map[string]any{"set": true, "selected": "SILVER"}}}}},
	}
	for _, s := range saves {
		if r := stoM.SaveForm(context.Background(), "apps.yaml", s.filename, s.data); !r.Success {
			t.Fatalf("SaveForm %s: %s", s.filename, r.Error)
		}
	}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, exprM, "apps.yaml"))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}

	// marge = amount * 0.5 -> 50 + 20 = 70.
	if got := dt.View().Aggregate("marge").Sum; got != 70 {
		t.Errorf("sum(marge) = %v, want 70", got)
	}
	// weighted = marge + 1 -> 51 + 21 = 72; nonzero proves marge resolved first.
	if got := dt.View().Aggregate("weighted").Sum; got != 72 {
		t.Errorf("sum(weighted) = %v, want 72 (declared-order dependency)", got)
	}
	// kind over the tier facet, used as a categorical dimension.
	assertBuckets(t, "kind", dt.View().Distribution("kind"), map[string]int{"premium": 1, "standard": 1})
}

// TestEvalFormulas_SkipsBadExpression: a formula that fails to evaluate leaves
// no cell rather than aborting the record.
func TestEvalFormulas_SkipsBadExpression(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	_ = tplM.EnsureTemplateDirectory()
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})

	cells := evalFormulas(exprM, []template.Formula{
		{Key: "good", Type: "number", Expression: `F["amount"] * 2`},
		{Key: "bad", Type: "number", Expression: `F["amount" *`}, // malformed
	}, map[string]any{"amount": float64(10)})
	if cells["good"] != "20" {
		t.Errorf("good = %q, want 20", cells["good"])
	}
	if _, ok := cells["bad"]; ok {
		t.Errorf("bad formula should not produce a cell, got %q", cells["bad"])
	}
}
