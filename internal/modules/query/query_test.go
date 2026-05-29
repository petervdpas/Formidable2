package query

import (
	"fmt"
	"testing"
)

// Manager.Run is a thin wrapper over Prepare + Execute (both covered
// exhaustively in prepare_test / matrix_test). These tests check the
// wiring: the template guard, that a run flows through both steps, and
// that errors from either step propagate.

func sampleLoader() fakeLoader {
	return fakeLoader{
		tpl: appsTpl(),
		forms: []FormData{
			{Filename: "a.json", Data: map[string]any{"team": "Alpha"}, Facets: map[string]string{"tier": "gold"}},
			{Filename: "b.json", Data: map[string]any{"team": "Beta"}, Facets: map[string]string{"tier": "silver"}},
		},
	}
}

func TestRun_RequiresTemplate(t *testing.T) {
	if _, err := NewManager(sampleLoader()).Run(Spec{Template: "   "}); err == nil {
		t.Fatal("blank template should error")
	}
}

func TestRun_PreparesAndExecutes(t *testing.T) {
	res, err := NewManager(sampleLoader()).Run(Spec{
		Template: "t.yaml",
		Columns:  cols(scalar("team"), facet("tier")),
		OrderBy:  []Sort{{Column: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 || res.Total != 2 {
		t.Fatalf("count=%d total=%d, want 2/2", res.Count, res.Total)
	}
	if res.Rows[0][0].Text != "Alpha" {
		t.Fatalf("ordered row 0 = %q, want Alpha", res.Rows[0][0].Text)
	}
}

func TestRun_GroupCount(t *testing.T) {
	res, err := NewManager(sampleLoader()).Run(Spec{
		Template: "t.yaml",
		Columns:  cols(facet("tier")),
		GroupBy:  []int{0},
		Measures: []Measure{{Func: "count_distinct", Header: "forms"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("want 2 tier groups, got %d", res.Count)
	}
}

func TestRun_PropagatesLoaderError(t *testing.T) {
	m := NewManager(fakeLoader{err: fmt.Errorf("boom")})
	if _, err := m.Run(Spec{Template: "t.yaml", Columns: cols(scalar("team"))}); err == nil {
		t.Fatal("loader error should propagate")
	}
}

func TestRun_UnknownSourceErrors(t *testing.T) {
	m := NewManager(sampleLoader())
	if _, err := m.Run(Spec{Template: "t.yaml", Columns: cols(scalar("nope"))}); err == nil {
		t.Fatal("unknown field should error from prepare")
	}
}

func TestRun_OutOfRangeGroupErrors(t *testing.T) {
	m := NewManager(sampleLoader())
	if _, err := m.Run(Spec{Template: "t.yaml", Columns: cols(scalar("team")), GroupBy: []int{5}}); err == nil {
		t.Fatal("out-of-range group should error from execute")
	}
}
