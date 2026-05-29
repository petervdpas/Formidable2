package datacore

import (
	"math"
	"testing"
)

func eq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func amounts(vals ...string) *Tensor {
	dt := New()
	for i, v := range vals {
		dt.Ingest(Record{ID: string(rune('a' + i)), Fields: map[string]string{"amount": v}})
	}
	return dt
}

func TestAggregateNumericSummary(t *testing.T) {
	a := amounts("10", "30", "20").View().Aggregate("amount")
	if a.N != 3 {
		t.Fatalf("N = %d, want 3", a.N)
	}
	if !eq(a.Sum, 60) || !eq(a.Mean, 20) || !eq(a.Min, 10) || !eq(a.Max, 30) {
		t.Fatalf("agg = %+v, want sum 60 mean 20 min 10 max 30", a)
	}
	if len(a.Anomalies) != 0 {
		t.Fatalf("anomalies = %v, want none", a.Anomalies)
	}
}

func TestAggregateHandlesDecimalsAndNegatives(t *testing.T) {
	a := amounts("-5", "2.5", "10").View().Aggregate("amount")
	if !eq(a.Sum, 7.5) || !eq(a.Min, -5) || !eq(a.Max, 10) {
		t.Fatalf("agg = %+v, want sum 7.5 min -5 max 10", a)
	}
}

// type-on-demand: the field is aggregated as numeric; a value that will not
// coerce is surfaced, not dropped, and does not corrupt the running stats.
func TestAggregateSurfacesNonNumericAsAnomaly(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"amount": "10"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{"amount": "oops"}})
	dt.Ingest(Record{ID: "c", Fields: map[string]string{"amount": "30"}})

	a := dt.View().Aggregate("amount")
	if a.N != 2 || !eq(a.Sum, 40) || !eq(a.Mean, 20) {
		t.Fatalf("agg = %+v, want N 2 sum 40 mean 20 (anomaly excluded)", a)
	}
	if len(a.Anomalies) != 1 || a.Anomalies[0].ID != "b" || a.Anomalies[0].Value != "oops" {
		t.Fatalf("anomalies = %+v, want one b/oops", a.Anomalies)
	}
}

func TestAggregateSkipsBlankAsAbsence(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"amount": "10"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{}}) // no amount
	dt.Ingest(Record{ID: "c", Fields: map[string]string{"amount": "20"}})

	a := dt.View().Aggregate("amount")
	if a.N != 2 || !eq(a.Sum, 30) || len(a.Anomalies) != 0 {
		t.Fatalf("agg = %+v, want N 2 sum 30 no anomalies (blank is absence)", a)
	}
}

func TestAggregateEmptyIsZeroValued(t *testing.T) {
	a := New().View().Aggregate("amount")
	if a.N != 0 || a.Sum != 0 || a.Min != 0 || a.Max != 0 || a.Mean != 0 {
		t.Fatalf("empty agg = %+v, want all zero", a)
	}
}

func TestAggregateUnknownFieldIsZeroValued(t *testing.T) {
	a := amounts("10", "20").View().Aggregate("ghost")
	if a.N != 0 || len(a.Anomalies) != 0 {
		t.Fatalf("unknown-field agg = %+v, want zero", a)
	}
}

// Aggregate over a followed table column: the reduction runs on the loop rows,
// not the parent forms.
func TestAggregateOverFollowedTableColumn(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "f", Tables: map[string][]map[string]string{
		"items": {{"cost": "100"}, {"cost": "50"}, {"cost": "25"}},
	}})

	a := dt.View().Follow("items").Aggregate("cost")
	if a.N != 3 || !eq(a.Sum, 175) || !eq(a.Min, 25) || !eq(a.Max, 100) {
		t.Fatalf("followed agg = %+v, want N 3 sum 175 min 25 max 100", a)
	}
}
