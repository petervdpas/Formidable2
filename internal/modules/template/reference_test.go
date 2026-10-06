package template

import (
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

func refCol(key, target string) map[string]any {
	m := map[string]any{"value": key, "label": key, "type": "reference"}
	if target != "" {
		m[ReferenceTargetKey] = target
	}
	return m
}

// trTemplate: a mapping table whose "rule" column references codes kept in a
// top-level loop of rules.
func trFields(target string) []Field {
	return []Field{
		{Key: "mapping", Type: "table", Options: []any{col("src", "Src"), refCol("rule", target)}},
		{Key: "rules", Type: "loopstart"},
		{Key: "tr_code", Type: "text"},
		{Key: "tr_text", Type: "textarea"},
		{Key: "rules", Type: "loopstop"},
		{Key: "title", Type: "text"},
	}
}

func TestReferenceColumnType_HasScalarTargetSubRow(t *testing.T) {
	for _, d := range builtinTableColumnTypes {
		if d.Name != "reference" {
			continue
		}
		if d.SubRow == nil || d.SubRow.RowKey != ReferenceTargetKey || !d.SubRow.Scalar || d.SubRow.LabelKey == "" || d.SubRow.Input != SubRowInputLoopField {
			t.Fatalf("reference sub-row = %+v", d.SubRow)
		}
		return
	}
	t.Fatal("no reference column type")
}

func TestReferenceColumns_IndexToTarget(t *testing.T) {
	f := Field{Key: "m", Type: "table", Options: []any{col("a", "A"), refCol("r1", "tr_code"), refCol("r2", ""), "junk"}}
	if got := ReferenceColumns(f); !reflect.DeepEqual(got, map[int]string{1: "tr_code"}) {
		t.Fatalf("got %v", got)
	}
	if got := ReferenceColumns(Field{Type: "text"}); len(got) != 0 {
		t.Fatalf("non-table: %v", got)
	}
}

func TestReferenceTargets_MapsTargetToItsLoop(t *testing.T) {
	if got := ReferenceTargets(trFields("tr_code")); !reflect.DeepEqual(got, map[string]string{"tr_code": "rules"}) {
		t.Fatalf("got %v", got)
	}
	if got := ReferenceTargets(trFields("title")); len(got) != 0 {
		t.Fatalf("a root field is not a valid target: %v", got)
	}
}

func TestReferenceValidation(t *testing.T) {
	nested := []Field{
		{Key: "mapping", Type: "table", Options: []any{refCol("rule", "deep")}},
		{Key: "outer", Type: "loopstart"},
		{Key: "inner", Type: "loopstart"},
		{Key: "deep", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "outer", Type: "loopstop"},
	}
	cases := []struct {
		name   string
		fields []Field
		want   string
	}{
		{"valid", trFields("tr_code"), ""},
		{"no target is a plain column", trFields(""), ""},
		{"unknown target", trFields("ghost"), "table-reference-unknown-target"},
		{"root target", trFields("title"), "table-reference-target-not-in-loop"},
		{"loop marker target", trFields("rules"), "table-reference-target-not-in-loop"},
		{"nested loop target", nested, "table-reference-target-not-in-loop"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, e := range Validate(&Template{Fields: c.fields}) {
				if len(e.Type) > 16 && e.Type[:16] == "table-reference-" {
					got = append(got, e.Type)
				}
			}
			if c.want == "" && len(got) != 0 || c.want != "" && !reflect.DeepEqual(got, []string{c.want}) {
				t.Fatalf("got %v, want %q", got, c.want)
			}
		})
	}
}

func TestReferenceAnchor(t *testing.T) {
	cases := []struct{ target, code, want string }{
		{"tr_code", "Q-01", "ref-tr_code-q-01"},
		{"tr_code", " q 01 ", "ref-tr_code-q-01"},
		{"TR.Code", "C/02!", "ref-tr-code-c-02"},
		{"tr_code", "", ""},
		{"tr_code", "---", ""},
	}
	for _, c := range cases {
		if got := ReferenceAnchor(c.target, c.code); got != c.want {
			t.Errorf("ReferenceAnchor(%q,%q) = %q, want %q", c.target, c.code, got, c.want)
		}
	}
}

func TestSplitReferenceCodes(t *testing.T) {
	cases := map[string][]string{
		"":                  nil,
		" , ; ":             nil,
		"Q-01":              {"Q-01"},
		"Q-01, Q-02,C-01":   {"Q-01", "Q-02", "C-01"},
		"Q-01;q-01 ; E-01 ": {"Q-01", "E-01"},
	}
	for in, want := range cases {
		if got := SplitReferenceCodes(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitReferenceCodes(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDataLineagePattern_RuleColumnIsAReference(t *testing.T) {
	opts := ApplyTablePattern([]any{map[string]any{"value": "transaction_rule", "label": "TR", ReferenceTargetKey: "tr_code"}}, TablePatternDataLineage)
	rule := opts[5].(map[string]any)
	if rule["type"] != "reference" || rule[ReferenceTargetKey] != "tr_code" || rule["label"] != "TR" {
		t.Fatalf("rule column = %v, want type reference with target and label kept", rule)
	}
	if opts[0].(map[string]any)["type"] != "string" {
		t.Fatalf("entity columns stay string: %v", opts[0])
	}
}

func TestService_ReferenceTargetLoops(t *testing.T) {
	m := NewManager(system.NewManager(t.TempDir(), nil), "templates", nil)
	tpl := &Template{Name: "M", Filename: "m.yaml", Fields: append([]Field{{Key: "id", Type: "guid"}}, trFields("tr_code")...)}
	if err := m.SaveTemplate("m.yaml", tpl); err != nil {
		t.Fatal(err)
	}
	s := NewService(m, nil)
	got, err := s.ReferenceTargetLoops("m.yaml")
	if err != nil || !reflect.DeepEqual(got, map[string]string{"tr_code": "rules"}) {
		t.Fatalf("got %v err %v", got, err)
	}
	if _, err := s.ReferenceTargetLoops("missing.yaml"); err == nil {
		t.Fatal("missing template must error")
	}
}

func TestReferenceTargetCandidates_DataFieldsDirectlyInTopLevelLoops(t *testing.T) {
	fields := []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "rules", Type: "loopstart", Label: "Rules"},
		{Key: "tr_code", Type: "text", Label: "Code"},
		{Key: "tr_tag", Type: "facet"},
		{Key: "inner", Type: "loopstart"},
		{Key: "deep", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "tr_text", Type: "textarea"},
		{Key: "rules", Type: "loopstop"},
		{Key: "after", Type: "text"},
	}
	want := []ItemField{{Key: "tr_code", Label: "Rules: Code"}, {Key: "tr_text", Label: "Rules: tr_text"}}
	if got := ReferenceTargetCandidates(fields); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, c := range ReferenceTargetCandidates(fields) {
		if referenceLoopOf(fields, c.Key) == "" {
			t.Fatalf("candidate %q fails the validation rule", c.Key)
		}
	}
	if got := ReferenceTargetCandidates(nil); len(got) != 0 {
		t.Fatalf("nil fields: %v", got)
	}
}
