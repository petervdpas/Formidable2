package storage

import (
	"reflect"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func copyStack(t *testing.T, data map[string]any) *Manager {
	t.Helper()
	m, _, tplM, _ := newTestStack(t)
	cols := func(keys ...string) []any {
		out := []any{}
		for _, k := range keys {
			out = append(out, map[string]any{"value": k, "label": k})
		}
		return out
	}
	tpl := &template.Template{Name: "C", Filename: "c.yaml", Fields: []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "old", Type: "table", Options: cols("a", "b", "x")},
		{Key: "new", Type: "table", Options: cols("b", "a", "c")},
		{Key: "other", Type: "table", Options: cols("p")},
		{Key: "title", Type: "text"},
	}}
	if err := tplM.SaveTemplate("c.yaml", tpl); err != nil {
		t.Fatal(err)
	}
	if data != nil {
		if r := m.SaveForm(bg, "c.yaml", "r.meta.json", data); !r.Success {
			t.Fatal(r.Error)
		}
	}
	return m
}

func rowsOf(t *testing.T, v any) [][]any {
	t.Helper()
	out := [][]any{}
	for _, r := range v.([]any) {
		out = append(out, r.([]any))
	}
	return out
}

func TestCopyTableRows_MapsByColumnKeyAndAppends(t *testing.T) {
	m := copyStack(t, map[string]any{
		"old": []any{[]any{"A1", "B1", "X1"}, []any{"A2", "B2", "X2"}},
		"new": []any{[]any{"keepB", "keepA", "keepC"}},
	})
	got, err := m.CopyTableRows("c.yaml", "r.meta.json", "new", "old")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"keepB", "keepA", "keepC"}, {"B1", "A1", ""}, {"B2", "A2", ""}}
	if !reflect.DeepEqual(rowsOf(t, got.Value), want) || got.Added != 2 || len(got.UnknownReferences) != 0 {
		t.Fatalf("got %+v, want %v with 2 added", got, want)
	}
}

func TestCopyTableRows_IsIdempotent(t *testing.T) {
	m := copyStack(t, map[string]any{
		"old": []any{[]any{"A1", "B1", "X1"}, []any{"A1", "B1", "other-x"}},
		"new": []any{[]any{"B1", "A1", ""}},
	})
	got, err := m.CopyTableRows("c.yaml", "r.meta.json", "new", "old")
	if err != nil || len(rowsOf(t, got.Value)) != 1 || got.Added != 0 {
		t.Fatalf("got %v err %v, want the one existing row (already-present and duplicate rows are not added)", got, err)
	}
}

func TestCopyTableRows_SkipsRowsThatMapToNothingAndToleratesJunk(t *testing.T) {
	m := copyStack(t, map[string]any{
		"old": []any{[]any{"", "", "only-x"}, []any{"A"}, "junk", []any{nil, 7.0, "x"}},
	})
	got, err := m.CopyTableRows("c.yaml", "r.meta.json", "new", "old")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"", "A", ""}, {7.0, "", ""}}
	if !reflect.DeepEqual(rowsOf(t, got.Value), want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCopyTableRows_DoesNotWriteToDisk(t *testing.T) {
	m := copyStack(t, map[string]any{"old": []any{[]any{"A1", "B1", "X1"}}})
	if _, err := m.CopyTableRows("c.yaml", "r.meta.json", "new", "old"); err != nil {
		t.Fatal(err)
	}
	if rows := asSlice(m.LoadForm("c.yaml", "r.meta.json").Data["new"]); len(rows) != 0 {
		t.Fatalf("target was persisted: %v", rows)
	}
}

func TestCopyTableRows_Errors(t *testing.T) {
	m := copyStack(t, map[string]any{"old": []any{[]any{"A1", "B1", "X1"}}})
	cases := []struct{ name, datafile, target, source, want string }{
		{"same table", "r.meta.json", "new", "new", "itself"},
		{"unknown source", "r.meta.json", "new", "ghost", "unknown field"},
		{"unknown target", "r.meta.json", "ghost", "old", "unknown field"},
		{"source not a table", "r.meta.json", "new", "title", "not a table"},
		{"target not a table", "r.meta.json", "title", "old", "not a table"},
		{"no shared column", "r.meta.json", "new", "other", "no column"},
		{"unsaved record", "nope.meta.json", "new", "old", "not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := m.CopyTableRows("c.yaml", c.datafile, c.target, c.source)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// refStack: an old free-text rule column copied into a reference column that
// targets the codes of a loop of rules.
func refStack(t *testing.T, target string, data map[string]any) *Manager {
	t.Helper()
	m, _, tplM, _ := newTestStack(t)
	ref := map[string]any{"value": "rule", "label": "Rule", "type": "reference"}
	if target != "" {
		ref[template.ReferenceTargetKey] = target
	}
	tpl := &template.Template{Name: "R", Filename: "r.yaml", Fields: []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "old", Type: "table", Options: []any{map[string]any{"value": "src", "label": "Src"}, map[string]any{"value": "rule", "label": "TR"}}},
		{Key: "new", Type: "table", Options: []any{map[string]any{"value": "src", "label": "Src"}, ref}},
		{Key: "rules", Type: "loopstart"},
		{Key: "tr_code", Type: "text"},
		{Key: "rules", Type: "loopstop"},
	}}
	if err := tplM.SaveTemplate("r.yaml", tpl); err != nil {
		t.Fatal(err)
	}
	if r := m.SaveForm(bg, "r.yaml", "r.meta.json", data); !r.Success {
		t.Fatal(r.Error)
	}
	return m
}

func ruleItems(codes ...string) []any {
	out := []any{}
	for _, c := range codes {
		out = append(out, map[string]any{"tr_code": c})
	}
	return out
}

func TestCopyTableRows_ReferenceCellsTakeTheLoopItemsSpelling(t *testing.T) {
	m := refStack(t, "tr_code", map[string]any{
		"old":   []any{[]any{"A", "q-01;C-03"}, []any{"B", " Q-01 ,  q-01 "}, []any{"C", ""}},
		"rules": ruleItems("Q-01", "C-03"),
	})
	got, err := m.CopyTableRows("r.yaml", "r.meta.json", "new", "old")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"A", "Q-01, C-03"}, {"B", "Q-01"}, {"C", ""}}
	if !reflect.DeepEqual(rowsOf(t, got.Value), want) || len(got.UnknownReferences) != 0 {
		t.Fatalf("got %+v, want %v with no unknown codes", got, want)
	}
}

func TestCopyTableRows_UnknownReferenceCodesAreKeptAndReported(t *testing.T) {
	m := refStack(t, "tr_code", map[string]any{
		"old":   []any{[]any{"A", "Q-01, X-99"}, []any{"B", "x-99, zie Q-01"}},
		"rules": ruleItems("Q-01"),
	})
	got, err := m.CopyTableRows("r.yaml", "r.meta.json", "new", "old")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"A", "Q-01, X-99"}, {"B", "x-99, zie Q-01"}}
	if !reflect.DeepEqual(rowsOf(t, got.Value), want) {
		t.Fatalf("rows = %v, want %v (unknown codes are never dropped)", got.Value, want)
	}
	if !reflect.DeepEqual(got.UnknownReferences, []string{"X-99", "zie Q-01"}) {
		t.Fatalf("unknown = %v, want each unmatched code once, first spelling", got.UnknownReferences)
	}
}

func TestCopyTableRows_ReferenceColumnWithoutTargetCopiesVerbatim(t *testing.T) {
	m := refStack(t, "", map[string]any{
		"old":   []any{[]any{"A", "q-01;C-03"}},
		"rules": ruleItems("Q-01"),
	})
	got, _ := m.CopyTableRows("r.yaml", "r.meta.json", "new", "old")
	if rows := rowsOf(t, got.Value); rows[0][1] != "q-01;C-03" || len(got.UnknownReferences) != 0 {
		t.Fatalf("got %+v, want the cell untouched and nothing reported", got)
	}
}

func TestCopyTableRows_NormalisedReferenceRowsStayIdempotent(t *testing.T) {
	m := refStack(t, "tr_code", map[string]any{
		"old":   []any{[]any{"A", "q-01;C-03"}},
		"new":   []any{[]any{"A", "Q-01, C-03"}},
		"rules": ruleItems("Q-01", "C-03"),
	})
	got, _ := m.CopyTableRows("r.yaml", "r.meta.json", "new", "old")
	if got.Added != 0 || len(rowsOf(t, got.Value)) != 1 {
		t.Fatalf("got %+v, want nothing added: the copied row equals the existing one after normalising", got)
	}
}
