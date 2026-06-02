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
	formulas  FormulaEvaluator
	scales    ScaleEvaluator
	root      string
}

// FormulaEvaluator computes a form's formula fields (raw values keyed by formula
// key) so the harvest can fold them into the expression context the sidebar
// reads. The index owns no expression engine; the composition root supplies one.
type FormulaEvaluator interface {
	FormulaValues(t *template.Template, f *storage.Form) map[string]any
}

// ScaleEvaluator computes a form's scaling factors (keyed by scaling name) so
// the harvest can fold them into the expression context under the S namespace.
// Supplied by the composition root, like FormulaEvaluator.
type ScaleEvaluator interface {
	ScaleValues(t *template.Template, f *storage.Form) map[string]any
}

// NewEventHandler wires the writer side of the index. The composition
// root creates one per profile alongside the Manager.
func NewEventHandler(m *Manager, t TemplateLoader, f FormStore) *EventHandler {
	return &EventHandler{m: m, templates: t, forms: f}
}

// SetRoot configures the context folder RescanAll uses.
func (h *EventHandler) SetRoot(path string) { h.root = path }

// SetFormulaEvaluator wires the optional formula evaluator; nil leaves the
// harvest as just the expression-flagged field values.
func (h *EventHandler) SetFormulaEvaluator(fe FormulaEvaluator) { h.formulas = fe }

// SetScaleEvaluator wires the optional scaling evaluator; nil leaves the
// harvest without an S namespace.
func (h *EventHandler) SetScaleEvaluator(se ScaleEvaluator) { h.scales = se }

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
			h.buildFormRow(rec.Template, formRec.Form, filename, datafile, formRec.Mtime))
	}

	return h.m.Reconcile(ReconcileBatch{
		UpsertTemplates: []TemplateRow{buildTemplateRow(rec.Template, rec.Mtime, filename)},
		UpsertForms:     formRows,
	})
}

// listIndexedFormFiles returns the form basenames currently indexed under a template (from the index, not disk).
func (h *EventHandler) listIndexedFormFiles(templateFilename string) ([]string, error) {
	return h.m.FormFilenames(templateFilename)
}

// OnTemplateDeleted deletes the template; FK cascades wipe its forms, form_tags, and images.
func (h *EventHandler) OnTemplateDeleted(filename string) error {
	return h.m.Reconcile(ReconcileBatch{
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
	row := h.buildFormRow(tplRec.Template, formRec.Form, templateFilename, datafile, formRec.Mtime)
	return h.m.Reconcile(ReconcileBatch{UpsertForms: []FormRow{row}})
}

// OnFormDeleted deletes the form; FK cascade removes its form_tags rows.
func (h *EventHandler) OnFormDeleted(templateFilename, datafile string) error {
	return h.m.Reconcile(ReconcileBatch{
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
func (h *EventHandler) buildFormRow(t *template.Template, f *storage.Form, templateFilename, datafile string, mtime int64) FormRow {
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
	// Expression-item values keyed by field key. A facet field's value lives in
	// meta.facets[facet_key], not f.Data, so it's pulled from there; otherwise
	// the scalar from f.Data. Without this a sidebar rule keyed on a facet field
	// never resolves (F["facet-field"] stays nil) and always hits the default.
	exprVals := map[string]any{}
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
		if !fld.ExpressionItem {
			continue
		}
		if fld.Type == "facet" {
			if st, ok := f.Meta.Facets[fld.FacetKey]; ok && st.Set && st.Selected != "" {
				exprVals[fld.Key] = st.Selected
			}
		} else if v, ok := f.Data[fld.Key]; ok {
			exprVals[fld.Key] = v
		}
	}
	// Expose each set facet under its own key too (F["facet-key"]), matching the
	// formula context, so a sidebar expression can read the raw facet value
	// regardless of whether a facet field is flagged as an expression item.
	for k, st := range f.Meta.Facets {
		if st.Set && st.Selected != "" {
			exprVals[k] = st.Selected
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
	var formulaVals map[string]any
	if h.formulas != nil {
		formulaVals = h.formulas.FormulaValues(t, f)
	}
	var scaleVals map[string]any
	if h.scales != nil {
		scaleVals = h.scales.ScaleValues(t, f)
	}
	row.ExpressionItems = encodeExpressionItems(exprVals, formulaVals, scaleVals)

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
		case "text", "textarea", "mermaid", "dropdown", "radio":
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

// encodeExpressionItems serialises the expression-item field values plus the
// computed formula values as one JSON blob, with the scaling factors nested
// under "S". A sidebar expression reads F["formula"] alongside F["field"] and
// S["scaling"]. Empty values are dropped so an unset field/formula simply
// doesn't appear.
func encodeExpressionItems(exprVals, formulaVals, scaleVals map[string]any) string {
	out := make(map[string]any, len(exprVals)+len(formulaVals)+1)
	for k, v := range exprVals {
		if v != nil && v != "" {
			out[k] = v
		}
	}
	for k, v := range formulaVals {
		if v != nil && v != "" {
			out[k] = v
		}
	}
	if len(scaleVals) > 0 {
		out["S"] = scaleVals
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
