package app

import (
	"strconv"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// formulaContext is the per-record context a formula sees: every non-virtual
// field's typed value (numbers stay float64 so arithmetic works) plus every set
// facet's selected value, keyed so F["key"] reaches them. Built from the live
// form data, not the stringified datacore cells, so `F["amount"] * 2` is real
// multiplication rather than a string error.
func formulaContext(tpl *template.Template, f *storage.Form) map[string]any {
	ctx := make(map[string]any)
	for _, fld := range tpl.Fields {
		// A facet field's value lives in meta.facets[facet_key]; expose it under
		// the field key so F["facet-field"] resolves like in the sidebar.
		if fld.Type == "facet" {
			if st, ok := f.Meta.Facets[fld.FacetKey]; ok && st.Set && st.Selected != "" {
				ctx[fld.Key] = st.Selected
			}
			continue
		}
		if datacoreSkipTypes[fld.Type] {
			continue
		}
		if v, ok := f.Data[fld.Key]; ok {
			ctx[fld.Key] = v
		}
	}
	// Also expose each set facet under its own key (F["facet-key"]).
	for k, st := range f.Meta.Facets {
		if st.Set && st.Selected != "" {
			ctx[k] = st.Selected
		}
	}
	// Scaling factors live in their own namespace: S["name"] (the per-record
	// weight), so a formula can write F["fcdm-dekking"] * S["fcdm-urgency"].
	if sv := scaleValues(tpl, f); len(sv) > 0 {
		ctx["S"] = sv
	}
	return ctx
}

// formulaSpecs adapts the template's formula catalog to the expression module's
// spec shape (it owns no dependency on template).
func formulaSpecs(formulas []template.Formula) []expression.FormulaSpec {
	specs := make([]expression.FormulaSpec, len(formulas))
	for i, f := range formulas {
		specs[i] = expression.FormulaSpec{Key: f.Key, Type: f.Type, Expression: f.Expression}
	}
	return specs
}

// formulaValues computes a form's formula fields (raw values), the single
// source both the datacore loader and the index harvest draw from.
func formulaValues(ev *expression.Manager, tpl *template.Template, f *storage.Form) map[string]any {
	if ev == nil || len(tpl.Formulas) == 0 {
		return nil
	}
	return ev.EvaluateFormulas(formulaSpecs(tpl.Formulas), formulaContext(tpl, f))
}

// formulaHarvester adapts formula evaluation to index.FormulaEvaluator, so the
// index harvest folds formula values into the expression context the sidebar
// reads, computed by the same engine the datacore loader uses.
type formulaHarvester struct{ ev *expression.Manager }

func (h formulaHarvester) FormulaValues(t *template.Template, f *storage.Form) map[string]any {
	return formulaValues(h.ev, t, f)
}

// applyFormulas computes the template's formulas for one form and writes each as
// an ordinary field cell on the datacore record (string-coerced; an empty or
// failed value is skipped, matching the loader's read-tolerance).
func applyFormulas(ev *expression.Manager, tpl *template.Template, f *storage.Form, rec *datacore.Record) {
	raw := formulaValues(ev, tpl, f)
	if len(raw) == 0 {
		return
	}
	for _, fm := range tpl.Formulas {
		v, ok := raw[fm.Key]
		if !ok {
			continue
		}
		s := coerceFormula(v, fm.Type)
		if s == "" {
			continue
		}
		if rec.Fields == nil {
			rec.Fields = map[string]string{}
		}
		rec.Fields[fm.Key] = s
	}
}

// coerceFormula renders a raw expression result as the string cell datacore
// stores, honouring the declared type. Numbers format without scientific
// notation so the numeric aggregate coerces them back cleanly.
func coerceFormula(raw any, typ string) string {
	switch typ {
	case "number":
		switch n := raw.(type) {
		case float64:
			return strconv.FormatFloat(n, 'f', -1, 64)
		case int:
			return strconv.Itoa(n)
		case int64:
			return strconv.FormatInt(n, 10)
		}
		return dcText(raw)
	case "bool":
		if b, ok := raw.(bool); ok {
			if b {
				return "true"
			}
			return "false"
		}
		return dcText(raw)
	default: // text, date, anything else
		return dcText(raw)
	}
}

// FormulaService previews an in-progress formula against the template's first
// stored form, so the editor can show the author a live value. It lives in the
// composition root because it spans template + storage + expression.
type FormulaService struct {
	tpl *template.Manager
	sto *storage.Manager
	ev  *expression.Manager
}

func NewFormulaService(tpl *template.Manager, sto *storage.Manager, ev *expression.Manager) *FormulaService {
	return &FormulaService{tpl: tpl, sto: sto, ev: ev}
}

// Preview evaluates exprSrc against the first form of templateFile and returns
// the value coerced to typ, or the evaluation error. Already-saved formulas are
// computed first so the new expression may reference them. No forms (or an
// empty result) returns "".
func (s *FormulaService) Preview(templateFile, exprSrc, typ string) (string, error) {
	tpl, err := s.tpl.LoadTemplate(templateFile)
	if err != nil {
		return "", err
	}
	files, err := s.sto.ListForms(templateFile)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	f := s.sto.LoadForm(templateFile, files[0])
	if f == nil {
		return "", nil
	}
	ctx := formulaContext(tpl, f)
	s.ev.EvaluateFormulas(formulaSpecs(tpl.Formulas), ctx) // seed saved formulas so the new one may reference them
	raw, err := s.ev.EvaluateValue(exprSrc, ctx)
	if err != nil {
		return "", err
	}
	return coerceFormula(raw, typ), nil
}
