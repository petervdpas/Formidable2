package dataprovider

import (
	"context"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
)

func collectionsFixture() *fakeIndex {
	return &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
			{Filename: "plain.yaml", Name: "Plain"}, // no collection
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "tap.meta.json",
					ID: "g-tap", Title: "Groene Tapenade",
					Tags: []string{"groen", "tapenade"}, Updated: "2026-04-01"},
				{Template: "recepten.yaml", Filename: "vin.meta.json",
					ID: "g-vin", Title: "Basis Vinaigrette",
					Tags: []string{"saus"}, Updated: "2026-03-01"},
				{Template: "recepten.yaml", Filename: "spa.meta.json",
					ID: "g-spa", Title: "Spaanse Groenteschotel",
					Tags: []string{"groen", "spaans"}, Updated: "2026-02-01"},
			},
		},
	}
}

func TestIsCollectionEnabled(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	if !m.IsCollectionEnabled(context.Background(), "recepten.yaml") {
		t.Errorf("recepten.yaml should be collection-enabled")
	}
	if m.IsCollectionEnabled(context.Background(), "plain.yaml") {
		t.Errorf("plain.yaml should NOT be collection-enabled")
	}
	if m.IsCollectionEnabled(context.Background(), "ghost.yaml") {
		t.Errorf("unknown template should NOT be collection-enabled")
	}
}

func TestListCollection_DisabledTemplate(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	page, err := m.ListCollection(context.Background(), "plain.yaml", CollectionListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Enabled {
		t.Errorf("plain.yaml shouldn't report Enabled=true")
	}
}

func TestListCollection_BasicShape(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	page, err := m.ListCollection(context.Background(), "recepten.yaml", CollectionListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Enabled {
		t.Fatalf("recepten.yaml should report Enabled=true")
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}
	if page.Template != "recepten" {
		t.Errorf("Template = %q, want recepten (stem)", page.Template)
	}
	if len(page.Items) != 3 {
		t.Fatalf("Items = %d, want 3", len(page.Items))
	}

	// Each item must have correct hrefs and identity.
	by := map[string]CollectionItem{}
	for _, it := range page.Items {
		by[it.ID] = it
	}
	if it, ok := by["g-tap"]; !ok || it.Filename != "tap.meta.json" || it.Title != "Groene Tapenade" {
		t.Errorf("g-tap shape wrong: %+v", it)
	} else {
		if it.HrefSelf != "/api/collections/recepten/g-tap" {
			t.Errorf("hrefSelf = %q", it.HrefSelf)
		}
		if it.HrefHTML != "/template/recepten/form/tap.meta.json" {
			t.Errorf("hrefHTML = %q", it.HrefHTML)
		}
	}
}

func TestListCollection_QSubstring_TitleAndTags(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	// "groen" matches:
	//   - title "Spaanse Groenteschotel" (case-insensitive substring)
	//   - tag "groen" on tap and spa
	// → tap and spa, not vinaigrette.
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Q: "GROEN"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range page.Items {
		got = append(got, it.Filename)
	}
	sort.Strings(got)
	want := []string{"spa.meta.json", "tap.meta.json"}
	if !equalSorted(got, want) {
		t.Errorf("q=GROEN matched %v, want %v", got, want)
	}
}

func TestListCollection_TagsAND(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	// "groen" + "spaans" → only spa.
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Tags: []string{"groen", "spaans"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Filename != "spa.meta.json" {
		t.Errorf("got %+v, want spa.meta.json only", page.Items)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestListCollection_FacetFilter_ReadsFromFormRow(t *testing.T) {
	// Verifies the disk-read replacement: when opts.Facets is set, the
	// filter must source meta from the FormRow.Facets the index already
	// materialized - no storage.LoadForm calls. The fakeIndex below
	// supplies facets on the row, and crucially no storage adapter is
	// wired (newManagerWithFakes passes nil) so a regression to the
	// old disk-read path would crash.
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "tap.meta.json",
					ID: "g-tap", Title: "Tap", Updated: "2026-04-01",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: "DONE"},
						{Key: "size", Set: true, Selected: "BIG"},
					}},
				{Template: "recepten.yaml", Filename: "vin.meta.json",
					ID: "g-vin", Title: "Vin", Updated: "2026-03-01",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: "TODO"},
					}},
				{Template: "recepten.yaml", Filename: "spa.meta.json",
					ID: "g-spa", Title: "Spa", Updated: "2026-02-01",
					Facets: []index.FormFacet{
						{Key: "status", Set: false, Selected: "DONE"},
					}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)

	// Single-facet filter - only tap matches.
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "DONE"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Filename != "tap.meta.json" {
		t.Errorf("status=DONE filter got %+v, want tap only", page.Items)
	}

	// Multi-facet AND - only the row that satisfies BOTH passes.
	page2, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "DONE", "size": "BIG"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].Filename != "tap.meta.json" {
		t.Errorf("status=DONE&size=BIG filter got %+v, want tap only", page2.Items)
	}

	// set=false must NOT count as a match even when the selected value
	// equals the filter - the facet has to be actively stamped.
	page3, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "DONE"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page3.Items {
		if it.Filename == "spa.meta.json" {
			t.Errorf("spa matched status=DONE despite set=false")
		}
	}
}

func TestListCollection_LimitOffset(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3 (pre-paginate)", page.Total)
	}
	if len(page.Items) != 1 {
		t.Errorf("Items = %d, want 1 (limit=1)", len(page.Items))
	}
	if page.Limit != 1 || page.Offset != 1 {
		t.Errorf("Limit/Offset roundtrip wrong: %+v", page)
	}
}

func TestResolveCollectionByID(t *testing.T) {
	idx := collectionsFixture()
	m := newManagerWithFakes(idx, nil)

	got, ok, err := m.ResolveCollectionByID(context.Background(), "recepten.yaml", "g-tap")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.Filename != "tap.meta.json" {
		t.Errorf("filename wrong: %+v", got)
	}
	if got.HrefSelf != "/api/collections/recepten/g-tap" {
		t.Errorf("hrefSelf = %q", got.HrefSelf)
	}

	_, ok, _ = m.ResolveCollectionByID(context.Background(), "recepten.yaml", "no-such")
	if ok {
		t.Errorf("expected not-found")
	}

	// Plain (no collection) → not found.
	_, ok, _ = m.ResolveCollectionByID(context.Background(), "plain.yaml", "anything")
	if ok {
		t.Errorf("plain.yaml has no collection - should miss")
	}
}

func TestCollectionRev_BumpsWithIndexRev(t *testing.T) {
	idx := collectionsFixture()
	idx.rev = 7
	m := newManagerWithFakes(idx, nil)

	got, err := m.CollectionRev(context.Background(), "recepten.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Errorf("got %d, want 7 (mirrors index.Rev for v1)", got)
	}
}
