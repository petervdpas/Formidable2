package datadb

import (
	"encoding/json"
	"testing"
)

func sampleRecords() []Record {
	return []Record{
		{
			Template: "kostenplaats.yaml",
			GUID:     "k-1",
			Title:    "Marketing",
			Payload:  map[string]any{"code": "MKT", "budget": 1000},
			Text:     "Marketing MKT budget afdeling",
		},
		{
			Template: "kostenplaats.yaml",
			GUID:     "k-2",
			Title:    "Engineering",
			Payload:  map[string]any{"code": "ENG", "budget": 5000},
			Text:     "Engineering ENG budget afdeling",
		},
		{
			Template: "afdeling.yaml",
			GUID:     "a-1",
			Title:    "Verkoop",
			Payload:  map[string]any{"naam": "Verkoop"},
			Text:     "Verkoop afdeling sales",
		},
	}
}

func openSample(t *testing.T) *DB {
	t.Helper()
	image, err := Build(sampleRecords())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(image) == 0 {
		t.Fatal("Build produced an empty image")
	}
	db, err := Open(image)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestBuildOpenTemplates(t *testing.T) {
	db := openSample(t)
	tcs, err := db.Templates()
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	got := map[string]int{}
	for _, tc := range tcs {
		got[tc.Template] = tc.Count
	}
	if got["kostenplaats.yaml"] != 2 || got["afdeling.yaml"] != 1 {
		t.Fatalf("template counts wrong: %+v", got)
	}
}

func TestRecordsForTemplate(t *testing.T) {
	db := openSample(t)
	recs, err := db.Records("kostenplaats.yaml")
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	// Ordered by title: Engineering before Marketing.
	if recs[0].Title != "Engineering" || recs[1].Title != "Marketing" {
		t.Fatalf("records not ordered by title: %+v", recs)
	}
}

func TestRecordByGUIDFullPayload(t *testing.T) {
	db := openSample(t)
	r, ok, err := db.Record("k-1")
	if err != nil || !ok {
		t.Fatalf("Record k-1: ok=%v err=%v", ok, err)
	}
	if r.Template != "kostenplaats.yaml" || r.Title != "Marketing" {
		t.Fatalf("record meta wrong: %+v", r)
	}
	var payload map[string]any
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		t.Fatalf("payload not valid json: %v", err)
	}
	if payload["code"] != "MKT" {
		t.Fatalf("payload lost fields: %+v", payload)
	}
}

func TestRecordMissing(t *testing.T) {
	db := openSample(t)
	if _, ok, err := db.Record("nope"); ok || err != nil {
		t.Fatalf("missing record: ok=%v err=%v", ok, err)
	}
}

func TestSearchFullText(t *testing.T) {
	db := openSample(t)
	hits, err := db.Search("engineering")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].GUID != "k-2" {
		t.Fatalf("search 'engineering' = %+v, want just k-2", hits)
	}

	// A term shared by many records returns them all.
	all, err := db.Search("afdeling")
	if err != nil {
		t.Fatalf("Search afdeling: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("search 'afdeling' = %d hits, want 3", len(all))
	}
}

func TestSearchEmptyAndPunctuation(t *testing.T) {
	db := openSample(t)
	if hits, err := db.Search("   "); err != nil || len(hits) != 0 {
		t.Fatalf("blank search: hits=%v err=%v", hits, err)
	}
	// Punctuation must not break the FTS syntax.
	if _, err := db.Search(`"quoted" AND (weird)`); err != nil {
		t.Fatalf("punctuation search errored: %v", err)
	}
}

func TestBuildDuplicateGUIDErrors(t *testing.T) {
	dup := []Record{
		{Template: "t.yaml", GUID: "x", Payload: map[string]any{}},
		{Template: "t.yaml", GUID: "x", Payload: map[string]any{}},
	}
	if _, err := Build(dup); err == nil {
		t.Fatal("duplicate (template, guid) should error")
	}
}
