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
		if datacoreSkipTypes[fld.Type] {
			continue
		}
		if v, ok := f.Data[fld.Key]; ok {
			ctx[fld.Key] = v
		}
	}
	for k, st := range f.Meta.Facets {
		if st.Set && st.Selected != "" {
			ctx[k] = st.Selected
		}
	}
	return ctx
}

// evalFormulas evaluates formulas in declared order against ctx, returning the
// coerced string cells. It mutates ctx with each raw result so a later formula
// can reference an earlier one via F["..."]. A formula that fails to evaluate,
// or yields an empty value, is skipped (no cell), matching the loader's
// read-tolerance.
func evalFormulas(ev *expression.Manager, formulas []template.Formula, ctx map[string]any) map[string]string {
	out := make(map[string]string, len(formulas))
	for _, f := range formulas {
		raw, err := ev.EvaluateValue(f.Expression, ctx)
		if err != nil {
			continue
		}
		s := coerceFormula(raw, f.Type)
		if s == "" {
			continue
		}
		out[f.Key] = s
		ctx[f.Key] = raw
	}
	return out
}

// applyFormulas computes the template's formulas for one form and writes each
// as an ordinary field cell on the datacore record.
func applyFormulas(ev *expression.Manager, tpl *template.Template, f *storage.Form, rec *datacore.Record) {
	if ev == nil || len(tpl.Formulas) == 0 {
		return
	}
	for k, v := range evalFormulas(ev, tpl.Formulas, formulaContext(tpl, f)) {
		if rec.Fields == nil {
			rec.Fields = map[string]string{}
		}
		rec.Fields[k] = v
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
	evalFormulas(s.ev, tpl.Formulas, ctx) // seed saved formulas so the new one may reference them
	raw, err := s.ev.EvaluateValue(exprSrc, ctx)
	if err != nil {
		return "", err
	}
	return coerceFormula(raw, typ), nil
}
