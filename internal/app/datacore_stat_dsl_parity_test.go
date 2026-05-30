package app

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/stat"
)

// Layer 2 of the stat-on-datacore parity gate (design/datacore-stat-migration.md):
// the real gate. Run the SAME DSL strings (and the same convenience-method
// calls) through two stat.Managers, one backed by the index, one by the
// datacore adapter, both wired with identical SourceOptions + ColumnResolver,
// and assert identical *Grid. This exercises the adapter through the real stat
// engine (parse, aggregate-raw, grouping, axis building, percents), not the
// Index methods in isolation, so it catches any call pattern the per-method
// tests miss.
//
// Note on divergence: the engine's fan-out guard rejects two table-column
// sources, so the one place datacore intentionally diverges from the index
// (the same-table cartesian) is unreachable through the DSL. Through stat, the
// two engines must agree on everything, including a single table-column
// dimension.

type fakeStatOptions struct{}

func (fakeStatOptions) DimensionLabels(_ string, src stat.SourceRef) ([]stat.CategoryOption, bool) {
	if src.Kind != stat.SourceFacet {
		return nil, false
	}
	switch src.Key {
	case "tier":
		return []stat.CategoryOption{{Value: "GOLD", Label: "GOLD"}, {Value: "SILVER", Label: "SILVER"}, {Value: "BRONZE", Label: "BRONZE"}}, true
	case "stage":
		return []stat.CategoryOption{{Value: "draft", Label: "draft"}, {Value: "live", Label: "live"}}, true
	}
	return nil, false
}

type fakeStatCols struct{}

func (fakeStatCols) ColumnIndex(_, fieldKey, columnKey string) (int, bool) {
	if fieldKey == "items" && columnKey == "cost" {
		return 0, true
	}
	return 0, false
}

func twoStatManagers(t *testing.T) (idxMgr, dcMgr *stat.Manager) {
	return twoStatManagersFor(t, richStatFixture())
}

func twoStatManagersFor(t *testing.T, forms []statForm) (idxMgr, dcMgr *stat.Manager) {
	t.Helper()
	adapter, idxM := newDatacoreStatAdapter(t, forms)
	idxMgr = stat.NewManager(idxM)
	dcMgr = stat.NewManager(adapter)
	for _, m := range []*stat.Manager{idxMgr, dcMgr} {
		m.SetSourceOptions(fakeStatOptions{})
		m.SetColumnResolver(fakeStatCols{})
	}
	return idxMgr, dcMgr
}

// TestStatDSLFacetUnsetDiverges confirms the settled (unset) divergence reaches
// the DSL path too: the index's facet-dim join includes set-but-unselected
// forms under the "" category (set_flag=1, COALESCE(selected,'')), datacore
// drops them (blank = absence). The engines agree on every real category; only
// the "" category differs. Pinned so the divergence stays intended, not a
// regression, and so a future parity test that accidentally adds a set-empty
// facet fails loudly here instead.
func TestStatDSLFacetUnsetDiverges(t *testing.T) {
	forms := []statForm{
		{id: "a.meta.json", text: map[string]string{"status": "high"}, facets: map[string]string{"tier": "GOLD", "stage": "live"}},
		{id: "b.meta.json", text: map[string]string{"status": "low"}, facets: map[string]string{"tier": "GOLD", "stage": ""}},
		{id: "c.meta.json", text: map[string]string{"status": "high"}, facets: map[string]string{"tier": "SILVER", "stage": ""}},
	}
	idxMgr, dcMgr := twoStatManagersFor(t, forms)

	want, err := idxMgr.EvaluateDSL("basic.yaml", `count() by Facet["stage"]`)
	if err != nil {
		t.Fatalf("index DSL: %v", err)
	}
	got, err := dcMgr.EvaluateDSL("basic.yaml", `count() by Facet["stage"]`)
	if err != nil {
		t.Fatalf("datacore DSL: %v", err)
	}
	// The index axis carries an "" tick (the unset category); datacore does not.
	if !slices.Contains(want.Axes[0].Labels, "") {
		t.Fatalf("index axis should carry the unset category: %v", want.Axes[0].Labels)
	}
	if slices.Contains(got.Axes[0].Labels, "") {
		t.Fatalf("datacore axis must drop the unset category: %v", got.Axes[0].Labels)
	}
	// Both still carry the real "live" category with the same count (1, form a).
	if liveCount(want) != 1 || liveCount(got) != 1 {
		t.Fatalf("real category disagreement: index live=%d datacore live=%d", liveCount(want), liveCount(got))
	}
}

func liveCount(g *stat.Grid) int {
	idx := -1
	for i, l := range g.Axes[0].Labels {
		if l == "live" {
			idx = i
		}
	}
	if idx < 0 {
		return 0
	}
	for _, c := range g.Cells {
		if len(c.Coords) == 1 && c.Coords[0] == idx && len(c.Values) > 0 {
			return int(c.Values[0])
		}
	}
	return 0
}

func TestStatDSLParity(t *testing.T) {
	idxMgr, dcMgr := twoStatManagers(t)

	queries := []string{
		`count() by F["status"]`,
		`count() by Facet["tier"]`,
		`count() by Facet["tier"], Facet["stage"]`,
		`count() by F["due"]@month`,
		`count() by F["due"]@year`,
		`count(), avg(F["amount"]) by F["status"]`,
		`count(), records() by F["status"]`,
		`count() by F["status"] pct forms`,
		`count() by Facet["tier"] pct forms`,
		`count() by F["status"] where F["amount"] gt 15`,
		`count() by F["status"] where Facet["tier"] eq "GOLD"`,
		`sum(F["amount"]), avg(F["amount"]), min(F["amount"]), max(F["amount"]), median(F["amount"]), stddev(F["amount"])`,
		`count() by F["items"]["cost"]`,                    // single table-column dim: must still agree
		`sum(F["items"]["cost"]) by Facet["tier"]`,         // table-column measure broadcast over a root facet
		`count() by F["status"], F["due"]@month`,           // rank-2 mixed
	}

	for _, q := range queries {
		want, errW := idxMgr.EvaluateDSL("basic.yaml", q)
		got, errG := dcMgr.EvaluateDSL("basic.yaml", q)
		if (errW == nil) != (errG == nil) {
			t.Fatalf("%q: error mismatch index=%v datacore=%v", q, errW, errG)
		}
		if errW != nil {
			continue
		}
		assertGridsEqual(t, q, want, got)
	}
}

// TestStatConvenienceMethodParity covers the non-DSL stat.Manager surface
// (Distribution / FacetDistribution / TimeSeries / NumericStats / CrossTab),
// which wraps the Index methods the DSL path does not touch.
func TestStatConvenienceMethodParity(t *testing.T) {
	idxMgr, dcMgr := twoStatManagers(t)
	col0 := 0
	p90 := 90.0

	cases := []struct {
		name string
		run  func(m *stat.Manager) (*stat.Grid, error)
	}{
		{"Distribution status", func(m *stat.Manager) (*stat.Grid, error) { return m.Distribution("basic.yaml", "status", nil) }},
		{"Distribution items.cost", func(m *stat.Manager) (*stat.Grid, error) { return m.Distribution("basic.yaml", "items", &col0) }},
		{"FacetDistribution tier", func(m *stat.Manager) (*stat.Grid, error) { return m.FacetDistribution("basic.yaml", "tier") }},
		{"TimeSeries due month", func(m *stat.Manager) (*stat.Grid, error) { return m.TimeSeries("basic.yaml", "due", nil, "month") }},
		{"NumericStats amount", func(m *stat.Manager) (*stat.Grid, error) { return m.NumericStats("basic.yaml", "amount", nil, nil) }},
		{"NumericStats amount p90", func(m *stat.Manager) (*stat.Grid, error) { return m.NumericStats("basic.yaml", "amount", nil, &p90) }},
		{"NumericStats items.cost", func(m *stat.Manager) (*stat.Grid, error) { return m.NumericStats("basic.yaml", "items", &col0, nil) }},
		{"CrossTab tier x stage", func(m *stat.Manager) (*stat.Grid, error) { return m.CrossTab("basic.yaml", "tier", "stage") }},
	}

	for _, c := range cases {
		want, errW := c.run(idxMgr)
		got, errG := c.run(dcMgr)
		if (errW == nil) != (errG == nil) {
			t.Fatalf("%s: error mismatch index=%v datacore=%v", c.name, errW, errG)
		}
		if errW != nil {
			continue
		}
		assertGridsEqual(t, c.name, want, got)
	}
}

// assertGridsEqual compares two grids for engine parity: axes and measures
// exactly (strings, deterministic), total exactly, and cells as a set keyed by
// coordinate (the engines may emit cells in a different order, but the same
// label ordering makes coordinates canonical). Values and percents are compared
// with a tolerance, because the only freedom between engines is float-add order
// inside a group's sum/avg/stddev.
func assertGridsEqual(t *testing.T, label string, want, got *stat.Grid) {
	t.Helper()
	if err := gridsEqual(want, got); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

// gridsEqual reports the first difference between two grids, or nil. Axes,
// measures, and total must match exactly; cells are matched as a set keyed by
// coordinate (emit order may differ) with values/percents compared by
// tolerance (float-add order is the only freedom). Returning an error rather
// than failing a *testing.T makes it safe to call off the test goroutine (the
// concurrency test).
func gridsEqual(want, got *stat.Grid) error {
	if len(want.Axes) != len(got.Axes) {
		return fmt.Errorf("axis count want=%d got=%d", len(want.Axes), len(got.Axes))
	}
	for i := range want.Axes {
		if want.Axes[i].Source != got.Axes[i].Source {
			return fmt.Errorf("axis %d source want=%q got=%q", i, want.Axes[i].Source, got.Axes[i].Source)
		}
		if !equalStrings(want.Axes[i].Labels, got.Axes[i].Labels) {
			return fmt.Errorf("axis %d labels want=%v got=%v", i, want.Axes[i].Labels, got.Axes[i].Labels)
		}
	}
	if !equalStrings(want.Measures, got.Measures) {
		return fmt.Errorf("measures want=%v got=%v", want.Measures, got.Measures)
	}
	if want.Total != got.Total {
		return fmt.Errorf("total want=%d got=%d", want.Total, got.Total)
	}
	if len(want.Cells) != len(got.Cells) {
		return fmt.Errorf("cell count want=%d got=%d", len(want.Cells), len(got.Cells))
	}
	gotByCoord := map[string]stat.GridCell{}
	for _, c := range got.Cells {
		gotByCoord[coordsKey(c.Coords)] = c
	}
	for _, w := range want.Cells {
		g, ok := gotByCoord[coordsKey(w.Coords)]
		if !ok {
			return fmt.Errorf("cell %v present in index, missing in datacore", w.Coords)
		}
		if !floatsClose(w.Values, g.Values) {
			return fmt.Errorf("cell %v values want=%v got=%v", w.Coords, w.Values, g.Values)
		}
		if !floatsClose(w.Pct, g.Pct) {
			return fmt.Errorf("cell %v pct want=%v got=%v", w.Coords, w.Pct, g.Pct)
		}
	}
	return nil
}

func coordsKey(coords []int) string {
	b := make([]byte, 0, len(coords)*3)
	for _, c := range coords {
		b = append(b, byte(c+1), ',')
	}
	return string(b)
}

func floatsClose(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}
