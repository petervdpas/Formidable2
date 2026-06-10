package dataprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/storage"
)

// ─────────────────────────────────────────────────────────────────────
// FetchAPIFieldRow - manager method.
//
// Source values stamp into the host form in their NATIVE JSON shape:
// scalars pass through, slices stay slices, maps stay maps. The host's
// .meta.json is JSON, so encoding inside the encoded form is just
// noise - every consumer would have to undo it.
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

func TestFetchAPIFieldRow_NonScalarPassesThroughNatively(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")
	sto.forms["people.yaml/alice.meta.json"] = &storage.Form{
		Data: map[string]any{
			"tags":  []any{"a", "b", "c"},
			"addr":  map[string]any{"city": "Amsterdam"},
			"table": []any{[]any{"r1c1", "r1c2"}, []any{"r2c1", "r2c2"}},
		},
	}

	row, err := m.FetchAPIFieldRow(context.Background(),
		"people.yaml", "g-1", []string{"tags", "addr", "table"})
	if err != nil {
		t.Fatalf("FetchAPIFieldRow: %v", err)
	}
	if got, ok := row["tags"].([]any); !ok || len(got) != 3 || got[0] != "a" {
		t.Errorf("tags: %#v, want native []any{\"a\",\"b\",\"c\"}", row["tags"])
	}
	if got, ok := row["addr"].(map[string]any); !ok || got["city"] != "Amsterdam" {
		t.Errorf("addr: %#v, want native map", row["addr"])
	}
	tbl, ok := row["table"].([]any)
	if !ok || len(tbl) != 2 {
		t.Fatalf("table: %#v, want native [][]any with 2 rows", row["table"])
	}
	if r0, ok := tbl[0].([]any); !ok || r0[0] != "r1c1" {
		t.Errorf("table[0]: %#v, want native row", tbl[0])
	}
}

// ─────────────────────────────────────────────────────────────────────
// APIFieldTitle - collapsed-card title from the FIRST mapped column, with
// the collection title and bare guid as fallbacks.
// ─────────────────────────────────────────────────────────────────────

func TestAPIFieldTitle_UsesFirstColumnValue(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	idx.forms["dm.yaml"][0].Title = "collection-title"
	sto.forms["dm.yaml/x.meta.json"] = &storage.Form{
		Data: map[string]any{"type": "inkomend", "title": "Some Title"},
	}

	// First mapped column is "type" -> its value wins over the collection title.
	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", []string{"type", "title"})
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "inkomend" {
		t.Errorf("title = %q, want %q", got, "inkomend")
	}
}

func TestAPIFieldTitle_EmptyFirstColumnFallsBackToCollectionTitle(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	idx.forms["dm.yaml"][0].Title = "collection-title"
	sto.forms["dm.yaml/x.meta.json"] = &storage.Form{
		Data: map[string]any{"type": "", "title": "Some Title"},
	}

	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", []string{"type"})
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "collection-title" {
		t.Errorf("title = %q, want collection-title fallback", got)
	}
}

func TestAPIFieldTitle_NoColumnsFallsBackToCollectionTitle(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	idx.forms["dm.yaml"][0].Title = "collection-title"

	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", nil)
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "collection-title" {
		t.Errorf("title = %q, want collection-title", got)
	}
}

func TestAPIFieldTitle_NestedShapeFallsBackToCollectionTitle(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	idx.forms["dm.yaml"][0].Title = "collection-title"
	sto.forms["dm.yaml/x.meta.json"] = &storage.Form{
		Data: map[string]any{"rows": []any{map[string]any{"a": 1}}},
	}

	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", []string{"rows"})
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "collection-title" {
		t.Errorf("title = %q, want collection-title for nested shape", got)
	}
}

func TestAPIFieldTitle_JoinsScalarList(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	sto.forms["dm.yaml/x.meta.json"] = &storage.Form{
		Data: map[string]any{"tags": []any{"a", "b", "c"}},
	}

	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", []string{"tags"})
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "a, b, c" {
		t.Errorf("title = %q, want \"a, b, c\"", got)
	}
}

func TestAPIFieldTitle_NoTitleAnywhereFallsBackToGuid(t *testing.T) {
	m, idx, sto := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")
	sto.forms["dm.yaml/x.meta.json"] = &storage.Form{Data: map[string]any{}}

	got, err := m.APIFieldTitle(context.Background(), "dm.yaml", "g-1", []string{"type"})
	if err != nil {
		t.Fatalf("APIFieldTitle: %v", err)
	}
	if got != "g-1" {
		t.Errorf("title = %q, want guid fallback g-1", got)
	}
}

func TestAPIFieldTitle_GuidNotFound(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	seedCollection(idx, "dm.yaml", "x.meta.json", "g-1")

	_, err := m.APIFieldTitle(context.Background(), "dm.yaml", "missing", []string{"type"})
	if !errors.Is(err, ErrAPIFieldGuidNotFound) {
		t.Errorf("err = %v, want ErrAPIFieldGuidNotFound", err)
	}
}

func TestResolveAPIFieldLink_BuildsFormidableHref(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")

	href, err := m.ResolveAPIFieldLink(context.Background(), "people.yaml", "g-1")
	if err != nil {
		t.Fatalf("ResolveAPIFieldLink: %v", err)
	}
	if href != "formidable://people.yaml:alice.meta.json" {
		t.Errorf("href: %q", href)
	}
}

func TestResolveAPIFieldLink_GuidNotFound(t *testing.T) {
	m, idx, _ := newAPIFieldWorld()
	seedCollection(idx, "people.yaml", "alice.meta.json", "g-1")

	_, err := m.ResolveAPIFieldLink(context.Background(), "people.yaml", "missing")
	if !errors.Is(err, ErrAPIFieldGuidNotFound) {
		t.Errorf("err: %v, want ErrAPIFieldGuidNotFound", err)
	}
}

func TestResolveAPIFieldLink_UnknownTemplate(t *testing.T) {
	m, _, _ := newAPIFieldWorld()
	_, err := m.ResolveAPIFieldLink(context.Background(), "ghost.yaml", "g-1")
	if !errors.Is(err, ErrAPIFieldTemplateNotFound) {
		t.Errorf("err: %v, want ErrAPIFieldTemplateNotFound", err)
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
