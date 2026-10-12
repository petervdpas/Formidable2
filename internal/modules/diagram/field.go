package diagram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// RenderField draws diagram field fieldKey of t from a record's data. ok is
// false when the field is not a validly bound diagram; empty tables are ok and
// yield "". A lineage table draws the mapping, an entity-relations table its
// relations; a bound second (relations) table is drawn into the same diagram.
func RenderField(t *template.Template, fieldKey string, data map[string]any) (string, bool) {
	b, ok := template.DiagramBindingOf(t, fieldKey)
	if !ok {
		return "", false
	}
	var g Graph
	switch b.Projection {
	case template.DiagramProjectionLineage:
		g = Lineage(tableRows(data[b.Source]), LineageColumns{
			FromEntity: b.Columns[template.DiagramFromEntity],
			FromAttr:   b.Columns[template.DiagramFromAttr],
			ToEntity:   b.Columns[template.DiagramToEntity],
			ToAttr:     b.Columns[template.DiagramToAttr],
			Label:      b.Columns[template.DiagramLabel],
			Example:    b.Columns[template.DiagramExample],
		})
		linkRules(&g, t, b, data)
	case template.DiagramProjectionRelations:
		g = Relations(tableRows(data[b.Source]), relationColumns(b.Columns))
	default:
		return "", false
	}
	if b.Relations != "" {
		AddRelations(&g, tableRows(data[b.Relations]), relationColumns(b.RelationColumns))
	}
	return SVG(Place(g)), true
}

func relationColumns(cols map[string]int) RelationColumns {
	return RelationColumns{
		Side:       cols[template.DiagramSide],
		FromEntity: cols[template.DiagramFromEntity],
		FromAttr:   cols[template.DiagramFromAttr],
		ToEntity:   cols[template.DiagramToEntity],
		ToAttr:     cols[template.DiagramToAttr],
		Label:      cols[template.DiagramLabel],
	}
}

// linkRules points each rule pill (shared, or part of a rule set) at its rule's in-page anchor when the rule
// column is a reference with a target; a code with no matching loop item is
// flagged undefined instead.
func linkRules(g *Graph, t *template.Template, b template.DiagramBinding, data map[string]any) {
	var table template.Field
	for _, f := range t.Fields {
		if f.Key == b.Source {
			table = f
		}
	}
	target := template.ReferenceColumns(table)[b.Columns[template.DiagramLabel]]
	loop, ok := template.ReferenceTargets(t.Fields)[target]
	if !ok {
		return
	}
	known := map[string]bool{}
	items, _ := data[loop].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			if v := strings.ToLower(strings.TrimSpace(cellText(m[target]))); v != "" {
				known[v] = true
			}
		}
	}
	resolve := func(code string) (link, class string) {
		if known[strings.ToLower(code)] {
			return "#" + template.ReferenceAnchor(target, code), ""
		}
		return "", NodeUndefined
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Kind == KindRule {
			n.Link, n.Class = resolve(n.Label)
		}
		for j := range n.Parts {
			n.Parts[j].Link, n.Parts[j].Class = resolve(n.Parts[j].Label)
		}
	}
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
