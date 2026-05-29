package query

import (
	"strings"
	"testing"
)

func TestExplain_ListWithFilterAndOrder(t *testing.T) {
	m := NewManager(sampleLoader())
	sql, err := m.Explain(Spec{
		Template: "t.yaml",
		Columns:  cols(scalar("team"), facet("tier")),
		Filters:  []Filter{{Source: facet("tier"), Op: "eq", Value: "gold"}},
		OrderBy:  []Sort{{Column: 0}},
		Distinct: true,
		Limit:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SELECT DISTINCT", `"team"`, "FROM", `WHERE "tier" = 'gold'`, "ORDER BY", "LIMIT 10;"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestExplain_GroupWithMeasures(t *testing.T) {
	tpl := appsTpl()
	m := NewManager(fakeLoader{tpl: tpl})
	sql, err := m.Explain(Spec{
		Template: "t.yaml",
		Columns:  cols(scalar("team"), tcol("apps", 1)),
		GroupBy:  []int{0},
		Measures: []Measure{
			{Func: "count_distinct", Header: "forms"},
			{Func: "sum", Source: tcol("apps", 1), Header: "total"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`COUNT(DISTINCT record) AS "forms"`,
		`SUM("apps / cost") AS "total"`,
		`GROUP BY "team"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestExplain_NumericFilterUnquoted(t *testing.T) {
	m := NewManager(fakeLoader{tpl: appsTpl()})
	sql, _ := m.Explain(Spec{
		Template: "t.yaml",
		Columns:  cols(tcol("apps", 1)),
		Filters:  []Filter{{Source: tcol("apps", 1), Op: "ge", Value: "100"}},
	})
	if !strings.Contains(sql, `"apps / cost" >= 100`) {
		t.Fatalf("numeric filter should be unquoted:\n%s", sql)
	}
}

func TestExplain_RequiresTemplate(t *testing.T) {
	if _, err := NewManager(sampleLoader()).Explain(Spec{Template: " "}); err == nil {
		t.Fatal("blank template should error")
	}
}
