package index

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TemplateRecord is a fresh-loaded template plus its mtime, so the indexed row matches the next stale-detect.
type TemplateRecord struct {
	Template *template.Template
	Mtime    int64
}

// FormRecord mirrors TemplateRecord for forms.
type FormRecord struct {
	Form  *storage.Form
	Mtime int64
}

// TemplateLoader loads a template plus its file mtime.
type TemplateLoader interface {
	LoadTemplate(filename string) (*TemplateRecord, error)
}

// FormStore loads a form plus its file mtime.
type FormStore interface {
	LoadForm(templateFilename, datafile string) (*FormRecord, error)
}

// EventHandler bridges template/storage events into single-row Reconcile calls; the DB stays on Manager
// so reads and writes share one handle. root (the context folder) is only needed by RescanAll.
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

// SetRoot configures the context folder RescanAll uses.
func (h *EventHandler) SetRoot(path string) { h.root = path }

// OnTemplateChanged re-derives the templates row AND every form row, because form columns
// (title/expression_items/tags/facets) are projections of the template and would otherwise go stale.
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
			// Gone/unparseable: skip so one bad file doesn't block the re-derive (RescanAll cleans orphans).
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

// listIndexedFormFiles returns the form basenames currently indexed under a template (from the index, not disk).
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

// OnTemplateDeleted deletes the template; FK cascades wipe its forms, form_tags, and images.
func (h *EventHandler) OnTemplateDeleted(filename string) error {
	return Reconcile(h.m.DB(), ReconcileBatch{
		DeleteTemplates: []string{filename},
	})
}

// OnFormChanged loads the form and its template (needed to extract id/title/tags from form.Data) and writes the row.
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

// OnFormDeleted deletes the form; FK cascade removes its form_tags rows.
func (h *EventHandler) OnFormDeleted(templateFilename, datafile string) error {
	return Reconcile(h.m.DB(), ReconcileBatch{
		DeleteForms: []FormRef{{Template: templateFilename, Filename: datafile}},
	})
}

// buildTemplateRow projects a Template into a TemplateRow, deriving the single guid/tags field.
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

// buildFormRow projects a (template, form) pair into a FormRow; tags come only from tags_field (FormMeta.Tags
// is intentionally ignored). templateFilename comes from the caller, not t.Filename, so the FK always matches the indexed key.
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
	row.SearchBody = pickSearchBody(t.Fields, f.Data)
	row.ExpressionItems = encodeExpressionItems(expressionFields, f.Data)

	return row
}

// pickSearchBody flattens every prose field (text, choice labels, list/tag entries, table cells) into the
// newline-separated FTS5 body, in template order. Structured-only fields (guid/image/api) and the title are skipped.
func pickSearchBody(fields []template.Field, data map[string]any) string {
	var b strings.Builder
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	for _, fld := range fields {
		raw, ok := data[fld.Key]
		if !ok {
			continue
		}
		switch fld.Type {
		case "text", "textarea", "dropdown", "radio":
			add(asText(raw))
		case "multioption", "list", "tags":
			for _, item := range asSlice(raw) {
				add(asText(item))
			}
		case "table":
			for _, rowAny := range asSlice(raw) {
				for _, cell := range asSlice(rowAny) {
					add(asText(cell))
				}
			}
		}
	}
	return b.String()
}

// pickTitle returns the item_field value, falling back to the filename.
func pickTitle(itemField string, data map[string]any, datafile string) string {
	if itemField != "" {
		if s, ok := data[itemField].(string); ok && s != "" {
			return s
		}
	}
	return datafile
}

// pickFacets projects FormMeta.Facets into the form_facets slice, key-sorted for deterministic SQL writes.
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

// pickTags returns the tag slice from data[tagsKey]; empty when the template declares no tag field.
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

// cleanTags drops empty entries; dedup/normalization is left to Reconcile and the writer side.
func cleanTags(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// encodeExpressionItems serialises {fieldKey: value} for every expression_item field as a JSON blob.
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
