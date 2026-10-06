package diagram

import (
	"fmt"
	"strconv"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// RenderField draws diagram field fieldKey of t from a record's data. ok is
// false when the field is not a validly bound diagram; an empty source table
// is ok and yields "".
func RenderField(t *template.Template, fieldKey string, data map[string]any) (string, bool) {
	b, ok := template.DiagramBindingOf(t, fieldKey)
	if !ok {
		return "", false
	}
	cols := LineageColumns{
		FromEntity: b.Columns[template.DiagramFromEntity],
		FromAttr:   b.Columns[template.DiagramFromAttr],
		ToEntity:   b.Columns[template.DiagramToEntity],
		ToAttr:     b.Columns[template.DiagramToAttr],
		Label:      b.Columns[template.DiagramLabel],
	}
	return SVG(Place(Lineage(tableRows(data[b.Source]), cols))), true
}

// tableRows reads a stored table value (rows of cells) as strings; anything
// that is not a row is skipped.
func tableRows(raw any) [][]string {
	switch v := raw.(type) {
	case [][]string:
		return v
	case []any:
		out := make([][]string, 0, len(v))
		for _, r := range v {
			cells, ok := r.([]any)
			if !ok {
				continue
			}
			row := make([]string, len(cells))
			for i, c := range cells {
				row[i] = cellText(c)
			}
			out = append(out, row)
		}
		return out
	}
	return nil
}

func cellText(c any) string {
	switch x := c.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(c)
}
