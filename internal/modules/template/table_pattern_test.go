package template

import (
	"reflect"
	"testing"
)

func col(key, label string) map[string]any {
	return map[string]any{"value": key, "label": label}
}

func colKeys(opts []any) []string {
	out := []string{}
	for _, o := range opts {
		m, _ := o.(map[string]any)
		v, _ := m["value"].(string)
		out = append(out, v)
	}
	return out
}

var lineageKeys = []string{"source_entity", "source_attribute", "target_entity", "target_attribute", "example_value", "transaction_rule"}

func TestTablePatterns_RegularFirstThenDataLineage(t *testing.T) {
	ps := TablePatterns()
	if len(ps) < 2 || ps[0].ID != TablePatternRegular || ps[0].Shape != nil {
		t.Fatalf("first pattern = %+v, want regular without a shape", ps[0])
	}
	var lin *TablePatternDescriptor
	for i := range ps {
		if ps[i].ID == TablePatternDataLineage {
			lin = &ps[i]
		}
		if ps[i].LabelKey == "" {
			t.Errorf("pattern %q has no label key", ps[i].ID)
		}
	}
	if lin == nil || lin.Shape == nil {
		t.Fatal("data-lineage pattern missing or shapeless")
	}
	if !reflect.DeepEqual(lin.Shape.LockedColumns, []string{"value", "type"}) || !lin.Shape.AllowExtraRows {
		t.Fatalf("shape = %+v, want key+type locked and extra rows allowed", lin.Shape)
	}
	var keys []string
	for _, r := range lin.Shape.Rows {
		keys = append(keys, r.Defaults["value"].(string))
		wantType := "string"
		if r.Defaults["value"] == "transaction_rule" {
			wantType = "reference"
		}
		if r.Defaults["type"] != wantType || r.LabelKey == "" {
			t.Errorf("row %+v: want type %s and a caption", r, wantType)
		}
	}
	if !reflect.DeepEqual(keys, lineageKeys) {
		t.Fatalf("keys = %v", keys)
	}
}

func TestTablePatterns_ReturnsACopy(t *testing.T) {
	TablePatterns()[1].Shape.Rows[0].Defaults["value"] = "mutated"
	if TablePatterns()[1].Shape.Rows[0].Defaults["value"] == "mutated" {
		t.Fatal("caller mutated the registry")
	}
}

func TestApplyTablePattern_EmptySeedsAllColumns(t *testing.T) {
	got := ApplyTablePattern(nil, TablePatternDataLineage)
	if !reflect.DeepEqual(colKeys(got), lineageKeys) {
		t.Fatalf("keys = %v", colKeys(got))
	}
	for _, o := range got {
		m := o.(map[string]any)
		if m["type"] == "" || m["label"] == "" {
			t.Fatalf("column %v: want a type and a default label", m)
		}
	}
}

func TestApplyTablePattern_ExistingMatchingTableKeepsLabels(t *testing.T) {
	in := []any{
		col("source_entity", "Bron Entiteit"), col("source_attribute", "Bron Veld"),
		col("target_entity", "Doel Entiteit"), col("target_attribute", "Doel Veld"),
		col("example_value", "Voorbeeld waarde"), col("transaction_rule", "TR code"),
	}
	got := ApplyTablePattern(in, TablePatternDataLineage)
	if !reflect.DeepEqual(colKeys(got), lineageKeys) {
		t.Fatalf("keys = %v", colKeys(got))
	}
	if got[5].(map[string]any)["label"] != "TR code" || got[0].(map[string]any)["label"] != "Bron Entiteit" {
		t.Fatalf("labels lost: %v", got)
	}
}

func TestApplyTablePattern_ReordersAndKeepsExtrasAfter(t *testing.T) {
	in := []any{
		col("notes", "Notities"),
		col("target_entity", "Doel"),
		map[string]any{"value": "source_entity", "label": "Bron", "type": "number"},
		col("prio", "Prio"),
	}
	got := ApplyTablePattern(in, TablePatternDataLineage)
	want := append(append([]string{}, lineageKeys...), "notes", "prio")
	if !reflect.DeepEqual(colKeys(got), want) {
		t.Fatalf("keys = %v, want %v", colKeys(got), want)
	}
	src := got[0].(map[string]any)
	if src["label"] != "Bron" || src["type"] != "string" {
		t.Fatalf("source_entity = %v, want label kept and type forced to string", src)
	}
	if got[6].(map[string]any)["label"] != "Notities" {
		t.Fatalf("extra column label lost: %v", got[6])
	}
}

func TestApplyTablePattern_DoesNotMutateInput(t *testing.T) {
	m := map[string]any{"value": "source_entity", "label": "Bron", "type": "number"}
	ApplyTablePattern([]any{m}, TablePatternDataLineage)
	if m["type"] != "number" {
		t.Fatal("input column was mutated")
	}
}

func TestApplyTablePattern_RegularOrUnknownLeavesOptionsAlone(t *testing.T) {
	in := []any{col("a", "A"), col("b", "B")}
	for _, p := range []string{"", TablePatternRegular, "nope"} {
		if got := ApplyTablePattern(in, p); !reflect.DeepEqual(got, in) {
			t.Fatalf("pattern %q changed options: %v", p, got)
		}
	}
}

func TestApplyTablePattern_SkipsMalformedEntriesAndDuplicateKeys(t *testing.T) {
	in := []any{"junk", 7, map[string]any{"label": "no key"}, col("x", "X"), col("x", "X2"), col("source_entity", "S"), col("source_entity", "S2")}
	got := ApplyTablePattern(in, TablePatternDataLineage)
	want := append(append([]string{}, lineageKeys...), "x")
	if !reflect.DeepEqual(colKeys(got), want) {
		t.Fatalf("keys = %v, want %v", colKeys(got), want)
	}
	if got[0].(map[string]any)["label"] != "S" {
		t.Fatalf("first spelling should win: %v", got[0])
	}
}

func TestNormalize_TablePattern(t *testing.T) {
	cases := []struct {
		format, want string
		applied      bool
	}{
		{" Data-Lineage ", TablePatternDataLineage, true},
		{TablePatternRegular, "", false},
		{"", "", false},
		{"bogus", "", false},
	}
	for _, c := range cases {
		tpl := &Template{Fields: []Field{{Key: "m", Type: "table", Format: c.format, Options: []any{col("a", "A")}}}}
		Normalize(tpl)
		f := tpl.Fields[0]
		if f.Format != c.want {
			t.Errorf("format %q -> %q, want %q", c.format, f.Format, c.want)
		}
		if applied := len(f.Options) == len(lineageKeys)+1; applied != c.applied {
			t.Errorf("format %q: options %v (applied=%v, want %v)", c.format, colKeys(f.Options), applied, c.applied)
		}
	}
}

func TestNormalize_FormatStillClearedOnTypesWithoutIt(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "n", Type: "number", Format: TablePatternDataLineage}}}
	Normalize(tpl)
	if tpl.Fields[0].Format != "" {
		t.Fatalf("number kept format %q", tpl.Fields[0].Format)
	}
}

func TestTablePatterns_LineageHasDiagramCounterpart(t *testing.T) {
	for _, p := range TablePatterns() {
		if p.ID == TablePatternDataLineage && p.Diagram != DiagramProjectionLineage {
			t.Fatalf("data-lineage diagram = %q", p.Diagram)
		}
		if p.ID == TablePatternRegular && p.Diagram != "" {
			t.Fatal("regular has no diagram counterpart")
		}
	}
}

func TestNormalize_UsersExistingDatastroomTableIsUnchangedExceptTypes(t *testing.T) {
	in := []any{
		col("source_entity", "Bron Entiteit"), col("source_attribute", "Bron Veld"),
		col("target_entity", "Doel Entiteit"), col("target_attribute", "Doel Veld"),
		col("example_value", "Voorbeeld waarde"), col("transaction_rule", "TR code"),
	}
	tpl := &Template{Fields: []Field{{Key: "datastroom_tabel", Type: "table", Format: TablePatternDataLineage, Options: in,
		UseInStatistics: true, StatisticsColumns: []string{"transaction_rule"}}}}
	Normalize(tpl)
	f := tpl.Fields[0]
	if !reflect.DeepEqual(colKeys(f.Options), lineageKeys) {
		t.Fatalf("keys moved: %v", colKeys(f.Options))
	}
	if !reflect.DeepEqual(f.StatisticsColumns, []string{"transaction_rule"}) {
		t.Fatalf("stat columns = %v", f.StatisticsColumns)
	}
}

func TestService_TablePatternWrappers(t *testing.T) {
	s := &Service{}
	if len(s.TablePatterns()) != len(TablePatterns()) {
		t.Fatal("TablePatterns wrapper")
	}
	if got := s.ApplyTablePattern(nil, TablePatternDataLineage, nil); len(got) != len(lineageKeys) {
		t.Fatalf("ApplyTablePattern wrapper = %v", got)
	}
}

func TestApplyTablePatternLabels_SeedsNewColumnsOnly(t *testing.T) {
	labels := map[string]string{"source_entity": "Bron-entiteit", "target_entity": "", "transaction_rule": "Transactieregel"}
	in := []any{col("transaction_rule", "TR code")}
	got := ApplyTablePatternLabels(in, TablePatternDataLineage, labels)
	lab := func(i int) any { return got[i].(map[string]any)["label"] }
	if lab(0) != "Bron-entiteit" {
		t.Fatalf("new column label = %v, want translated", lab(0))
	}
	if lab(2) != "Target entity" {
		t.Fatalf("blank translation falls back to default: %v", lab(2))
	}
	if lab(5) != "TR code" {
		t.Fatalf("existing label overwritten: %v", lab(5))
	}
}

func TestService_ApplyTablePatternUsesLabels(t *testing.T) {
	got := (&Service{}).ApplyTablePattern(nil, TablePatternDataLineage, map[string]string{"example_value": "Voorbeeldwaarde"})
	if got[4].(map[string]any)["label"] != "Voorbeeldwaarde" {
		t.Fatalf("got %v", got[4])
	}
}
