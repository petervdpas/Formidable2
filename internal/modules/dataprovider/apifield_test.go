package dataprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/storage"
)

// ─────────────────────────────────────────────────────────────────────
// flattenAPIValue — pure helper.
// Scalars (string/number/bool/nil) pass through; everything else is
// json.Marshal'd into a string. Keeps host-form storage flat.
// ─────────────────────────────────────────────────────────────────────

func TestFlattenAPIValue_Scalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"string", "hello", "hello"},
		{"empty-string", "", ""},
		{"int", 42, 42},
		{"float", 3.14, 3.14},
		{"bool-true", true, true},
		{"bool-false", false, false},
		{"nil", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := flattenAPIValue(tc.in)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFlattenAPIValue_SlicesAndMapsBecomeJSONStrings(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string-slice", []string{"a", "b", "c"}, `["a","b","c"]`},
		{"any-slice", []any{"a", 1, true}, `["a",1,true]`},
		{"map", map[string]any{"k": "v"}, `{"k":"v"}`},
		{"empty-slice", []any{}, `[]`},
		{"empty-map", map[string]any{}, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := flattenAPIValue(tc.in)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			s, ok := got.(string)
			if !ok {
				t.Fatalf("expected string, got %T (%#v)", got, got)
			}
			if s != tc.want {
				t.Errorf("got %q, want %q", s, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// FetchAPIFieldRow — manager method.
// ─────────────────────────────────────────────────────────────────────

// fakeStorage is the minimal storage adapter dataprovider needs for
// api-field reads. Tests stash forms keyed by (template, datafile).
type fakeStorage struct {
	forms map[string]*storage.Form // "<template>/<datafile>" → form
}

func (f *fakeStorage) LoadForm(template, datafile string) *storage.Form {
	if f.forms == nil {
		return nil
	}
	return f.forms[template+"/"+datafile]
}

// newAPIFieldWorld wires a Manager with the new Storage dependency.
func newAPIFieldWorld() (*Manager, *fakeIndex, *fakeStorage) {
	idx := &fakeIndex{forms: map[string][]index.FormRow{}}
	sto := &fakeStorage{forms: map[string]*storage.Form{}}
	m := NewManager(idx, &fakeRenderer{}, sto)
	return m, idx, sto
}

// seedCollection enables collection mode for `tpl` and stamps one form
// with the given guid pointing at `datafile`.
func seedCollection(idx *fakeIndex, tpl, datafile, guid string) {
	idx.templates = append(idx.templates, index.TemplateRow{
		Filename:         tpl,
		EnableCollection: true,
		GuidField:        "id",
	})
	idx.forms[tpl] = append(idx.forms[tpl], index.FormRow{
		Filename: datafile,
		ID:       guid,
	})
}

func TestFetchAPIFieldRow_ScalarsPassThrough(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{
			"name":  "Alice",
			"email": "alice@a.com",
			"age":   42,
		},
	}

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "email", "age"})
	if err != nil {
		t.Fatalf("FetchAPIFieldRow: %v", err)
	}
	if row["name"] != "Alice" {
		t.Errorf("name: %v", row["name"])
	}
	if row["email"] != "alice@a.com" {
		t.Errorf("email: %v", row["email"])
	}
	if row["age"] != 42 {
		t.Errorf("age: %v", row["age"])
	}
}

func TestFetchAPIFieldRow_TagsFlattenedToJSON(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"tags": []any{"a", "b", "c"}},
	}

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"tags"})
	if err != nil {
		t.Fatalf("FetchAPIFieldRow: %v", err)
	}
	if got := row["tags"]; got != `["a","b","c"]` {
		t.Errorf("tags: %#v, want JSON string", got)
	}
}

func TestFetchAPIFieldRow_MissingColumnIsNil(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice"},
	}

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "absent"})
	if err != nil {
		t.Fatalf("FetchAPIFieldRow: %v", err)
	}
	if _, ok := row["absent"]; !ok {
		t.Errorf("absent column should be present in row (as nil); got missing")
	}
	if row["absent"] != nil {
		t.Errorf("absent column: %v, want nil", row["absent"])
	}
}

func TestFetchAPIFieldRow_UnknownTemplate(t *testing.T) {
	m, _, _ := newAPIFieldWorld()
	_, err := m.FetchAPIFieldRow(context.Background(),
		"ghost.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldTemplateNotFound) {
		t.Errorf("err = %v; want ErrAPIFieldTemplateNotFound", err)
	}
}

func TestFetchAPIFieldRow_CollectionDisabled(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	idx.templates = append(idx.templates, index.TemplateRow{
		Filename:         "notes.yaml",
		EnableCollection: false,
	})
	_, err := m.FetchAPIFieldRow(context.Background(),
		"notes.yaml", "g-1", []string{"name"})
	if !errors.Is(err, ErrAPIFieldCollectionDisabled) {
		t.Errorf("err = %v; want ErrAPIFieldCollectionDisabled", err)
	}
}

func TestFetchAPIFieldRow_GuidNotFound(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	idx.templates = append(idx.templates, index.TemplateRow{
		Filename:         "people.yaml",
		EnableCollection: true,
		GuidField:        "id",
	})
	_, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-missing", []string{"name"})
	if !errors.Is(err, ErrAPIFieldGuidNotFound) {
		t.Errorf("err = %v; want ErrAPIFieldGuidNotFound", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// RefetchAPIFieldRow — drift detection.
// Compares stored projected columns against a fresh fetch and returns
// (fresh row, drift entries). Drift is per-column; both nil → no drift.
// ─────────────────────────────────────────────────────────────────────

func TestRefetchAPIFieldRow_NoDriftWhenStoredMatchesSource(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice", "email": "alice@a.com"},
	}
	stored := map[string]any{"name": "Alice", "email": "alice@a.com"}

	res, err := m.RefetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "email"}, stored)
	if err != nil {
		t.Fatalf("RefetchAPIFieldRow: %v", err)
	}
	if len(res.Drift) != 0 {
		t.Errorf("expected no drift; got %v", res.Drift)
	}
	if res.Row["name"] != "Alice" {
		t.Errorf("row.name = %v", res.Row["name"])
	}
}

func TestRefetchAPIFieldRow_DriftSurfacesChangedColumns(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice Renamed", "email": "alice@a.com"},
	}
	stored := map[string]any{"name": "Alice", "email": "alice@a.com"}

	res, err := m.RefetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "email"}, stored)
	if err != nil {
		t.Fatalf("RefetchAPIFieldRow: %v", err)
	}
	if len(res.Drift) != 1 {
		t.Fatalf("expected 1 drift entry; got %v", res.Drift)
	}
	d := res.Drift[0]
	if d.Key != "name" || d.Stored != "Alice" || d.Current != "Alice Renamed" {
		t.Errorf("drift entry mismatch: %+v", d)
	}
}

func TestRefetchAPIFieldRow_NewColumnAddedToMapShowsAsDrift(t *testing.T) {
	// Map[] was extended after the form was saved — the stored row
	// is missing the new key, the current row has it. Surface as a
	// drift entry with stored=nil so the user sees what's new.
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice", "email": "alice@a.com"},
	}
	stored := map[string]any{"name": "Alice"} // no email yet

	res, err := m.RefetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "email"}, stored)
	if err != nil {
		t.Fatalf("RefetchAPIFieldRow: %v", err)
	}
	if len(res.Drift) != 1 || res.Drift[0].Key != "email" {
		t.Fatalf("expected drift on email; got %+v", res.Drift)
	}
	if res.Drift[0].Stored != nil {
		t.Errorf("stored should be nil for added column; got %v", res.Drift[0].Stored)
	}
	if res.Drift[0].Current != "alice@a.com" {
		t.Errorf("current should carry fresh value; got %v", res.Drift[0].Current)
	}
}

func TestRefetchAPIFieldRow_NilStoredEqualsAllDriftFromZero(t *testing.T) {
	// First-time refetch (no prior stored data). Every requested
	// column whose current value is non-nil counts as drift from
	// "stored=nil" — gives the picker UI a clean way to render
	// "everything is new" without special-casing.
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice", "email": "alice@a.com"},
	}

	res, err := m.RefetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"name", "email"}, nil)
	if err != nil {
		t.Fatalf("RefetchAPIFieldRow: %v", err)
	}
	if len(res.Drift) != 2 {
		t.Errorf("expected 2 drift entries (all from zero); got %v", res.Drift)
	}
}

func TestRefetchAPIFieldRow_PropagatesFetchErrors(t *testing.T) {
	m, _, _ := newAPIFieldWorld()
	_, err := m.RefetchAPIFieldRow(context.Background(),
		"ghost.yaml", "g-1", []string{"name"}, nil)
	if !errors.Is(err, ErrAPIFieldTemplateNotFound) {
		t.Errorf("err = %v; want ErrAPIFieldTemplateNotFound", err)
	}
}

func TestFetchAPIFieldRow_EmptyColumnsReturnsEmptyRow(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{"name": "Alice"},
	}

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", nil)
	if err != nil {
		t.Fatalf("FetchAPIFieldRow: %v", err)
	}
	if len(row) != 0 {
		t.Errorf("empty columns should produce empty row; got %v", row)
	}
}
