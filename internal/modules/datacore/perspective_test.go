package datacore

import "testing"

// A record with a table: each row gets its own identity and the record
// references the rows, so Follow walks record -> rows and the row columns
// project independently. This is the table-row-identity property EAV drops.
func withTable() *Tensor {
	dt := New()
	dt.Ingest(Record{
		ID:     "form1",
		Fields: map[string]string{"title": "Audit"},
		Tables: map[string][]map[string]string{
			"items": {
				{"name": "disk", "cost": "100"},
				{"name": "ram", "cost": "50"},
			},
		},
	})
	return dt
}

func TestFollowWalksRecordToTableRows(t *testing.T) {
	dt := withTable()

	rows := dt.View().Follow("items").Project("name", "cost")
	if len(rows) != 2 {
		t.Fatalf("table rows = %d, want 2", len(rows))
	}
	// Sorted by synthetic row identity: form1#items#0, form1#items#1.
	if rows[0].Cells[0] != "disk" || rows[0].Cells[1] != "100" {
		t.Fatalf("row0 = %+v, want disk/100", rows[0])
	}
	if rows[1].Cells[0] != "ram" || rows[1].Cells[1] != "50" {
		t.Fatalf("row1 = %+v, want ram/50", rows[1])
	}
}

func TestRowsAreDistinctIdentities(t *testing.T) {
	dt := withTable()

	rows := dt.View().Follow("items").Project("name")
	if rows[0].ID == rows[1].ID {
		t.Fatalf("table rows share identity %q; rows must be distinct", rows[0].ID)
	}
}

func TestFollowUnknownReferenceYieldsEmpty(t *testing.T) {
	dt := withTable()

	if n := dt.View().Follow("missing").Count(); n != 0 {
		t.Fatalf("follow unknown ref count = %d, want 0", n)
	}
}

func TestParentRecordExcludesRowColumns(t *testing.T) {
	dt := withTable()

	// The parent identity carries title, not the row columns.
	rows := dt.View().Where("title", func(v string) bool { return v == "Audit" }).Project("name")
	if len(rows) != 1 || rows[0].ID != "form1" {
		t.Fatalf("parent rows = %+v, want one form1", rows)
	}
	if rows[0].Cells[0] != "" {
		t.Fatalf("parent name = %q, want blank (row column, not parent)", rows[0].Cells[0])
	}
}

// Count on an empty contingency (unknown axis field) is always zero.
func TestCrossCountOnEmptyTabIsZero(t *testing.T) {
	dt := New()
	ingestAll(dt, sampleRecords())

	empty := dt.View().Cross("team", "ghost")
	if empty.Count("east", "x") != 0 {
		t.Fatal("Count on empty cross must be 0")
	}
}

// With no roots marked (raw Put usage without Ingest), the working set falls
// back to every distinct non-satellite identity in first-seen order, never
// satellites.
func TestIdentitiesFallbackWhenNoRootsMarked(t *testing.T) {
	dt := New()
	// Put writes cells directly without marking a root (no Ingest).
	dt.Put("x", "team", Universal, "east")
	dt.Put("y", "team", Universal, "west")

	v := dt.View()
	if v.Count() != 2 {
		t.Fatalf("no-roots fallback count = %d, want 2", v.Count())
	}
	dist := v.Distribution("team")
	if len(dist) != 2 {
		t.Fatalf("no-roots fallback distribution = %+v, want two buckets", dist)
	}
}

func TestDistributionOverFollowedRows(t *testing.T) {
	dt := New()
	dt.Ingest(Record{
		ID: "f",
		Tables: map[string][]map[string]string{
			"items": {
				{"kind": "hw"}, {"kind": "hw"}, {"kind": "sw"},
			},
		},
	})

	dist := dt.View().Follow("items").Distribution("kind")
	want := map[string]int{"hw": 2, "sw": 1}
	for _, b := range dist {
		if want[b.Value] != b.Count {
			t.Fatalf("kind %q = %d, want %d", b.Value, b.Count, want[b.Value])
		}
	}
}
