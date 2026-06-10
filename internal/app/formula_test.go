package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/index"
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

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, exprM, nil, "apps.yaml", false))
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

// TestFormula_ResolvesInSidebarExpression is the end-to-end for "formulas in
// the expression engine": a form save harvests the formula value into the
// index's expression context, and the sidebar_expression reads F["formula"].
func TestFormula_ResolvesInSidebarExpression(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)

	idxM, err := index.NewManager(filepath.Join(root, "index", "test.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(root)
	ehM.SetFormulaEvaluator(formulaHarvester{ev: exprM})
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	// reference = code, plus "-N" only when the story number is present.
	tpl := &template.Template{
		Name: "audits", Filename: "audits.yaml", ItemField: "code",
		SidebarExpression: `{text: F["reference"]}`,
		Fields: []template.Field{
			{Key: "code", Type: "text"},
			{Key: "story", Type: "number"},
		},
		Formulas: []template.Formula{
			{Key: "reference", Type: "text", Expression: `str(F["code"]) + (F["story"] > 0 ? "-" + str(F["story"]) : "")`},
		},
	}
	if err := tplM.SaveTemplate("audits.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	saves := []map[string]any{
		{"code": "CH.02", "story": float64(5)},
		{"code": "CH.04", "story": float64(0)},
	}
	for i, d := range saves {
		name := []string{"a.meta.json", "b.meta.json"}[i]
		if r := stoM.SaveForm(context.Background(), "audits.yaml", name, d); !r.Success {
			t.Fatalf("SaveForm %s: %s", name, r.Error)
		}
	}

	results, err := exprM.EvaluateList("audits.yaml")
	if err != nil {
		t.Fatalf("EvaluateList: %v", err)
	}
	got := map[string]string{}
	for _, r := range results {
		got[r.Filename] = r.Text
	}
	if got["a.meta.json"] != "CH.02-5" {
		t.Errorf("a sidebar = %q, want CH.02-5", got["a.meta.json"])
	}
	if got["b.meta.json"] != "CH.04" {
		t.Errorf("b sidebar = %q, want CH.04 (separator dropped when story is 0)", got["b.meta.json"])
	}
}

// TestFormula_ResolvesInSidebarOutcomePart reproduces the studio case: the
// sidebar builder concatenates a field part and a formula part, so the compiled
// expression is str(F["code"]) + str(F["sb-formula"]). The number field flows
// through the formula and the sub-label reads "CH.04 - 99642".
func TestFormula_ResolvesInSidebarOutcomePart(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	idxM, err := index.NewManager(filepath.Join(root, "index", "test.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(root)
	ehM.SetFormulaEvaluator(formulaHarvester{ev: exprM})
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	tpl := &template.Template{
		Name: "audits", Filename: "audits.yaml", ItemField: "code",
		// The builder's multi-part output: a field part + a formula part.
		SidebarExpression: `{text: str(F["code"]) + str(F["sb-formula"])}`,
		Fields: []template.Field{
			{Key: "code", Type: "text", ExpressionItem: true},
			{Key: "story", Type: "number"},
		},
		Formulas: []template.Formula{
			{Key: "sb-formula", Type: "text", Expression: `F["story"] >= 0 ? " - " + str(F["story"]) : ""`},
		},
	}
	if err := tplM.SaveTemplate("audits.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if r := stoM.SaveForm(context.Background(), "audits.yaml", "x.meta.json",
		map[string]any{"code": "CH.04", "story": float64(99642)}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	results, err := exprM.EvaluateList("audits.yaml")
	if err != nil {
		t.Fatalf("EvaluateList: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Text != "CH.04 - 99642" {
		t.Errorf("sidebar = %q, want %q", results[0].Text, "CH.04 - 99642")
	}
}

// TestFormula_SidebarRuleOnFacetFieldShowsFormula is the real (unhappy) path:
// the sidebar rule cascade keys on a FACET field (value in meta.facets, not
// f.Data) and only the matching rule's outcome carries the formula. The facet
// field must resolve in the harvested context or no rule matches and the
// default outcome (no formula) shows.
func TestFormula_SidebarRuleOnFacetFieldShowsFormula(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	idxM, err := index.NewManager(filepath.Join(root, "index", "test.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(root)
	ehM.SetFormulaEvaluator(formulaHarvester{ev: exprM})
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	tpl := &template.Template{
		Name: "audits", Filename: "audits.yaml", ItemField: "code",
		// Rule on the facet field; only the matching outcome has the formula.
		SidebarExpression: `F["wstatus"] == "OPEN" ? {text: str(F["code"]) + str(F["sb-formula"])} : {text: F["code"]}`,
		Facets: []template.Facet{
			{Key: "status", Icon: "fa-bell", Options: []template.FacetOption{
				{Label: "OPEN", Color: "red"}, {Label: "DONE", Color: "green"},
			}},
		},
		Fields: []template.Field{
			{Key: "code", Type: "text", ExpressionItem: true},
			{Key: "wstatus", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN", ExpressionItem: true},
			{Key: "story", Type: "number"},
		},
		Formulas: []template.Formula{
			{Key: "sb-formula", Type: "text", Expression: `F["story"] >= 0 ? " - " + str(F["story"]) : ""`},
		},
	}
	if err := tplM.SaveTemplate("audits.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if r := stoM.SaveForm(context.Background(), "audits.yaml", "x.meta.json", map[string]any{
		"code":  "CH.04",
		"story": float64(99641),
		"meta":  map[string]any{"facets": map[string]any{"status": map[string]any{"set": true, "selected": "OPEN"}}},
	}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	results, err := exprM.EvaluateList("audits.yaml")
	if err != nil {
		t.Fatalf("EvaluateList: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Text != "CH.04 - 99641" {
		t.Errorf("sidebar = %q, want %q (facet-keyed rule must match so the formula shows)", results[0].Text, "CH.04 - 99641")
	}
}

// TestFormula_SidebarViaEvaluateListMany covers the StorageWorkspace path
// (EvaluateListMany -> ExtendedLoadForm), the one that was actually broken: it
// must read the harvested context from the index (single source) so the formula
// resolves, not re-harvest on disk where formulas are absent. Facet-keyed rule
// + formula outcome, the real shape.
func TestFormula_SidebarViaEvaluateListMany(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	idxM, err := index.NewManager(filepath.Join(root, "index", "test.db"))
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })

	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(root)
	ehM.SetFormulaEvaluator(formulaHarvester{ev: exprM})
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	tpl := &template.Template{
		Name: "audits", Filename: "audits.yaml", ItemField: "code",
		SidebarExpression: `F["wstatus"] == "OPEN" ? {text: str(F["code"]) + str(F["sb-formula"])} : {text: F["code"]}`,
		Facets: []template.Facet{
			{Key: "status", Icon: "fa-bell", Options: []template.FacetOption{
				{Label: "OPEN", Color: "red"}, {Label: "DONE", Color: "green"},
			}},
		},
		Fields: []template.Field{
			{Key: "code", Type: "text", ExpressionItem: true},
			{Key: "wstatus", Type: "facet", FacetKey: "status", Format: "radio", Default: "OPEN", ExpressionItem: true},
			{Key: "story", Type: "number"},
		},
		Formulas: []template.Formula{
			{Key: "sb-formula", Type: "text", Expression: `F["story"] >= 0 ? " - " + str(F["story"]) : ""`},
		},
	}
	if err := tplM.SaveTemplate("audits.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if r := stoM.SaveForm(context.Background(), "audits.yaml", "x.meta.json", map[string]any{
		"code":  "CH.04",
		"story": float64(99641),
		"meta":  map[string]any{"facets": map[string]any{"status": map[string]any{"set": true, "selected": "OPEN"}}},
	}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	// The StorageWorkspace path: EvaluateListMany over explicit filenames.
	results, err := exprM.EvaluateListMany("audits.yaml", []string{"x.meta.json"})
	if err != nil {
		t.Fatalf("EvaluateListMany: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Text != "CH.04 - 99641" {
		t.Errorf("sub-label = %q, want %q (per-record path must read harvested formula from the index)", results[0].Text, "CH.04 - 99641")
	}
}

// TestFormula_DiskFallbackComputesFormulas covers the no-index path: with no
// reader wired (index broken/absent), ExtendedLoadForm reads from disk, and the
// formula filler must still produce the formula value so the sidebar label is
// complete rather than silently dropping the formula.
func TestFormula_DiskFallbackComputesFormulas(t *testing.T) {
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
	// No SetReader: the disk fallback is exercised. Install the formula filler.
	stoM.SetFormulaFiller(formulaHarvester{ev: exprM})

	tpl := &template.Template{
		Name: "audits", Filename: "audits.yaml", ItemField: "code",
		SidebarExpression: `{text: str(F["code"]) + str(F["sb-formula"])}`,
		Fields: []template.Field{
			{Key: "code", Type: "text", ExpressionItem: true},
			{Key: "story", Type: "number"},
		},
		Formulas: []template.Formula{
			{Key: "sb-formula", Type: "text", Expression: `F["story"] >= 0 ? " - " + str(F["story"]) : ""`},
		},
	}
	if err := tplM.SaveTemplate("audits.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if r := stoM.SaveForm(context.Background(), "audits.yaml", "x.meta.json",
		map[string]any{"code": "CH.04", "story": float64(99641)}); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	results, err := exprM.EvaluateListMany("audits.yaml", []string{"x.meta.json"})
	if err != nil {
		t.Fatalf("EvaluateListMany: %v", err)
	}
	if len(results) != 1 || results[0].Text != "CH.04 - 99641" {
		t.Errorf("disk-fallback sub-label = %q, want %q", results, "CH.04 - 99641")
	}
}

// TestApplyFormulas_WritesCoercedCellsSkipsBad checks the app-side write path:
// a good formula lands as a string cell on the record (coerced by type), a
// malformed one leaves no cell, and a typed number ctx value drives arithmetic.
func TestApplyFormulas_WritesCoercedCellsSkipsBad(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	_ = tplM.EnsureTemplateDirectory()
	sfrM := sfr.NewManager(sys, log)
	stoM := storage.NewManager(sys, sfrM, tplM, "storage", log)
	exprM := expression.NewManager(expressionTemplateAdapter{tpl: tplM}, expressionStorageAdapter{sto: stoM})

	tpl := &template.Template{
		Fields: []template.Field{{Key: "amount", Type: "number"}},
		Formulas: []template.Formula{
			{Key: "good", Type: "number", Expression: `F["amount"] * 2`},
			{Key: "bad", Type: "number", Expression: `F["amount" *`}, // malformed
		},
	}
	f := &storage.Form{Data: map[string]any{"amount": float64(10)}}
	rec := datacore.Record{ID: "x"}
	applyFormulas(exprM, tpl, f, &rec)

	if rec.Fields["good"] != "20" {
		t.Errorf("good cell = %q, want 20", rec.Fields["good"])
	}
	if _, ok := rec.Fields["bad"]; ok {
		t.Errorf("bad formula should not produce a cell, got %q", rec.Fields["bad"])
	}
}

// TestCoerceFormula_TypeBranches pins coerceFormula's per-type coercion: numbers
// format without scientific notation, ints/int64 round-trip, a non-numeric value
// under "number" falls back to dcText, bools render as the literal strings, a
// non-bool under "bool" falls back, and the default branch stringifies anything.
func TestCoerceFormula_TypeBranches(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		typ  string
		want string
	}{
		{"number float64", float64(12.5), "number", "12.5"},
		{"number large no sci notation", float64(99642), "number", "99642"},
		{"number int", 7, "number", "7"},
		{"number int64", int64(9), "number", "9"},
		{"number non-numeric fallback", "n/a", "number", "n/a"},
		{"bool true", true, "bool", "true"},
		{"bool false", false, "bool", "false"},
		{"bool non-bool fallback", float64(1), "bool", "1"},
		{"text string", "hello", "text", "hello"},
		{"text from float64", float64(3), "text", "3"},
		{"default unknown type stringifies", float64(2), "weird", "2"},
		{"text nil empty", nil, "text", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := coerceFormula(c.raw, c.typ); got != c.want {
				t.Errorf("coerceFormula(%#v, %q) = %q, want %q", c.raw, c.typ, got, c.want)
			}
		})
	}
}
