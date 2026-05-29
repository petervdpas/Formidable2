package datacore

import "testing"

// Three forms with an items loop carrying a cost column.
func costedForms() *Tensor {
	dt := New()
	dt.Ingest(Record{ID: "a", Tables: map[string][]map[string]string{
		"items": {{"cost": "100"}, {"cost": "50"}},
	}})
	dt.Ingest(Record{ID: "b", Tables: map[string][]map[string]string{
		"items": {{"cost": "25"}},
	}})
	dt.Ingest(Record{ID: "c", Fields: map[string]string{"title": "no items"}}) // empty loop
	return dt
}

func byID(sums []RootSummary) map[string]RootSummary {
	m := map[string]RootSummary{}
	for _, s := range sums {
		m[s.ID] = s
	}
	return m
}

func TestSummarizeLandsLoopTotalsOnEachRoot(t *testing.T) {
	sums := costedForms().View().Summarize("items", "cost")
	m := byID(sums)

	if len(sums) != 3 {
		t.Fatalf("summaries = %d, want 3 (one per root)", len(sums))
	}
	if a := m["a"]; a.Rows != 2 || !eq(a.Agg.Sum, 150) || !eq(a.Agg.Mean, 75) {
		t.Fatalf("a = %+v, want rows 2 sum 150 mean 75", a)
	}
	if b := m["b"]; b.Rows != 1 || !eq(b.Agg.Sum, 25) {
		t.Fatalf("b = %+v, want rows 1 sum 25", b)
	}
}

func TestSummarizeEmptyLoopAppearsWithZeroRows(t *testing.T) {
	m := byID(costedForms().View().Summarize("items", "cost"))
	if c, ok := m["c"]; !ok || c.Rows != 0 || c.Agg.N != 0 || c.Agg.Sum != 0 {
		t.Fatalf("c = %+v (present %v), want rows 0 and zero agg", c, ok)
	}
}

// A plain count summary: no value field, just the loop length per root.
func TestSummarizeCountOnly(t *testing.T) {
	m := byID(costedForms().View().Summarize("items", ""))
	if m["a"].Rows != 2 || m["b"].Rows != 1 || m["c"].Rows != 0 {
		t.Fatalf("row counts = a:%d b:%d c:%d, want 2/1/0", m["a"].Rows, m["b"].Rows, m["c"].Rows)
	}
	if m["a"].Agg.N != 0 {
		t.Fatalf("count-only summary should leave Agg zero, got %+v", m["a"].Agg)
	}
}

// The defining property: the per-root summaries partition the global reduction.
// Summing each form's loop total reconstitutes the pooled Follow().Aggregate(),
// which is itself parity-checked against the index.
func TestSummarizePartitionsTheGlobalReduction(t *testing.T) {
	dt := costedForms()

	perRoot := 0.0
	rowSum := 0
	for _, s := range dt.View().Summarize("items", "cost") {
		perRoot += s.Agg.Sum
		rowSum += s.Rows
	}
	global := dt.View().Follow("items").Aggregate("cost")

	if !eq(perRoot, global.Sum) {
		t.Fatalf("sum of per-root totals = %g, global = %g; must match", perRoot, global.Sum)
	}
	if rowSum != global.N {
		t.Fatalf("sum of loop lengths = %d, global numeric N = %d; must match", rowSum, global.N)
	}
}

// An anomaly stays scoped to the root whose row carries it; other roots'
// summaries are unaffected.
func TestSummarizeKeepsAnomaliesScopedToTheRoot(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Tables: map[string][]map[string]string{
		"items": {{"cost": "100"}, {"cost": "junk"}},
	}})
	dt.Ingest(Record{ID: "b", Tables: map[string][]map[string]string{
		"items": {{"cost": "25"}},
	}})

	m := byID(dt.View().Summarize("items", "cost"))
	if a := m["a"]; a.Agg.N != 1 || !eq(a.Agg.Sum, 100) || len(a.Agg.Anomalies) != 1 || a.Agg.Anomalies[0].Value != "junk" {
		t.Fatalf("a = %+v, want N 1 sum 100 one junk anomaly", a)
	}
	if b := m["b"]; len(b.Agg.Anomalies) != 0 || !eq(b.Agg.Sum, 25) {
		t.Fatalf("b = %+v, want clean sum 25", b)
	}
}

func TestSummarizeUnknownLinkFieldIsNil(t *testing.T) {
	if s := costedForms().View().Summarize("ghost", "cost"); s != nil {
		t.Fatalf("summary over unknown link = %+v, want nil", s)
	}
}
