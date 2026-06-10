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

// TestScaleValues_FactorLookup is the unit for the per-record factor: a facet
// source resolves through the selected option's weight, a dropdown/radio field
// source through its value, and an unlisted/unset value falls to the default.
func TestScaleValues_FactorLookup(t *testing.T) {
	tpl := &template.Template{
		Fields: []template.Field{{Key: "size", Type: "dropdown"}},
		Scalings: []template.Scaling{
			{Name: "urgency", Source: template.StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []template.StatWeightEntry{{Label: "NIET", Factor: 10}, {Label: "AANWEZIG", Factor: 1}}, Default: 3},
			{Name: "bulk", Source: template.StatSource{Kind: "field", Key: "size"},
				Weights: []template.StatWeightEntry{{Label: "L", Factor: 5}}, Default: 2},
		},
	}

	cases := []struct {
		name     string
		facet    string
		facetSet bool
		size     any
		urgency  float64
		bulk     float64
	}{
		{"listed facet + listed field", "NIET", true, "L", 10, 5},
		{"other facet -> default; unset field -> default", "AANWEZIG", true, nil, 1, 2},
		{"unlisted facet value -> default", "ONBEKEND", true, "M", 3, 2}, // M unlisted -> bulk default
		{"facet not set -> default", "", false, "L", 3, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &storage.Form{
				Meta: storage.FormMeta{Facets: map[string]storage.FacetState{"fcdm": {Set: c.facetSet, Selected: c.facet}}},
				Data: map[string]any{},
			}
			if c.size != nil {
				f.Data["size"] = c.size
			}
			got := scaleValues(tpl, f)
			if got["urgency"] != c.urgency {
				t.Errorf("urgency = %v, want %v", got["urgency"], c.urgency)
			}
			if got["bulk"] != c.bulk {
				t.Errorf("bulk = %v, want %v", got["bulk"], c.bulk)
			}
		})
	}
}

// TestScaling_SurfacedInStatCatalog proves a top-level scaling is still
// exposed to the Statistical Engine as a StatObject of kind scaling, so the DSL
// scale "<name>" clause keeps resolving after scalings left the Statistics tab.
func TestScaling_SurfacedInStatCatalog(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tplM := template.NewManager(sys, "templates", log)
	if err := tplM.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}

	tpl := &template.Template{
		Name: "apps", Filename: "apps.yaml",
		Fields: []template.Field{{Key: "name", Type: "text"}},
		Statistics: []template.Statistic{
			{Name: "weighted", DSL: `count() by F["name"] scale "urgency"`},
		},
		Scalings: []template.Scaling{
			{Name: "urgency", Source: template.StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []template.StatWeightEntry{{Label: "NIET", Factor: 10}}, Default: 1},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	objs, err := statTemplateSource{tpl: tplM}.ListStatistics("apps.yaml")
	if err != nil {
		t.Fatalf("ListStatistics: %v", err)
	}
	var sawDSL, sawScaling bool
	for _, o := range objs {
		if o.Name == "weighted" && o.DSL != "" {
			sawDSL = true
		}
		if o.Name == "urgency" && o.Scaling != nil {
			sawScaling = true
		}
	}
	if !sawDSL {
		t.Errorf("DSL statistic missing from catalog: %+v", objs)
	}
	if !sawScaling {
		t.Errorf("top-level scaling not surfaced as a kind-scaling object: %+v", objs)
	}
}

// TestScaling_ResolvesInFormulaAndStatistics proves S["name"] is usable inside a
// formula, and the formula then aggregates as a datacore field: a scaling
// weights a facet, a number formula multiplies a field by that weight, and
// sum(formula) over all forms equals the hand-computed total.
func TestScaling_ResolvesInFormulaAndStatistics(t *testing.T) {
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
			{Key: "dekking", Type: "number", UseInStatistics: true},
		},
		Facets: []template.Facet{
			{Key: "fcdm", Icon: "fa-flag", Options: []template.FacetOption{
				{Label: "NIET", Color: "red"}, {Label: "AANWEZIG", Color: "green"},
			}},
		},
		Scalings: []template.Scaling{
			{Name: "urgency", Source: template.StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []template.StatWeightEntry{{Label: "NIET", Factor: 10}, {Label: "AANWEZIG", Factor: 1}}, Default: 1},
		},
		Formulas: []template.Formula{
			{Key: "heavy", Type: "number", Expression: `F["dekking"] * S["urgency"]`},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	saves := []map[string]any{
		{"dekking": float64(5), "meta": map[string]any{"facets": map[string]any{"fcdm": map[string]any{"set": true, "selected": "NIET"}}}},
		{"dekking": float64(5), "meta": map[string]any{"facets": map[string]any{"fcdm": map[string]any{"set": true, "selected": "AANWEZIG"}}}},
	}
	for i, d := range saves {
		name := []string{"a.meta.json", "b.meta.json"}[i]
		if r := stoM.SaveForm(context.Background(), "apps.yaml", name, d); !r.Success {
			t.Fatalf("SaveForm %s: %s", name, r.Error)
		}
	}

	dt, err := datacore.Build(newDatacoreLoaderAdapter(tplM, stoM, exprM, nil, "apps.yaml", false))
	if err != nil {
		t.Fatalf("datacore.Build: %v", err)
	}
	// heavy = dekking * urgency -> 5*10 + 5*1 = 55.
	if got := dt.View().Aggregate("heavy").Sum; got != 55 {
		t.Errorf("sum(heavy) = %v, want 55 (5*10 + 5*1)", got)
	}
}

// TestScaling_ResolvesInSidebarExpression is the end-to-end for "scalings in the
// expression engine": the index harvest folds the S map into the expression
// context, and a sidebar_expression reads S["name"]. It also asserts the raw
// facet value resolves as F["facet-key"].
func TestScaling_ResolvesInSidebarExpression(t *testing.T) {
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
	ehM.SetScaleEvaluator(formulaHarvester{ev: exprM})
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	tpl := &template.Template{
		Name: "apps", Filename: "apps.yaml", ItemField: "name",
		SidebarExpression: `{text: (S["urgency"] >= 5 ? "high" : "low") + ":" + F["fcdm"]}`,
		Fields:            []template.Field{{Key: "name", Type: "text"}},
		Facets: []template.Facet{
			{Key: "fcdm", Icon: "fa-flag", Options: []template.FacetOption{
				{Label: "NIET", Color: "red"}, {Label: "AANWEZIG", Color: "green"},
			}},
		},
		Scalings: []template.Scaling{
			{Name: "urgency", Source: template.StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []template.StatWeightEntry{{Label: "NIET", Factor: 10}, {Label: "AANWEZIG", Factor: 1}}, Default: 1},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	saves := []map[string]any{
		{"name": "A", "meta": map[string]any{"facets": map[string]any{"fcdm": map[string]any{"set": true, "selected": "NIET"}}}},
		{"name": "B", "meta": map[string]any{"facets": map[string]any{"fcdm": map[string]any{"set": true, "selected": "AANWEZIG"}}}},
	}
	for i, d := range saves {
		name := []string{"a.meta.json", "b.meta.json"}[i]
		if r := stoM.SaveForm(context.Background(), "apps.yaml", name, d); !r.Success {
			t.Fatalf("SaveForm %s: %s", name, r.Error)
		}
	}

	results, err := exprM.EvaluateList("apps.yaml")
	if err != nil {
		t.Fatalf("EvaluateList: %v", err)
	}
	got := map[string]string{}
	for _, r := range results {
		got[r.Filename] = r.Text
	}
	if got["a.meta.json"] != "high:NIET" {
		t.Errorf("a sidebar = %q, want high:NIET", got["a.meta.json"])
	}
	if got["b.meta.json"] != "low:AANWEZIG" {
		t.Errorf("b sidebar = %q, want low:AANWEZIG", got["b.meta.json"])
	}
}

// TestScaling_DiskFallbackComputesS proves the disk-read path (no index reader,
// only the ScaleFiller installed) still resolves S["name"] in a sidebar
// expression, so a scaling works even when the index is not the source.
func TestScaling_DiskFallbackComputesS(t *testing.T) {
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
	// No index reader; only the disk-path scaling calculator.
	stoM.SetScaleFiller(formulaHarvester{ev: exprM})

	tpl := &template.Template{
		Name: "apps", Filename: "apps.yaml", ItemField: "name",
		SidebarExpression: `{text: S["urgency"] >= 5 ? "high" : "low"}`,
		Fields:            []template.Field{{Key: "name", Type: "text"}},
		Facets: []template.Facet{
			{Key: "fcdm", Icon: "fa-flag", Options: []template.FacetOption{{Label: "NIET", Color: "red"}}},
		},
		Scalings: []template.Scaling{
			{Name: "urgency", Source: template.StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []template.StatWeightEntry{{Label: "NIET", Factor: 10}}, Default: 1},
		},
	}
	if err := tplM.SaveTemplate("apps.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	d := map[string]any{"name": "A", "meta": map[string]any{"facets": map[string]any{"fcdm": map[string]any{"set": true, "selected": "NIET"}}}}
	if r := stoM.SaveForm(context.Background(), "apps.yaml", "a.meta.json", d); !r.Success {
		t.Fatalf("SaveForm: %s", r.Error)
	}

	res, err := exprM.EvaluateListOne("apps.yaml", "a.meta.json")
	if err != nil {
		t.Fatalf("EvaluateListOne: %v", err)
	}
	if res.Text != "high" {
		t.Errorf("disk-fallback sidebar = %q, want high (S resolved on disk path)", res.Text)
	}
}
