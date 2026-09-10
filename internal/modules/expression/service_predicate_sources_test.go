package expression

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/expression/builder"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestBuilderPredicateSources_PredicateableFieldsThenFormulas(t *testing.T) {
	s := NewService(nil)
	got := s.BuilderPredicateSources(
		[]template.Field{
			{Key: "check", Type: "boolean", Label: "Checked"},
			{Key: "title", Type: "text", Label: "Title"}, // no rule kind -> excluded
			{Key: "size", Type: "dropdown"},              // no label -> falls back to key
		},
		[]template.Formula{
			{Key: "margin", Type: "number", Label: "Margin"},
			{Key: "summary", Type: "text"}, // no rule kind -> excluded
			{Key: "untyped"},               // blank type means number -> included
		},
	)

	if len(got) != 4 {
		t.Fatalf("got %d sources, want 4: %+v", len(got), got)
	}
	if got[0].Key != "check" || got[0].Label != "Checked" || got[0].Type != "boolean" || got[0].Group != "field" {
		t.Errorf("source[0] = %+v", got[0])
	}
	if got[1].Key != "size" || got[1].Label != "size" || got[1].Group != "field" {
		t.Errorf("source[1] = %+v (label should fall back to key)", got[1])
	}
	if got[2].Key != "margin" || got[2].Label != "Margin" || got[2].Type != "number" || got[2].Group != "formula" {
		t.Errorf("source[2] = %+v", got[2])
	}
	if got[3].Key != "untyped" || got[3].Group != "formula" {
		t.Errorf("source[3] = %+v (an untyped formula is a number formula)", got[3])
	}
	for _, o := range got {
		if o.Key == "title" || o.Key == "summary" {
			t.Errorf("text source without a rule kind leaked in: %+v", got)
		}
	}
}

// The picker's Type is what the editor hands back to ask for a default
// Predicate, so every offered source must actually yield one.
func TestBuilderPredicateSources_EveryEntryYieldsADefaultPredicate(t *testing.T) {
	s := NewService(nil)
	fields := []template.Field{
		{Key: "check", Type: "boolean"},
		{Key: "size", Type: "dropdown"},
		{Key: "score", Type: "number"},
		{Key: "due", Type: "date"},
		{Key: "fac", Type: "facet"},
	}
	formulas := []template.Formula{
		{Key: "margin", Type: "number"},
		{Key: "at-risk", Type: "bool"},
		{Key: "deadline", Type: "date"},
		{Key: "untyped"},
	}
	for _, o := range s.BuilderPredicateSources(fields, formulas) {
		var err error
		switch o.Group {
		case "field":
			_, err = s.BuilderDefaultPredicate(o.Type, o.Key)
		case "formula":
			_, err = s.BuilderDefaultPredicateForFormula(o.Type, o.Key)
		default:
			t.Fatalf("unknown group %q on %+v", o.Group, o)
		}
		if err != nil {
			t.Errorf("offered source %+v cannot produce a predicate: %v", o, err)
		}
	}
}

func TestBuilderPredicateSources_EmptyInputs(t *testing.T) {
	s := NewService(nil)
	got := s.BuilderPredicateSources(nil, nil)
	if got == nil {
		t.Error("want an empty slice, not nil, so the picker renders an empty list")
	}
	if len(got) != 0 {
		t.Errorf("got %d sources, want 0", len(got))
	}
}

// Compile must accept exactly what the pickers offered: a rule built from the
// source list and the ref list round-trips without the caller reconciling them.
func TestBuilderCompile_AcceptsFormulaPredicateFromSources(t *testing.T) {
	s := NewService(nil)

	p, err := s.BuilderDefaultPredicateForFormula("number", "margin")
	if err != nil {
		t.Fatalf("default predicate: %v", err)
	}
	cfg := s.BuilderDefaultConfig()
	rule := s.BuilderDefaultRule()
	rule.ID = "r1"
	rule.Predicates = append(rule.Predicates, p)
	cfg.Rules = append(cfg.Rules, rule)

	src, err := s.BuilderCompile(cfg, nil, []builder.FormulaRef{{Key: "margin", Type: "number"}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	want := `F["margin"] == 0 ? {} : {}`
	if src != want {
		t.Errorf("got  %s\nwant %s", src, want)
	}
}
