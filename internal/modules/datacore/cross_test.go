package datacore

import "testing"

// region x tier, with one record missing a tier (complete-case drop).
func crossFixture() *Tensor {
	dt := New()
	recs := []Record{
		{ID: "a", Fields: map[string]string{"region": "east", "tier": "gold"}},
		{ID: "b", Fields: map[string]string{"region": "east", "tier": "gold"}},
		{ID: "c", Fields: map[string]string{"region": "east", "tier": "silver"}},
		{ID: "d", Fields: map[string]string{"region": "west", "tier": "gold"}},
		{ID: "e", Fields: map[string]string{"region": "west"}}, // no tier: dropped
	}
	for _, r := range recs {
		dt.Ingest(r)
	}
	return dt
}

func TestCrossBinsByTwoFields(t *testing.T) {
	ct := crossFixture().View().Cross("region", "tier")

	if got := ct.Count("east", "gold"); got != 2 {
		t.Fatalf("east/gold = %d, want 2", got)
	}
	if got := ct.Count("east", "silver"); got != 1 {
		t.Fatalf("east/silver = %d, want 1", got)
	}
	if got := ct.Count("west", "gold"); got != 1 {
		t.Fatalf("west/gold = %d, want 1", got)
	}
	if got := ct.Count("west", "silver"); got != 0 {
		t.Fatalf("west/silver = %d, want 0 (absent cell)", got)
	}
}

func TestCrossAxesAreSortedDistinctValues(t *testing.T) {
	ct := crossFixture().View().Cross("region", "tier")

	if len(ct.Rows) != 2 || ct.Rows[0] != "east" || ct.Rows[1] != "west" {
		t.Fatalf("rows = %v, want [east west]", ct.Rows)
	}
	if len(ct.Cols) != 2 || ct.Cols[0] != "gold" || ct.Cols[1] != "silver" {
		t.Fatalf("cols = %v, want [gold silver]", ct.Cols)
	}
}

func TestCrossDropsIncompleteIdentities(t *testing.T) {
	ct := crossFixture().View().Cross("region", "tier")
	sum := 0
	for _, c := range ct.Cells {
		sum += c.Count
	}
	// 5 records, but "e" has no tier, so 4 complete cases enter the table.
	if sum != 4 {
		t.Fatalf("cross-tab total = %d, want 4 (e dropped)", sum)
	}
}

// The defining identity: a margin of the rank-2 cross-tab equals the rank-1
// Distribution along that axis, computed over the same complete cases. The
// cross-tab is the joint; the distribution is its contraction.
func TestCrossMarginsEqualDistributions(t *testing.T) {
	dt := crossFixture()
	ct := dt.View().Cross("region", "tier")

	// Row margins: sum each region across tiers.
	rowMargin := map[string]int{}
	for _, c := range ct.Cells {
		rowMargin[c.Row] += c.Count
	}
	// Distribution of region over the same complete cases (those with a tier).
	regionDist := dt.View().Where("tier", func(v string) bool { return v != "" }).Distribution("region")
	for _, b := range regionDist {
		if rowMargin[b.Value] != b.Count {
			t.Fatalf("region margin %q = %d, distribution = %d", b.Value, rowMargin[b.Value], b.Count)
		}
	}

	// Column margins likewise contract to the tier distribution.
	colMargin := map[string]int{}
	for _, c := range ct.Cells {
		colMargin[c.Col] += c.Count
	}
	tierDist := dt.View().Where("region", func(v string) bool { return v != "" }).Distribution("tier")
	for _, b := range tierDist {
		if colMargin[b.Value] != b.Count {
			t.Fatalf("tier margin %q = %d, distribution = %d", b.Value, colMargin[b.Value], b.Count)
		}
	}
}

func TestCrossUnknownFieldYieldsEmpty(t *testing.T) {
	ct := crossFixture().View().Cross("region", "ghost")
	if len(ct.Cells) != 0 || len(ct.Cols) != 0 {
		t.Fatalf("cross over unknown field = %+v, want empty", ct)
	}
}
