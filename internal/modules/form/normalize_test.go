package form

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// Normalize — save-side coercions.
//
// Mirrors the original parsers' quirky cases that aren't pure DOM reads:
//
//   parseLatexField — value is ALWAYS field.default; user input ignored.
//   parseApiField   — fetches remote at save time. Stubbed in v1.
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_LatexValueCoercedToFieldDefault(t *testing.T) {
	fields := []template.Field{
		{Key: "math", Type: "latex", Default: "$x = y$"},
	}
	values := map[string]any{
		"math": "user-typed-something-here-that-must-be-overwritten",
	}
	Normalize(values, fields)
	if got := values["math"]; got != "$x = y$" {
		t.Errorf("latex value: want %q, got %v", "$x = y$", got)
	}
}

func TestNormalize_LatexEmptyDefaultGivesEmptyString(t *testing.T) {
	fields := []template.Field{
		{Key: "math", Type: "latex"},
	}
	values := map[string]any{"math": "anything"}
	Normalize(values, fields)
	if got := values["math"]; got != "" {
		t.Errorf("empty default: want empty string, got %q", got)
	}
}

func TestNormalize_LatexNormalizesCRLF(t *testing.T) {
	// Mirrors parseLatexField's `replace(/\r\n?/g, "\n")` behaviour.
	fields := []template.Field{
		{Key: "math", Type: "latex", Default: "line1\r\nline2\rline3"},
	}
	values := map[string]any{"math": ""}
	Normalize(values, fields)
	want := "line1\nline2\nline3"
	if got := values["math"]; got != want {
		t.Errorf("latex CRLF: want %q, got %q", want, got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Pass-through — non-LaTeX field types untouched.
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_NonLatexUntouched(t *testing.T) {
	fields := []template.Field{
		{Key: "name", Type: "text"},
		{Key: "active", Type: "boolean"},
		{Key: "qty", Type: "number"},
	}
	values := map[string]any{
		"name":   "Alice",
		"active": true,
		"qty":    42,
	}
	Normalize(values, fields)
	if values["name"] != "Alice" {
		t.Errorf("text field changed: %v", values["name"])
	}
	if values["active"] != true {
		t.Errorf("bool field changed: %v", values["active"])
	}
	if values["qty"] != 42 {
		t.Errorf("number field changed: %v", values["qty"])
	}
}

// ─────────────────────────────────────────────────────────────────────
// Unhappy paths
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_NilValuesIsSafe(t *testing.T) {
	fields := []template.Field{{Key: "math", Type: "latex"}}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil values panicked: %v", r)
		}
	}()
	Normalize(nil, fields)
}

func TestNormalize_NilFieldsIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil fields panicked: %v", r)
		}
	}()
	Normalize(map[string]any{"k": "v"}, nil)
}

func TestNormalize_EmptyInputIsSafe(t *testing.T) {
	Normalize(map[string]any{}, []template.Field{})
}

func TestNormalize_LatexFieldMissingFromValuesGetsDefault(t *testing.T) {
	// If the field exists in the template but not in the values map
	// (e.g. a freshly-added LaTeX field on a previously-saved form),
	// Normalize should still inject the default so the saved file
	// carries it.
	fields := []template.Field{
		{Key: "math", Type: "latex", Default: "$\\alpha$"},
	}
	values := map[string]any{}
	Normalize(values, fields)
	if got := values["math"]; got != "$\\alpha$" {
		t.Errorf("missing latex value: want default, got %v", got)
	}
}

func TestNormalize_NonStringDefaultStringifies(t *testing.T) {
	// Whatever was put in field.Default ought to round-trip as a string;
	// LaTeX is rendered text, never a number/bool.
	fields := []template.Field{
		{Key: "math", Type: "latex", Default: 42},
	}
	values := map[string]any{}
	Normalize(values, fields)
	if got, ok := values["math"].(string); !ok || got != "42" {
		t.Errorf("non-string default: want %q, got %v (%T)", "42", values["math"], values["math"])
	}
}
