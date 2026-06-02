package app

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// scaleValues computes a form's scaling factors: for each template scaling, the
// per-record factor (its source option's weight, or the default). Keyed by
// scaling name, it is the S["name"] namespace the expression engine sees. This
// is the single calculator both the formula/datacore path and the sidebar
// harvest draw from (the role formulaValues plays for formulas).
func scaleValues(tpl *template.Template, f *storage.Form) map[string]any {
	if tpl == nil || f == nil || len(tpl.Scalings) == 0 {
		return nil
	}
	out := make(map[string]any, len(tpl.Scalings))
	for _, sc := range tpl.Scalings {
		out[sc.Name] = scaleFactor(sc, f)
	}
	return out
}

// scaleFactor resolves one scaling for one form: read the per-form source value
// (a facet's selected label or a dropdown/radio field's value), match it to a
// weight, fall back to the default when unlisted or unset.
func scaleFactor(sc template.Scaling, f *storage.Form) float64 {
	val := scaleSourceValue(sc.Source, f)
	for _, w := range sc.Weights {
		if w.Label == val {
			return w.Factor
		}
	}
	return sc.Default
}

// scaleSourceValue reads the per-form categorical value a scaling weights: a
// facet's selected option label from meta.facets, or a scalar field's value
// from f.Data. Unset/missing yields "" (so the default factor applies).
func scaleSourceValue(src template.StatSource, f *storage.Form) string {
	if src.Kind == "facet" {
		if st, ok := f.Meta.Facets[src.Key]; ok && st.Set {
			return st.Selected
		}
		return ""
	}
	if v, ok := f.Data[src.Key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return dcText(v)
	}
	return ""
}

// ScaleValues lets formulaHarvester satisfy the index.ScaleEvaluator and
// storage.ScaleFiller interfaces, so both harvest sites fold the S map into the
// expression context the sidebar reads. Needs no expression engine.
func (h formulaHarvester) ScaleValues(t *template.Template, f *storage.Form) map[string]any {
	return scaleValues(t, f)
}
