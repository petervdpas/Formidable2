package stat

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

// appCell is one Code-repositories row contributing an Application value in
// column 0 (matching the resolver below).
func appCell(app string) index.FormValueRow {
	c := 0
	return index.FormValueRow{FieldKey: "code-repositories", Col: &c, ValueType: "text", Text: app}
}

func odsForm(file string, apps ...string) index.FormRow {
	vals := make([]index.FormValueRow, 0, len(apps))
	for _, a := range apps {
		vals = append(vals, appCell(a))
	}
	return index.FormRow{Template: "ods.yaml", Filename: file, Mtime: 1, Values: vals}
}

func odsManager(t *testing.T, forms []index.FormRow) *Manager {
	m := NewManager(datacoreBackend(forms))
	m.SetColumnResolver(fakeColResolver{idx: map[string]int{"code-repositories.application": 0}})
	return m
}

// TestRecords_EndToEnd_ODSHeaviness is the real scenario: across storage
// items, rank applications by how often the name appears (count), and carry
// the heaviness (distinct storage-items hit = records) alongside. A record
// listing the same application on two components must count twice for
// mentions but once for records.
func TestRecords_EndToEnd_ODSHeaviness(t *testing.T) {
	forms := []index.FormRow{
		// FMU appears 5 times across 3 distinct storage-items
		// (x lists it twice; y twice; z once).
		odsForm("x.meta.json", "FMU", "FMU", "Gradework"),
		odsForm("y.meta.json", "FMU", "FMU"),
		odsForm("z.meta.json", "FMU", "jouwfontys"),
		// Gradework: 1 mention, 1 record. jouwfontys: 1 mention, 1 record.
		odsForm("w.meta.json", "jouwfontys"),
	}
	m := odsManager(t, forms)

	g, err := m.EvaluateDSL("ods.yaml", `count(), records() by F["code-repositories"]["application"] top 10`)
	if err != nil {
		t.Fatal(err)
	}
	if g.Total != 4 {
		t.Errorf("total = %d, want 4 storage-items", g.Total)
	}
	type pair struct{ count, records float64 }
	got := map[string]pair{}
	for _, c := range g.Cells {
		got[g.Axes[0].Labels[c.Coords[0]]] = pair{c.Values[0], c.Values[1]}
	}
	// FMU: 5 mentions, 3 distinct storage-items hit.
	if p := got["FMU"]; p.count != 5 || p.records != 3 {
		t.Errorf("FMU = %+v, want {count:5 records:3}", p)
	}
	// jouwfontys: 2 mentions (z + w), 2 records.
	if p := got["jouwfontys"]; p.count != 2 || p.records != 2 {
		t.Errorf("jouwfontys = %+v, want {count:2 records:2}", p)
	}
	// Gradework: 1 mention, 1 record.
	if p := got["Gradework"]; p.count != 1 || p.records != 1 {
		t.Errorf("Gradework = %+v, want {count:1 records:1}", p)
	}
	// All three categories present (top 10 does not cap only 3 of them;
	// count-ranking is exercised by the top-N tests, where a tail drops).
	if len(g.Axes[0].Labels) != 3 {
		t.Errorf("labels = %v, want all 3 applications", g.Axes[0].Labels)
	}
}

// TestRecords_EndToEnd_NeverExceedsTotalForms is the invariant the user
// cares about: an application's heaviness can never report more
// storage-items than exist, no matter how many rows it is mentioned on.
func TestRecords_EndToEnd_NeverExceedsTotalForms(t *testing.T) {
	// One storage-item lists the same application on 50 components.
	apps := make([]string, 50)
	for i := range apps {
		apps[i] = "Monolith"
	}
	m := odsManager(t, []index.FormRow{odsForm("solo.meta.json", apps...)})

	g, err := m.EvaluateDSL("ods.yaml", `count(), records() by F["code-repositories"]["application"]`)
	if err != nil {
		t.Fatal(err)
	}
	c := findCell(g, 0)
	if c == nil {
		t.Fatal("no cell for Monolith")
	}
	if c.Values[0] != 50 {
		t.Errorf("count = %v, want 50 mentions", c.Values[0])
	}
	if c.Values[1] != 1 {
		t.Errorf("records = %v, want 1 (a single storage-item, deduped)", c.Values[1])
	}
	if c.Values[1] > float64(g.Total) {
		t.Errorf("records %v exceeds total forms %d", c.Values[1], g.Total)
	}
}

// TestRecords_EndToEnd_ScalarDimEqualsCount: on a scalar (one-row-per-form)
// dimension there is no fan-out, so records() and count() must agree.
func TestRecords_EndToEnd_ScalarDimEqualsCount(t *testing.T) {
	statusVal := func(s string) index.FormValueRow {
		return index.FormValueRow{FieldKey: "status", ValueType: "text", Text: s}
	}
	forms := []index.FormRow{
		{Template: "ods.yaml", Filename: "a.meta.json", Mtime: 1, Values: []index.FormValueRow{statusVal("active")}},
		{Template: "ods.yaml", Filename: "b.meta.json", Mtime: 1, Values: []index.FormValueRow{statusVal("active")}},
		{Template: "ods.yaml", Filename: "c.meta.json", Mtime: 1, Values: []index.FormValueRow{statusVal("retired")}},
	}
	m := NewManager(datacoreBackend(forms))

	g, err := m.EvaluateDSL("ods.yaml", `count(), records() by F["status"]`)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Cells {
		if c.Values[0] != c.Values[1] {
			t.Errorf("%s: count %v != records %v (scalar dim must not fan out)",
				g.Axes[0].Labels[c.Coords[0]], c.Values[0], c.Values[1])
		}
	}
}

// TestRecords_EndToEnd_RankByRecordsThenTopN: ranking by records() (heaviness
// first) selects a different top-N than ranking by mentions when they
// diverge. Proves top-N keys off the first measure consistently.
func TestRecords_EndToEnd_RankByRecordsThenTopN(t *testing.T) {
	forms := []index.FormRow{
		// Heavy: hits 3 distinct storage-items, 3 mentions.
		odsForm("a.meta.json", "Heavy"),
		odsForm("b.meta.json", "Heavy"),
		odsForm("c.meta.json", "Heavy"),
		// Noisy: 4 mentions but only 1 storage-item.
		odsForm("d.meta.json", "Noisy", "Noisy", "Noisy", "Noisy"),
	}
	m := odsManager(t, forms)

	g, err := m.EvaluateDSL("ods.yaml", `records(), count() by F["code-repositories"]["application"] top 1`)
	if err != nil {
		t.Fatal(err)
	}
	// records() is first, so Heavy (3 records) beats Noisy (1 record),
	// even though Noisy has more mentions.
	if want := []string{"Heavy"}; !equalStrs(g.Axes[0].Labels, want) {
		t.Errorf("labels = %v, want %v (ranked by records, not mentions)", g.Axes[0].Labels, want)
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
