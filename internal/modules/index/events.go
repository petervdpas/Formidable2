package index

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TemplateRecord is one template loaded fresh from disk together with
// its file mtime. Carrying mtime alongside the parsed YAML lets the
// event handler write a row that matches what the next stale-detect
// would expect, avoiding a follow-up "changed" rev bump on next boot.
type TemplateRecord struct {
	Template *template.Template
	Mtime    int64
}

// FormRecord mirrors TemplateRecord for forms.
type FormRecord struct {
	Form  *storage.Form
	Mtime int64
}

// TemplateLoader is the narrow interface the event handler needs from
// the template module. The composition root supplies an adapter
// around *template.Manager that also stat()s the file for mtime.
type TemplateLoader interface {
	LoadTemplate(filename string) (*TemplateRecord, error)
}

// FormStore mirrors TemplateLoader for forms - adapter around
// *storage.Manager.
type FormStore interface {
	LoadForm(templateFilename, datafile string) (*FormRecord, error)
}

// EventHandler bridges template/storage manager events into single-
// row Reconcile calls on the index. It deliberately does not own the
// SQLite DB itself; that stays on Manager so reads and writes share
// one handle.
//
// `root` is the context folder (the dir that holds templates/ and
// storage/). It's only needed by RescanAll - per-event hooks don't
// touch disk directly. Composition root sets it via SetRoot.
type EventHandler struct {
	m         *Manager
	templates TemplateLoader
	forms     FormStore
	root      string
}

// NewEventHandler wires the writer side of the index. The composition
// root creates one per profile alongside the Manager.
func NewEventHandler(m *Manager, t TemplateLoader, f FormStore) *EventHandler {
	return &EventHandler{m: m, templates: t, forms: f}
}

// SetRoot configures the context folder used by RescanAll. Per-event
// hooks (OnTemplateChanged, OnFormChanged, etc.) don't need it.
func (h *EventHandler) SetRoot(path string) { h.root = path }

// OnTemplateChanged is called after a template YAML save. Loads the
// template fresh, derives the templates row, and - critically -
// re-derives every form row owned by this template.
//
// Why the form re-derive: form columns title / expression_items /
// tags / facets are projections of the template (item_field,
// expression_item flags, tags_field). When the template changes,
// those projections must update too, otherwise ExtendedListForms
// keeps shipping stale derivations until each form is individually
// re-saved. Cost is bounded by the number of forms in this one
// template; on save-heavy templates the user pays once per edit.
func (h *EventHandler) OnTemplateChanged(filename string) error {
	rec, err := h.templates.LoadTemplate(filename)
	if err != nil {
		return fmt.Errorf("index: load template %q: %w", filename, err)
	}
	if rec == nil || rec.Template == nil {
		return fmt.Errorf("index: template %q loader returned nil", filename)
	}

	formFilenames, err := h.listIndexedFormFiles(filename)
	if err != nil {
		return fmt.Errorf("index: list form files for %q: %w", filename, err)
	}
	formRows := make([]FormRow, 0, len(formFilenames))
	for _, datafile := range formFilenames {
		formRec, err := h.forms.LoadForm(filename, datafile)
		if err != nil || formRec == nil || formRec.Form == nil {
			// Form file is gone / unparseable - skip silently so one
			// bad file doesn't block the rest of the re-derive.
			// RescanAll will clean up orphans on next boot.
			continue
		}
		formRows = append(formRows,
			buildFormRow(rec.Template, formRec.Form, filename, datafile, formRec.Mtime))
	}

	return Reconcile(h.m.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{buildTemplateRow(rec.Template, rec.Mtime, filename)},
		UpsertForms:     formRows,
	})
}

// listIndexedFormFiles returns the form basenames currently indexed
// under the given template. Sourced from the index (one SQL query)
// rather than disk so we don't widen the EventHandler's surface area
// to a "list every form on disk" capability - disk-only files arrive
// via OnFormChanged and are caught up by RescanAll otherwise.
func (h *EventHandler) listIndexedFormFiles(templateFilename string) ([]string, error) {
	rows, err := h.m.DB().Query(
		`SELECT filename FROM forms WHERE template = ?`,
		templateFilename,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// OnTemplateDeleted is called after a template YAML delete. The DB's
// foreign-key cascades wipe the template's forms, form_tags, and
// images automatically.
func (h *EventHandler) OnTemplateDeleted(filename string) error {
	return Reconcile(h.m.DB(), ReconcileBatch{
		DeleteTemplates: []string{filename},
	})
}

// OnFormChanged is called after a form save. Loads the form AND its
// owning template (we need the template's guid_field/tags_field/
// item_field to extract the right values from form.Data) and writes
// the index row.
func (h *EventHandler) OnFormChanged(templateFilename, datafile string) error {
	tplRec, err := h.templates.LoadTemplate(templateFilename)
	if err != nil {
		return fmt.Errorf("index: load template %q: %w", templateFilename, err)
	}
	formRec, err := h.forms.LoadForm(templateFilename, datafile)
	if err != nil {
		return fmt.Errorf("index: load form %q/%q: %w", templateFilename, datafile, err)
	}
	if tplRec == nil || tplRec.Template == nil || formRec == nil || formRec.Form == nil {
		return fmt.Errorf("index: nil load result for %q/%q", templateFilename, datafile)
	}
	row := buildFormRow(tplRec.Template, formRec.Form, templateFilename, datafile, formRec.Mtime)
	return Reconcile(h.m.DB(), ReconcileBatch{UpsertForms: []FormRow{row}})
}

// OnFormDeleted is called after a form delete. FK cascade removes the
// form_tags rows automatically.
func (h *EventHandler) OnFormDeleted(templateFilename, datafile string) error {
	return Reconcile(h.m.DB(), ReconcileBatch{
		DeleteForms: []FormRef{{Template: templateFilename, Filename: datafile}},
	})
}

// ── row builders ─────────────────────────────────────────────────────

// buildTemplateRow projects a parsed *template.Template into a
// TemplateRow. Walks the field list once to derive the (single)
// guid_field and tags_field - validators upstream guarantee at most
// one of each per template.
func buildTemplateRow(t *template.Template, mtime int64, filename string) TemplateRow {
	row := TemplateRow{
		Filename:            filename,
		Name:                t.Name,
		ItemField:           t.ItemField,
		HasMarkdownTemplate: t.MarkdownTemplate != "",
		EnableCollection:    t.EnableCollection,
		Mtime:               mtime,
	}
	for _, f := range t.Fields {
		switch f.Type {
		case "guid":
			if row.GuidField == "" {
				row.GuidField = f.Key
			}
		case "tags":
			if row.TagsField == "" {
				row.TagsField = f.Key
			}
		}
	}
	return row
}

// buildFormRow projects a (template, form) pair into a FormRow. The
// template tells us where to look for id (guid_field), title
// (item_field, with filename fallback), and tags (tags_field - and
// only that field; storage.FormMeta.Tags is intentionally ignored to
// keep the index aligned with the schema's intent).
//
// templateFilename is taken from the caller (OnFormChanged) rather
// than t.Filename so the row's foreign key always matches whatever
// key the templates row was indexed under, even if the template's
// own `filename` field is stale or differs in case.
func buildFormRow(t *template.Template, f *storage.Form, templateFilename, datafile string, mtime int64) FormRow {
	row := FormRow{
		Template:     templateFilename,
		Filename:     datafile,
		CreatedName:  f.Meta.Created.Name,
		CreatedEmail: f.Meta.Created.Email,
		UpdatedName:  f.Meta.Updated.Name,
		UpdatedEmail: f.Meta.Updated.Email,
		Created:      f.Meta.Created.At,
		Updated:      f.Meta.Updated.At,
		Facets:       pickFacets(f.Meta.Facets),
		Mtime:        mtime,
	}

	guidKey, tagsKey := "", ""
	expressionFields := []string{}
	for _, fld := range t.Fields {
		switch fld.Type {
		case "guid":
			if guidKey == "" {
				guidKey = fld.Key
			}
		case "tags":
			if tagsKey == "" {
				tagsKey = fld.Key
			}
		}
		if fld.ExpressionItem {
			expressionFields = append(expressionFields, fld.Key)
		}
	}

	if guidKey != "" {
		if v, ok := f.Data[guidKey].(string); ok {
			row.ID = v
		}
	}
	row.Title = pickTitle(t.ItemField, f.Data, datafile)
	row.Tags = pickTags(tagsKey, f.Data)
	row.Values = pickValues(t.Fields, f.Data)
	row.ExpressionItems = encodeExpressionItems(expressionFields, f.Data)

	return row
}

// pickTitle implements the same "item_field value, else filename"
// fallback the original Formidable wiki uses for its form list.
func pickTitle(itemField string, data map[string]any, datafile string) string {
	if itemField != "" {
		if s, ok := data[itemField].(string); ok && s != "" {
			return s
		}
	}
	return datafile
}

// pickFacets projects FormMeta.Facets (key → FacetState) into the
// reconcile-side slice the index stores in form_facets. Stable
// iteration order so the reconciler's "delete then re-insert" pattern
// produces deterministic SQL writes for golden tests.
func pickFacets(in map[string]storage.FacetState) []FormFacet {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]FormFacet, 0, len(in))
	for _, k := range keys {
		s := in[k]
		out = append(out, FormFacet{Key: k, Set: s.Set, Selected: s.Selected})
	}
	return out
}

// pickTags returns the tag slice from data[tagsKey] when tagsKey is
// non-empty AND the value is a slice (with elements coerced to string).
// Empty when the template has no tag field - matches the contract
// that ListByTags only surfaces forms whose templates declare a tag
// field.
func pickTags(tagsKey string, data map[string]any) []string {
	if tagsKey == "" {
		return nil
	}
	raw, ok := data[tagsKey]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return cleanTags(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, t := range v {
			if s, ok := t.(string); ok {
				out = append(out, s)
			}
		}
		return cleanTags(out)
	}
	return nil
}

// cleanTags drops empty entries; dedup/normalization is left to
// Reconcile (which dedupes inside upsertFormsWithTags) and to the
// writer side that produced the data.
func cleanTags(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// encodeExpressionItems serialises {fieldKey: value} for every field
// flagged expression_item: true. Matches the wiki's old
// `expressionItems` blob shape so the sidebar expression engine can
// keep reading it without a schema change.
func encodeExpressionItems(keys []string, data map[string]any) string {
	if len(keys) == 0 {
		return ""
	}
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := data[k]; ok && v != nil && v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}
