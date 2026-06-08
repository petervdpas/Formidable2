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

func TestListCollection_FieldFilter_ViaValueIndex(t *testing.T) {
	// opts.Filter narrows by a data field through the value index (the fake
	// mirrors form_values: eq/ne on Text, compare on Num). Facet routing is the
	// caller's job, so this exercises the data-field path only.
	num := func(n float64) *float64 { return &n }
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "a.meta.json", ID: "g-a", Title: "A",
					Values: []index.FormValueRow{
						{FieldKey: "status", ValueType: "text", Text: "done"},
						{FieldKey: "amount", ValueType: "number", Num: num(100), Text: "100"},
					}},
				{Template: "recepten.yaml", Filename: "b.meta.json", ID: "g-b", Title: "B",
					Values: []index.FormValueRow{
						{FieldKey: "status", ValueType: "text", Text: "todo"},
						{FieldKey: "amount", ValueType: "number", Num: num(300), Text: "300"},
					}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)

	// eq on a text/dropdown-style field.
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Filter: &CollectionFieldFilter{FieldKey: "status", Op: "eq", Value: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Filename != "a.meta.json" {
		t.Errorf("status eq done got %+v, want a only", page.Items)
	}

	// numeric compare.
	page2, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Filter: &CollectionFieldFilter{FieldKey: "amount", Op: "ge", Value: "200"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].Filename != "b.meta.json" {
		t.Errorf("amount ge 200 got %+v, want b only", page2.Items)
	}

	// Unset filter leaves the list unchanged.
	page3, err := m.ListCollection(context.Background(), "recepten.yaml", CollectionListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page3.Items) != 2 {
		t.Errorf("no filter got %d items, want 2", len(page3.Items))
	}
}

// TestListCollection_FieldFilterValueIndexErrorPropagates covers the
// error return from the value-index predicate (collection.go routes a
// failed FormsWithValueOp straight out). The fake mirrors the real index:
// a numeric comparison op with an unparseable value errors, and ListCollection
// must surface it rather than swallowing it into an empty page.
func TestListCollection_FieldFilterValueIndexErrorPropagates(t *testing.T) {
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "a.meta.json", ID: "g-a", Title: "A",
					Values: []index.FormValueRow{
						{FieldKey: "amount", ValueType: "number", Text: "100"},
					}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)

	_, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Filter: &CollectionFieldFilter{
			FieldKey: "amount", Op: "ge", Value: "not-a-number"}})
	if err == nil {
		t.Fatal("expected error: numeric op with unparseable value must propagate")
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

	// An empty id must never resolve (so an empty relation-edge target can't
	// match a guid-less record).
	_, ok, _ = m.ResolveCollectionByID(context.Background(), "recepten.yaml", "")
	if ok {
		t.Errorf("empty id must not resolve")
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

// CollectionListOpts facet filter boundaries.

func facetIndexFixture() *fakeIndex {
	return &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "tap.meta.json",
					ID: "g-tap", Title: "Tap",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: "DONE"},
						{Key: "size", Set: true, Selected: "BIG"},
					}},
				{Template: "recepten.yaml", Filename: "vin.meta.json",
					ID: "g-vin", Title: "Vin",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: "TODO"},
						{Key: "size", Set: true, Selected: "SMALL"},
					}},
				{Template: "recepten.yaml", Filename: "spa.meta.json",
					ID: "g-spa", Title: "Spa",
					Facets: []index.FormFacet{
						{Key: "status", Set: false, Selected: "DONE"},
					}},
				{Template: "recepten.yaml", Filename: "emp.meta.json",
					ID: "g-emp", Title: "Emp",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: ""},
					}},
			},
		},
	}
}

func collectFilenames(items []CollectionItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Filename)
	}
	return out
}

func TestListCollection_FacetSetFalseNeverMatches(t *testing.T) {
	// spa has status set=false selected=DONE: must not match status=DONE.
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "DONE"}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectFilenames(page.Items)
	if !equalSorted(got, []string{"tap.meta.json"}) {
		t.Errorf("status=DONE matched %v, want only tap.meta.json", got)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

func TestListCollection_FacetEmptyValueMatchesEmptySelected(t *testing.T) {
	// emp has status set=true selected="": filtering for status=="" must
	// match it exactly and nothing else (the others have non-empty values
	// or set=false).
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": ""}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectFilenames(page.Items)
	if !equalSorted(got, []string{"emp.meta.json"}) {
		t.Errorf("status=\"\" matched %v, want only emp.meta.json", got)
	}
}

func TestListCollection_MultiFacetANDOneFails(t *testing.T) {
	// vin satisfies status=TODO but its size is SMALL, not BIG: the AND
	// must drop it. tap satisfies status=DONE but the requested status is
	// TODO, so the only-both-pass set is empty.
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "TODO", "size": "BIG"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("status=TODO&size=BIG matched %v, want empty", collectFilenames(page.Items))
	}
}

func TestListCollection_UnknownFacetKeyMatchesNothing(t *testing.T) {
	// No row carries a "color" facet: requesting one yields zero rows.
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"color": "RED"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("unknown facet matched %v, want empty", collectFilenames(page.Items))
	}
}

func TestListCollection_EmptyQDoesNotFilter(t *testing.T) {
	// Empty Q must skip the substring filter entirely: all 4 rows survive
	// (all are addressable, none has a facet filter applied).
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Q: ""})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 {
		t.Errorf("empty Q Total = %d, want 4 (no filtering)", page.Total)
	}
	got := collectFilenames(page.Items)
	want := []string{"emp.meta.json", "spa.meta.json", "tap.meta.json", "vin.meta.json"}
	if !equalSorted(got, want) {
		t.Errorf("empty Q items = %v, want all four %v", got, want)
	}
	if !page.Enabled {
		t.Errorf("page should be Enabled")
	}
}

func TestListCollection_QNoMatchEmptyButEnabled(t *testing.T) {
	// A Q that matches nothing yields zero items, but the page is still
	// Enabled with the stem set.
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Q: "zzz-no-such-needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Enabled {
		t.Errorf("page should stay Enabled even with empty result")
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("Total = %d items = %v, want 0", page.Total, collectFilenames(page.Items))
	}
	if page.Template != "recepten" {
		t.Errorf("Template = %q, want recepten", page.Template)
	}
}

func TestListCollection_OffsetBeyondEndYieldsNoItemsButRealTotal(t *testing.T) {
	// Offset past the row count empties Items but Total still reflects the
	// pre-paginate count.
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Offset: 99})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 {
		t.Errorf("Total = %d, want 4 (pre-paginate)", page.Total)
	}
	if len(page.Items) != 0 {
		t.Errorf("Items = %v, want empty for out-of-range offset", collectFilenames(page.Items))
	}
}

func TestListCollection_RowWithoutGuidDropped(t *testing.T) {
	// A form row with an empty ID cannot be addressed in /api/collections
	// and must be filtered out before pagination.
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "ok.meta.json", ID: "g-ok", Title: "Ok"},
				{Template: "recepten.yaml", Filename: "noid.meta.json", ID: "", Title: "NoId"},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml", CollectionListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	got := collectFilenames(page.Items)
	if !equalSorted(got, []string{"ok.meta.json"}) {
		t.Errorf("got %v, want only ok.meta.json (guid-less dropped)", got)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

// Multi-facet AND where the second facet is set=false on the only row that
// satisfies the first: the set=false facet must veto, leaving zero rows.
func TestListCollection_MultiFacetSetFalseVetoes(t *testing.T) {
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "tap.meta.json",
					ID: "g-tap", Title: "Tap",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: "DONE"},
						{Key: "size", Set: false, Selected: "BIG"},
					}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "DONE", "size": "BIG"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Errorf("set=false size facet should veto; got %v", collectFilenames(page.Items))
	}
}

// Two facets that both pass, one matching an empty selected value, the other
// a non-empty value: both AND conditions hold so the row survives.
func TestListCollection_MultiFacetEmptyAndNonEmptyBothPass(t *testing.T) {
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "recepten.yaml", Name: "Recepten",
				GuidField: "id", TagsField: "tags", ItemField: "title",
				EnableCollection: true},
		},
		forms: map[string][]index.FormRow{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "both.meta.json",
					ID: "g-both", Title: "Both",
					Facets: []index.FormFacet{
						{Key: "status", Set: true, Selected: ""},
						{Key: "size", Set: true, Selected: "BIG"},
					}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Facets: map[string]string{"status": "", "size": "BIG"}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectFilenames(page.Items)
	if !equalSorted(got, []string{"both.meta.json"}) {
		t.Errorf("status=\"\"&size=BIG matched %v, want both.meta.json", got)
	}
	if page.Total != 1 {
		t.Errorf("Total = %d, want 1", page.Total)
	}
}

// Limit larger than the result count must not truncate: all matching rows
// come back and Total reflects the full pre-paginate count.
func TestListCollection_LimitBeyondResultReturnsAll(t *testing.T) {
	m := newManagerWithFakes(facetIndexFixture(), nil)
	page, err := m.ListCollection(context.Background(), "recepten.yaml",
		CollectionListOpts{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 {
		t.Errorf("Total = %d, want 4", page.Total)
	}
	if len(page.Items) != 4 {
		t.Errorf("Items = %d, want 4 (limit exceeds count)", len(page.Items))
	}
	if page.Limit != 100 {
		t.Errorf("Limit roundtrip = %d, want 100", page.Limit)
	}
}
