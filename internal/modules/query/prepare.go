package query

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// FormData is one form's raw values for prepare: filename (record
// identity), data map (field key -> value), and set facet selections
// (facet key -> selected option label).
type FormData struct {
	Filename string
	Data     map[string]any
	Facets   map[string]string
}

// Loader supplies the template and forms that prepare flattens.
type Loader interface {
	Template(name string) (*template.Template, error)
	Forms(name string) ([]FormData, error)
}

type colPlan struct {
	src   Source
	hint  string
	facet bool
	fan   string // multi-valued field key this column fans from ("" = scalar/facet)
	col   int    // table column index (-1 when not a table column)
}

// referencedSources lists the distinct sources a spec touches (columns,
// filters, numeric measures). Those become the matrix's columns.
func referencedSources(spec Spec) []Source {
	seen := map[string]bool{}
	var out []Source
	add := func(s Source) {
		id := sourceID(s)
		if !seen[id] {
			seen[id] = true
			out = append(out, s)
		}
	}
	for _, c := range spec.Columns {
		add(c.Source)
	}
	for _, f := range spec.Filters {
		add(f.Source)
	}
	for _, ms := range spec.Measures {
		if needsSource(ms.Func) {
			add(ms.Source)
		}
	}
	return out
}

// Prepare reads the template's forms and flattens them into a Matrix.
// Referenced multi-valued fields (table columns, list/tags/multioption)
// are cartesian-exploded with provenance: columns of one table stay
// aligned while two different tables produce their product. Each exploded
// row carries a content hash per source table (stable identity) plus the
// positional index (keeps duplicates distinct). A form whose referenced
// table is empty contributes no rows (an empty product).
func Prepare(spec Spec, loader Loader) (*Matrix, error) {
	tpl, err := loader.Template(spec.Template)
	if err != nil {
		return nil, err
	}
	if tpl == nil {
		return nil, fmt.Errorf("query: template %q not found", spec.Template)
	}
	if tpl.Presentation {
		return nil, ErrPresentationExcluded
	}
	byKey := map[string]template.Field{}
	for _, f := range tpl.Fields {
		byKey[f.Key] = f
	}
	facetKeys := map[string]bool{}
	for _, fc := range tpl.Facets {
		facetKeys[fc.Key] = true
	}

	plans := make([]colPlan, 0)
	fanType := map[string]string{}
	for _, s := range referencedSources(spec) {
		p := colPlan{src: s, col: -1}
		switch {
		case s.Kind == "facet":
			if !facetKeys[s.Key] {
				return nil, fmt.Errorf("query: unknown facet %q", s.Key)
			}
			p.facet = true
		case s.Col != nil:
			f, ok := byKey[s.Key]
			if !ok {
				return nil, fmt.Errorf("query: unknown field %q", s.Key)
			}
			p.fan = s.Key
			p.col = *s.Col
			fanType[s.Key] = "table"
			p.hint = colTypeHint(f, *s.Col)
		default:
			f, ok := byKey[s.Key]
			if !ok {
				return nil, fmt.Errorf("query: unknown field %q", s.Key)
			}
			if isMultiField(f.Type) {
				p.fan = s.Key
				fanType[s.Key] = f.Type
			} else {
				p.hint = scalarHint(f.Type)
			}
		}
		plans = append(plans, p)
	}

	fanFields := make([]string, 0, len(fanType))
	for k := range fanType {
		fanFields = append(fanFields, k)
	}
	sort.Strings(fanFields)
	fanPos := map[string]int{}
	for i, k := range fanFields {
		fanPos[k] = i
	}

	cols := make([]MatrixCol, len(plans))
	for i, p := range plans {
		cols[i] = MatrixCol{ID: sourceID(p.src), Hint: p.hint}
	}

	forms, err := loader.Forms(spec.Template)
	if err != nil {
		return nil, err
	}
	m := &Matrix{Cols: cols, FormCount: len(forms)}

	for _, fd := range forms {
		entries := make([][]any, len(fanFields))
		hashes := make([][]string, len(fanFields))
		empty := false
		for fi, k := range fanFields {
			es := asSlice(fd.Data[k])
			entries[fi] = es
			hs := make([]string, len(es))
			for i, e := range es {
				if fanType[k] == "table" {
					hs[i] = hashCells(stringsOf(asSlice(e)))
				} else {
					hs[i] = hashCells([]string{asText(e)})
				}
			}
			hashes[fi] = hs
			if len(es) == 0 {
				empty = true
			}
		}
		if len(fanFields) > 0 && empty {
			continue // an empty referenced table -> empty cartesian for this form
		}

		idxs := make([]int, len(fanFields))
		for {
			row := MatrixRow{Form: fd.Filename, Cells: make([]string, len(plans))}
			for fi, k := range fanFields {
				row.Origins = append(row.Origins, Origin{
					Field: k, Row: idxs[fi], Hash: hashes[fi][idxs[fi]], Count: len(entries[fi]),
				})
			}
			for ci, p := range plans {
				row.Cells[ci] = planValue(p, fd, entries, fanPos, idxs)
			}
			m.Rows = append(m.Rows, row)

			if len(fanFields) == 0 || !advance(idxs, entries) {
				break
			}
		}
	}
	return m, nil
}

// advance increments the cartesian odometer (rightmost fastest),
// returning false once every combination has been produced.
func advance(idxs []int, entries [][]any) bool {
	for d := len(idxs) - 1; d >= 0; d-- {
		idxs[d]++
		if idxs[d] < len(entries[d]) {
			return true
		}
		idxs[d] = 0
	}
	return false
}

func planValue(p colPlan, fd FormData, entries [][]any, fanPos map[string]int, idxs []int) string {
	switch {
	case p.facet:
		return fd.Facets[p.src.Key]
	case p.fan != "":
		entry := entries[fanPos[p.fan]][idxs[fanPos[p.fan]]]
		if p.col >= 0 {
			cells := asSlice(entry)
			if p.col < len(cells) {
				return asText(cells[p.col])
			}
			return ""
		}
		return asText(entry)
	default:
		return asText(fd.Data[p.src.Key])
	}
}

func isMultiField(t string) bool {
	return t == "list" || t == "tags" || t == "multioption"
}

func scalarHint(t string) string {
	switch t {
	case "number", "range", "sequence":
		return "number"
	case "date":
		return "date"
	}
	return ""
}

func colTypeHint(f template.Field, col int) string {
	if col < 0 || col >= len(f.Options) {
		return ""
	}
	m, ok := f.Options[col].(map[string]any)
	if !ok {
		return ""
	}
	switch t, _ := m["type"].(string); t {
	case "number":
		return "number"
	case "date":
		return "date"
	}
	return ""
}

// hashCells is the stable content id of a source entry. Truncated to 128
// bits: collision-safe for row identity, compact in a large matrix.
func hashCells(cells []string) string {
	sum := sha256.Sum256([]byte(strings.Join(cells, "\x1f")))
	return hex.EncodeToString(sum[:16])
}

func stringsOf(vs []any) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = asText(v)
	}
	return out
}

// asText mirrors the index coercions so the matrix stringifies a value
// exactly as the rest of the app does.
func asText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
