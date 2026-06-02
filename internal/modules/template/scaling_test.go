package template

import "testing"

// TestLoadTemplate_MigratesLegacyScaling proves the lift runs on the editor's
// load path (not just on save/validate): a YAML with a legacy
// statistics[].scaling is migrated to t.Scalings on load, removed from
// statistics, and flagged for resave so the new shape persists.
func TestLoadTemplate_MigratesLegacyScaling(t *testing.T) {
	m, sys, _ := newTestManager(t)
	raw := `name: Apps
fields:
  - {key: name, type: text}
statistics:
  - name: by-name
    dsl: count() by F["name"]
  - name: fcdm-urgency
    label: FCDM
    scaling:
      source: {kind: facet, key: fcdm}
      weights:
        - {label: NIET, factor: 10}
      default: 1
`
	if err := sys.SaveFile("templates/apps.yaml", raw); err != nil {
		t.Fatal(err)
	}
	tpl, err := m.LoadTemplate("apps.yaml")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if len(tpl.Statistics) != 1 || tpl.Statistics[0].Name != "by-name" {
		t.Fatalf("statistics should keep only the DSL object, got %+v", tpl.Statistics)
	}
	if len(tpl.Scalings) != 1 || tpl.Scalings[0].Name != "fcdm-urgency" || tpl.Scalings[0].Source.Key != "fcdm" {
		t.Fatalf("scaling not lifted on load, got %+v", tpl.Scalings)
	}
	if !tpl.NeedsResave {
		t.Error("migrated template should be flagged NeedsResave")
	}
}

// TestMigrateLegacyScalings_LiftsFromStatistics proves a scaling stored under
// the legacy statistics[].scaling shape is moved to t.Scalings (carrying its
// Name/Label) and removed from t.Statistics, while a plain DSL statistic stays.
func TestMigrateLegacyScalings_LiftsFromStatistics(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "id", Type: "guid"}},
		Statistics: []Statistic{
			{Name: "by-status", Label: "By status", DSL: `count() by F["status"]`},
			{Name: "fcdm-urgency", Label: "FCDM", Scaling: &StatScaling{
				Source:  StatSource{Kind: "facet", Key: "fcdm"},
				Weights: []StatWeightEntry{{Label: "NIET", Factor: 10}},
				Default: 1,
			}},
		},
	}
	Normalize(tpl)

	if len(tpl.Statistics) != 1 || tpl.Statistics[0].Name != "by-status" {
		t.Fatalf("statistics should keep only the DSL object, got %+v", tpl.Statistics)
	}
	if len(tpl.Scalings) != 1 {
		t.Fatalf("want 1 lifted scaling, got %d: %+v", len(tpl.Scalings), tpl.Scalings)
	}
	sc := tpl.Scalings[0]
	if sc.Name != "fcdm-urgency" || sc.Label != "FCDM" || sc.Source.Key != "fcdm" || sc.Default != 1 {
		t.Errorf("lifted scaling wrong: %+v", sc)
	}
	if len(sc.Weights) != 1 || sc.Weights[0].Label != "NIET" || sc.Weights[0].Factor != 10 {
		t.Errorf("weights not carried: %+v", sc.Weights)
	}
}

// TestNormalizeScalings_TrimsDropsAndDedups drops empties and dedupes by name.
func TestNormalizeScalings_TrimsDropsAndDedups(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "id", Type: "guid"}},
		Scalings: []Scaling{
			{Name: "  weight ", Label: " W ", Source: StatSource{Kind: "facet", Key: "fcdm"}},
			{Name: "weight", Source: StatSource{Kind: "facet", Key: "fcdm"}}, // dup name -> dropped
			{Name: "nosource", Source: StatSource{Kind: "facet", Key: "  "}}, // empty source -> dropped
			{Name: "", Source: StatSource{Kind: "facet", Key: "fcdm"}},       // empty name -> dropped
		},
	}
	Normalize(tpl)
	if len(tpl.Scalings) != 1 {
		t.Fatalf("kept %d scalings, want 1: %+v", len(tpl.Scalings), tpl.Scalings)
	}
	if s := tpl.Scalings[0]; s.Name != "weight" || s.Label != "W" {
		t.Errorf("not trimmed: %+v", s)
	}
}

// TestScalingsErrors_FlagsStructuralProblems checks the validation paths.
func TestScalingsErrors_FlagsStructuralProblems(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "amount", Type: "number"}},
		Scalings: []Scaling{
			{Name: "Bad Name", Source: StatSource{Kind: "facet", Key: "fcdm"}},   // invalid name
			{Name: "nosrc", Source: StatSource{Kind: "facet", Key: ""}},          // missing source
			{Name: "tbl", Source: StatSource{Kind: "field", Key: "rows", Column: "qty"}}, // table column
			{Name: "badkind", Source: StatSource{Kind: "weird", Key: "x"}},       // bad kind
			{Name: "dup", Source: StatSource{Kind: "facet", Key: "fcdm"}},
			{Name: "dup", Source: StatSource{Kind: "facet", Key: "fcdm"}}, // duplicate
		},
	}
	errs := scalingsErrors(tpl)
	for _, want := range []string{
		"invalid-scaling-name", "scaling-missing-source",
		"invalid-scaling-source", "duplicate-scaling-name",
	} {
		if !hasErr(errs, want) {
			t.Errorf("expected a %q error, got %+v", want, errs)
		}
	}
}

// TestScalingsErrors_CleanCatalogPasses: a name matching a field key is fine
// (S["x"] and F["x"] are separate namespaces).
func TestScalingsErrors_CleanCatalogPasses(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "amount", Type: "number"}},
		Scalings: []Scaling{
			{Name: "amount", Source: StatSource{Kind: "field", Key: "amount"}},
			{Name: "fcdm-urgency", Source: StatSource{Kind: "facet", Key: "fcdm"}},
		},
	}
	if errs := scalingsErrors(tpl); len(errs) != 0 {
		t.Errorf("clean catalog should pass, got %+v", errs)
	}
}
