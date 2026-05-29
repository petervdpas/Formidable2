package datacore

import "testing"

func datedForms() *Tensor {
	dt := New()
	dates := map[string]string{
		"a": "2026-01-15",
		"b": "2026-01-20",
		"c": "2026-02-05",
		"d": "2027-03-01",
	}
	for id, due := range dates {
		dt.Ingest(Record{ID: id, Fields: map[string]string{"due": due}})
	}
	return dt
}

func seriesMap(s Series) map[string]int {
	m := map[string]int{}
	for _, b := range s.Buckets {
		m[b.Value] = b.Count
	}
	return m
}

func TestDateSeriesByMonth(t *testing.T) {
	s := datedForms().View().DateSeries("due", "month")
	if s.Period != "month" {
		t.Fatalf("period = %q, want month", s.Period)
	}
	m := seriesMap(s)
	if m["2026-01"] != 2 || m["2026-02"] != 1 || m["2027-03"] != 1 || len(m) != 3 {
		t.Fatalf("month series = %v, want 2026-01:2 2026-02:1 2027-03:1", m)
	}
}

func TestDateSeriesByYear(t *testing.T) {
	m := seriesMap(datedForms().View().DateSeries("due", "year"))
	if m["2026"] != 3 || m["2027"] != 1 || len(m) != 2 {
		t.Fatalf("year series = %v, want 2026:3 2027:1", m)
	}
}

func TestDateSeriesByDay(t *testing.T) {
	m := seriesMap(datedForms().View().DateSeries("due", "day"))
	if len(m) != 4 || m["2026-01-15"] != 1 {
		t.Fatalf("day series = %v, want 4 distinct days", m)
	}
}

func TestDateSeriesDefaultsToMonth(t *testing.T) {
	if s := datedForms().View().DateSeries("due", "decade"); s.Period != "month" {
		t.Fatalf("unknown period normalized to %q, want month", s.Period)
	}
}

func TestDateSeriesSurfacesNonDateAsAnomaly(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"due": "2026-01-15"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{"due": "someday"}})

	s := dt.View().DateSeries("due", "month")
	if len(s.Buckets) != 1 || s.Buckets[0].Value != "2026-01" {
		t.Fatalf("buckets = %v, want one 2026-01", s.Buckets)
	}
	if len(s.Anomalies) != 1 || s.Anomalies[0].ID != "b" || s.Anomalies[0].Value != "someday" {
		t.Fatalf("anomalies = %v, want one b/someday", s.Anomalies)
	}
}

func TestDateSeriesSkipsBlankAndUnknownField(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "a", Fields: map[string]string{"due": "2026-01-15"}})
	dt.Ingest(Record{ID: "b", Fields: map[string]string{}})

	if m := seriesMap(dt.View().DateSeries("due", "month")); m["2026-01"] != 1 || len(m) != 1 {
		t.Fatalf("series = %v, want one 2026-01 (blank skipped)", m)
	}
	if s := dt.View().DateSeries("ghost", "month"); len(s.Buckets) != 0 {
		t.Fatalf("unknown field series = %v, want empty", s.Buckets)
	}
}

// Date series over a followed table column: bucket the loop rows, not forms.
func TestDateSeriesOverFollowedColumn(t *testing.T) {
	dt := New()
	dt.Ingest(Record{ID: "f", Tables: map[string][]map[string]string{
		"events": {{"when": "2026-01-10"}, {"when": "2026-01-28"}, {"when": "2026-02-02"}},
	}})

	m := seriesMap(dt.View().Follow("events").DateSeries("when", "month"))
	if m["2026-01"] != 2 || m["2026-02"] != 1 {
		t.Fatalf("followed series = %v, want 2026-01:2 2026-02:1", m)
	}
}
