package dataprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/storage"
)

// API-field error taxonomy: each failure mode must wrap its exact sentinel
// so the Wails layer can errors.Is and map to a stable code.

func TestFetchAPIFieldRow_TemplateNotFound_ExactSentinel(t *testing.T) {
	m, _, _ := newAPIFieldWorld()
	_, err := m.FetchAPIFieldRow(context.Background(),
		"ghost.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldTemplateNotFound) {
		t.Fatalf("err = %v; want ErrAPIFieldTemplateNotFound", err)
	}
	if errors.Is(err, ErrAPIFieldCollectionDisabled) ||
		errors.Is(err, ErrAPIFieldGuidNotFound) ||
		errors.Is(err, ErrAPIFieldStorageMissing) {
		t.Errorf("err = %v matched a wrong sentinel", err)
	}
}

func TestFetchAPIFieldRow_GuidFieldEmptyDisablesCollection(t *testing.T) {
	// EnableCollection=true but GuidField=="" must NOT enable collection,
	// so the call surfaces ErrAPIFieldCollectionDisabled.
	m, idx, _ := newAPIFieldWorld()
	idx.templates = append(idx.templates, index.TemplateRow{
		Filename:         "people.yaml",
		EnableCollection: true,
		GuidField:        "",
	})
	_, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldCollectionDisabled) {
		t.Fatalf("err = %v; want ErrAPIFieldCollectionDisabled", err)
	}
	if errors.Is(err, ErrAPIFieldTemplateNotFound) {
		t.Errorf("err = %v matched TemplateNotFound; template exists", err)
	}
}

func TestFetchAPIFieldRow_StorageMissing_ExactSentinel(t *testing.T) {
	// Collection enabled and the guid resolves to a datafile, but no
	// storage adapter is wired (sto==nil): must return the bare
	// ErrAPIFieldStorageMissing sentinel.
	idx := &fakeIndex{forms: map[string][]index.FormRow{}}
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	m := NewManager(idx, &fakeRenderer{}, nil)

	_, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldStorageMissing) {
		t.Fatalf("err = %v; want ErrAPIFieldStorageMissing", err)
	}
	if errors.Is(err, ErrAPIFieldGuidNotFound) {
		t.Errorf("err = %v matched GuidNotFound; guid did resolve", err)
	}
}

func TestFetchAPIFieldRow_StaleIndexFormGone_MapsToGuidNotFound(t *testing.T) {
	// The index knows the guid but storage.LoadForm returns nil (file
	// gone from disk). The facade folds this into ErrAPIFieldGuidNotFound
	// rather than inventing a fifth code.
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	// Deliberately do NOT stamp the form into sto.forms.
	_ = sto

	_, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldGuidNotFound) {
		t.Fatalf("err = %v; want ErrAPIFieldGuidNotFound", err)
	}
	if errors.Is(err, ErrAPIFieldStorageMissing) {
		t.Errorf("err = %v matched StorageMissing; storage was wired", err)
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

// FetchAPIFieldRow must propagate the StorageMissing sentinel rather than
// returning a nil-but-error-free result when no storage adapter is wired.
func TestFetchAPIFieldRow_PropagatesStorageMissing(t *testing.T) {
	idx := &fakeIndex{forms: map[string][]index.FormRow{}}
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	m := NewManager(idx, &fakeRenderer{}, nil)

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldStorageMissing) {
		t.Fatalf("err = %v; want ErrAPIFieldStorageMissing", err)
	}
	if row != nil {
		t.Errorf("row = %+v, want nil on error", row)
	}
}

// Concurrent reads of the same row must be race-free and each return the
// exact same projected value. Guards the read facade under -race.
func TestFetchAPIFieldRow_ConcurrentReadsStable(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice", "age": 42},
	}

	const n = 32
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			row, err := m.FetchAPIFieldRow(context.Background(),
				"people.yaml", "g-1", []string{"name", "age"})
			if err != nil {
				errCh <- err
				return
			}
			if row["name"] != "Alice" || row["age"] != 42 {
				errCh <- errors.New("unexpected projected value")
				return
			}
			errCh <- nil
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent read %d: %v", i, err)
		}
	}
}
