package form

import (
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Normalize applies save-side coercions that aren't pure user input.
// Idempotent. nil-safe.
//
// Currently:
//   - LaTeX: value is forced to field.Default (the original parser
//     ignored DOM input and always returned field.default). \r\n? is
//     normalized to \n, matching parseLatexField in the JS code.
//
// Punted (TODO when API field comes back):
//   - api: live-fetch the remote doc, snapshot per field.Map.
func Normalize(values map[string]any, fields []template.Field) {
	if values == nil {
		return
	}
	for _, f := range fields {
		switch f.Type {
		case "latex":
			values[f.Key] = normalizeLatex(f.Default)
		}
	}
}

func normalizeLatex(def any) string {
	if def == nil {
		return ""
	}
	var s string
	switch v := def.(type) {
	case string:
		s = v
	default:
		s = fmt.Sprint(v)
	}
	// Normalize line endings — mirrors the JS parser.
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
	return s
}
