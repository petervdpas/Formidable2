package index

import (
	"encoding/json"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type stubFormulaEval struct{ vals map[string]any }

func (s stubFormulaEval) FormulaValues(t *template.Template, f *storage.Form) map[string]any {
	return s.vals
}

func decodeItems(t *testing.T, blob string) map[string]any {
	t.Helper()
	if blob == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(blob), &m); err != nil {
		t.Fatalf("decode ExpressionItems %q: %v", blob, err)
	}
	return m
}

// TestBuildFormRow_FoldsFormulaValuesIntoExpressionItems: a wired evaluator's
// formula values join the harvested expression context next to the
// expression-flagged field values, so the sidebar can read F["formula"].
func TestBuildFormRow_FoldsFormulaValuesIntoExpressionItems(t *testing.T) {
	h := &EventHandler{formulas: stubFormulaEval{vals: map[string]any{"reference": "CH.02-5"}}}
	tpl := &template.Template{Fields: []template.Field{{Key: "code", Type: "text", ExpressionItem: true}}}
	f := &storage.Form{Data: map[string]any{"code": "CH.02"}}

	items := decodeItems(t, h.buildFormRow(tpl, f, "t.yaml", "a.meta.json", 1).ExpressionItems)
	if items["code"] != "CH.02" {
		t.Errorf("field value missing: %#v", items)
	}
	if items["reference"] != "CH.02-5" {
		t.Errorf("formula value not folded in: %#v", items)
	}
}

// TestBuildFormRow_NoEvaluatorOnlyFields: with no evaluator the harvest is
// unchanged (just the expression-flagged fields).
func TestBuildFormRow_NoEvaluatorOnlyFields(t *testing.T) {
	h := &EventHandler{}
	tpl := &template.Template{Fields: []template.Field{{Key: "code", Type: "text", ExpressionItem: true}}}
	f := &storage.Form{Data: map[string]any{"code": "CH.02"}}

	items := decodeItems(t, h.buildFormRow(tpl, f, "t.yaml", "a.meta.json", 1).ExpressionItems)
	if len(items) != 1 || items["code"] != "CH.02" {
		t.Errorf("expected only the field value, got %#v", items)
	}
}
