package index

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fakeTemplateLoader returns a canned template by filename.
type fakeTemplateLoader struct {
	tpls map[string]*TemplateRecord
}

func (f *fakeTemplateLoader) LoadTemplate(name string) (*TemplateRecord, error) {
	r, ok := f.tpls[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return r, nil
}

// fakeFormStore returns a canned form by (template, datafile).
type fakeFormStore struct {
	forms map[string]*FormRecord // key = template + "/" + datafile
}

func (f *fakeFormStore) LoadForm(tpl, file string) (*FormRecord, error) {
	r, ok := f.forms[tpl+"/"+file]
	if !ok {
		return nil, errors.New("not found")
	}
	return r, nil
}

func newEventHandler(t *testing.T,
	tpls map[string]*TemplateRecord,
	forms map[string]*FormRecord,
) (*EventHandler, *Manager) {
	t.Helper()
	m, err := NewManager(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })

	h := NewEventHandler(m,
		&fakeTemplateLoader{tpls: tpls},
		&fakeFormStore{forms: forms},
	)
	return h, m
}

func tplRecord(name string, fields []template.Field, mtime int64) *TemplateRecord {
	return &TemplateRecord{
		Template: &template.Template{
			Name:             name,
			Filename:         name + ".yaml",
			MarkdownTemplate: "# hi",
			Fields:           fields,
		},
		Mtime: mtime,
	}
}

func formRecord(meta storage.FormMeta, data map[string]any, mtime int64) *FormRecord {
	return &FormRecord{
		Form:  &storage.Form{Meta: meta, Data: data},
		Mtime: mtime,
	}
}

func TestEventHandler_OnTemplateChanged_Inserts(t *testing.T) {
	tpls := map[string]*TemplateRecord{
		"basic.yaml": tplRecord("Basic", []template.Field{
			{Key: "title", Type: "text"},
			{Key: "id", Type: "guid"},
			{Key: "labels", Type: "tags"},
		}, 100),
	}
	h, m := newEventHandler(t, tpls, nil)

	if err := h.OnTemplateChanged("basic.yaml"); err != nil {
		t.Fatal(err)
	}

	rows, err := m.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Filename != "basic.yaml" || r.Name != "Basic" {
		t.Errorf("identity wrong: %+v", r)
	}
	if r.GuidField != "id" || r.TagsField != "labels" {
		t.Errorf("derived fields wrong: guid=%q tags=%q", r.GuidField, r.TagsField)
	}
	if !r.HasMarkdownTemplate {
		t.Errorf("HasMarkdownTemplate should be true (template carries markdown_template)")
	}
	if r.Mtime != 100 {
		t.Errorf("mtime = %d, want 100", r.Mtime)
	}
}

func TestEventHandler_OnTemplateChanged_NoSpecialFields(t *testing.T) {
	// A template with only a text field shouldn't have a guid/tags
	// derived; those columns must come back empty.
	tpls := map[string]*TemplateRecord{
		"plain.yaml": tplRecord("Plain", []template.Field{
			{Key: "x", Type: "text"},
		}, 50),
	}
	h, m := newEventHandler(t, tpls, nil)

	if err := h.OnTemplateChanged("plain.yaml"); err != nil {
		t.Fatal(err)
	}
	rows, _ := m.ListTemplates()
	if rows[0].GuidField != "" || rows[0].TagsField != "" {
		t.Errorf("expected empty special fields, got %+v", rows[0])
	}
}

func TestEventHandler_OnTemplateDeleted_RemovesAndCascades(t *testing.T) {
	tpls := map[string]*TemplateRecord{
		"basic.yaml": tplRecord("Basic", []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "labels", Type: "tags"},
		}, 100),
	}
	forms := map[string]*FormRecord{
		"basic.yaml/one.meta.json": formRecord(
			storage.FormMeta{ID: "g1", AuthorName: "Alice", Updated: "2026-05-01T00:00:00Z"},
			map[string]any{"id": "g1", "labels": []any{"a"}},
			10,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("basic.yaml"))
	must(t, h.OnFormChanged("basic.yaml", "one.meta.json"))

	must(t, h.OnTemplateDeleted("basic.yaml"))

	tplsRows, _ := m.ListTemplates()
	if len(tplsRows) != 0 {
		t.Errorf("templates not removed: %+v", tplsRows)
	}
	formRows, _ := m.ListForms("basic.yaml", QueryOpts{})
	if len(formRows) != 0 {
		t.Errorf("forms not cascaded: %+v", formRows)
	}
}

func TestEventHandler_OnFormChanged_Inserts_WithGuidAndTags(t *testing.T) {
	tpls := map[string]*TemplateRecord{
		"basic.yaml": tplRecord("Basic", []template.Field{
			{Key: "title", Type: "text"},
			{Key: "id", Type: "guid"},
			{Key: "labels", Type: "tags"},
		}, 100),
	}
	forms := map[string]*FormRecord{
		"basic.yaml/first.meta.json": formRecord(
			storage.FormMeta{
				ID: "meta-id-not-used", // form.Meta.ID is NOT what we want as id
				AuthorName: "Alice",
				Created:    "2026-01-01T00:00:00Z",
				Updated:    "2026-05-01T00:00:00Z",
			},
			map[string]any{
				"title":  "First",
				"id":     "g1",                // from data[guid_field]
				"labels": []any{"alpha", "common"},
			},
			500,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("basic.yaml"))
	must(t, h.OnFormChanged("basic.yaml", "first.meta.json"))

	row, ok, err := m.GetForm("basic.yaml", "first.meta.json")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if row.ID != "g1" {
		t.Errorf("id = %q, want g1 (data[guid_field], not meta.ID)", row.ID)
	}
	// Template here has no item_field set, so title falls back to the
	// datafile name. A separate test below covers the item_field path.
	if row.Title != "first.meta.json" {
		t.Errorf("title = %q, want filename fallback", row.Title)
	}
	if row.Author != "Alice" || row.Updated != "2026-05-01T00:00:00Z" {
		t.Errorf("audit wrong: %+v", row)
	}
	if got := sortedCopy(row.Tags); !equalStrings(got, []string{"alpha", "common"}) {
		t.Errorf("tags = %v, want [alpha common]", got)
	}
	if row.Mtime != 500 {
		t.Errorf("mtime = %d, want 500", row.Mtime)
	}
}

func TestEventHandler_OnFormChanged_TitleFromItemField(t *testing.T) {
	tpls := map[string]*TemplateRecord{
		"basic.yaml": &TemplateRecord{
			Template: &template.Template{
				Name:      "Basic",
				Filename:  "basic.yaml",
				ItemField: "title",
				Fields: []template.Field{
					{Key: "title", Type: "text"},
					{Key: "id", Type: "guid"},
				},
			},
			Mtime: 100,
		},
	}
	forms := map[string]*FormRecord{
		"basic.yaml/x.meta.json": formRecord(
			storage.FormMeta{},
			map[string]any{"title": "From Item Field", "id": "g"},
			10,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("basic.yaml"))
	must(t, h.OnFormChanged("basic.yaml", "x.meta.json"))

	row, _, _ := m.GetForm("basic.yaml", "x.meta.json")
	if row.Title != "From Item Field" {
		t.Errorf("title = %q, want From Item Field", row.Title)
	}
}

func TestEventHandler_OnFormChanged_TitleFallbackToFilename(t *testing.T) {
	// No item_field on the template AND no usable value → title is the
	// filename, matching the wiki's old "Available Forms" list behaviour.
	tpls := map[string]*TemplateRecord{
		"basic.yaml": tplRecord("Basic", []template.Field{
			{Key: "x", Type: "text"},
		}, 100),
	}
	forms := map[string]*FormRecord{
		"basic.yaml/note.meta.json": formRecord(
			storage.FormMeta{},
			map[string]any{"x": "hello"},
			10,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("basic.yaml"))
	must(t, h.OnFormChanged("basic.yaml", "note.meta.json"))

	row, _, _ := m.GetForm("basic.yaml", "note.meta.json")
	if row.Title != "note.meta.json" {
		t.Errorf("title = %q, want filename fallback", row.Title)
	}
}

func TestEventHandler_OnFormChanged_TagsOnlyFromTagsField(t *testing.T) {
	// Templates without a tags field MUST NOT pick up tags from any
	// other source. Also: form.Meta.Tags is *not* the index source —
	// the data-side tag field is.
	tpls := map[string]*TemplateRecord{
		"plain.yaml": tplRecord("Plain", []template.Field{
			{Key: "x", Type: "text"},
		}, 100),
	}
	forms := map[string]*FormRecord{
		"plain.yaml/n.meta.json": formRecord(
			storage.FormMeta{Tags: []string{"this", "is", "ignored"}},
			map[string]any{"x": "hi"},
			10,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("plain.yaml"))
	must(t, h.OnFormChanged("plain.yaml", "n.meta.json"))

	row, _, _ := m.GetForm("plain.yaml", "n.meta.json")
	if len(row.Tags) != 0 {
		t.Errorf("tags should be empty on tagless template, got %v", row.Tags)
	}
}

func TestEventHandler_OnFormDeleted(t *testing.T) {
	tpls := map[string]*TemplateRecord{
		"basic.yaml": tplRecord("Basic", []template.Field{
			{Key: "labels", Type: "tags"},
		}, 100),
	}
	forms := map[string]*FormRecord{
		"basic.yaml/one.meta.json": formRecord(
			storage.FormMeta{},
			map[string]any{"labels": []any{"x"}},
			10,
		),
	}
	h, m := newEventHandler(t, tpls, forms)

	must(t, h.OnTemplateChanged("basic.yaml"))
	must(t, h.OnFormChanged("basic.yaml", "one.meta.json"))

	must(t, h.OnFormDeleted("basic.yaml", "one.meta.json"))

	rows, _ := m.ListForms("basic.yaml", QueryOpts{})
	if len(rows) != 0 {
		t.Errorf("form not deleted: %+v", rows)
	}
}
