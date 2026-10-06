package template

// A `diagram` field is virtual: it stores nothing and draws a table of the same
// record. It can only draw a table whose pattern has a diagram counterpart (e.g.
// data-lineage -> lineage); the pattern fixes which column plays which role, so
// the field's single option row only names the source table.
const (
	DiagramSource     = "source"
	DiagramFromEntity = "from_entity"
	DiagramFromAttr   = "from_attr"
	DiagramToEntity   = "to_entity"
	DiagramToAttr     = "to_attr"
	DiagramLabel      = "label"
	DiagramExample    = "example"

	DiagramProjectionLineage = "lineage"
)

var diagramColumnRoles = []string{DiagramFromEntity, DiagramFromAttr, DiagramToEntity, DiagramToAttr, DiagramLabel, DiagramExample}

// DiagramBinding is a diagram field resolved against its template: Columns maps
// every column role to its index in the source table, -1 when absent.
type DiagramBinding struct {
	Projection string
	Source     string
	Columns    map[string]int
}

// diagramSourceKey reads the bound table key from a diagram field's options.
func diagramSourceKey(f Field) string {
	for _, o := range f.Options {
		if m, ok := o.(map[string]any); ok {
			if role, _ := m["value"].(string); role == DiagramSource {
				key, _ := m["label"].(string)
				return key
			}
		}
	}
	return ""
}

func tableColumnIndex(f Field) map[string]int {
	out := map[string]int{}
	for i, o := range f.Options {
		if m, ok := o.(map[string]any); ok {
			if v, _ := m["value"].(string); v != "" {
				if _, dup := out[v]; !dup {
					out[v] = i
				}
			}
		}
	}
	return out
}

// DiagramTableKeys lists the root tables of fields a diagram can draw: those
// whose pattern has a diagram counterpart. Empty means no diagram is possible.
func DiagramTableKeys(fields []Field) []string {
	canonical := assignLevelScopes(fields)
	var out []string
	for i, f := range fields {
		if f.Type == "table" && f.Key != "" && canonical[i].LevelScope == 0 && tablePatternOf(f) != nil {
			out = append(out, f.Key)
		}
	}
	return out
}

func tablePatternOf(f Field) *tablePattern {
	p := findTablePattern(canonicalTablePattern(f.Format))
	if p == nil || p.diagram == "" {
		return nil
	}
	return p
}

// DiagramBindingOf resolves diagram field fieldKey of t; ok is false when the
// field is not a diagram or has any error Validate would report.
func DiagramBindingOf(t *Template, fieldKey string) (DiagramBinding, bool) {
	if t == nil || fieldKey == "" {
		return DiagramBinding{}, false
	}
	for i, f := range t.Fields {
		if f.Key != fieldKey || f.Type != "diagram" {
			continue
		}
		if len(diagramFieldErrorsAt(t.Fields, assignLevelScopes(t.Fields), i)) > 0 {
			return DiagramBinding{}, false
		}
		src := diagramSourceKey(f)
		for _, tf := range t.Fields {
			if tf.Key != src || tf.Type != "table" {
				continue
			}
			p := tablePatternOf(tf)
			cols := tableColumnIndex(tf)
			b := DiagramBinding{Projection: p.diagram, Source: src, Columns: map[string]int{}}
			for _, role := range diagramColumnRoles {
				b.Columns[role] = -1
				if idx, ok := cols[p.roles[role]]; ok {
					b.Columns[role] = idx
				}
			}
			return b, true
		}
	}
	return DiagramBinding{}, false
}

// diagramFieldErrors flags diagram fields on a template with no diagram-pattern
// table, or whose source is not such a root table.
func diagramFieldErrors(fields, canonical []Field) []ValidationError {
	var errs []ValidationError
	for i, f := range fields {
		if f.Type == "diagram" {
			errs = append(errs, diagramFieldErrorsAt(fields, canonical, i)...)
		}
	}
	return errs
}

func diagramFieldErrorsAt(fields, canonical []Field, i int) []ValidationError {
	f := fields[i]
	ff := f
	fail := func(typ, msg string, detail map[string]any) []ValidationError {
		return []ValidationError{{Type: typ, Field: &ff, Index: i, Key: f.Key, Detail: detail, Message: msg}}
	}
	if len(DiagramTableKeys(fields)) == 0 {
		return fail("diagram-field-no-pattern-table", "Diagram field needs a top-level table with a diagram pattern (e.g. data-lineage) on this template", nil)
	}
	src := diagramSourceKey(f)
	if src == "" {
		return fail("diagram-field-missing-source", "Diagram field is missing its source table", nil)
	}
	idx := -1
	for j, tf := range fields {
		if tf.Key == src && j != i {
			idx = j
			break
		}
	}
	switch {
	case idx < 0:
		return fail("diagram-field-unknown-source", "Diagram field source is not a field: "+src, map[string]any{"source": src})
	case fields[idx].Type != "table":
		return fail("diagram-field-source-not-table", "Diagram field source must be a table field: "+src, map[string]any{"source": src, "type": fields[idx].Type})
	case idx < len(canonical) && canonical[idx].LevelScope > 0:
		return fail("diagram-field-source-not-root", "Diagram field source must be a top-level table, not inside a loop: "+src, map[string]any{"source": src})
	case tablePatternOf(fields[idx]) == nil:
		return fail("diagram-field-source-no-pattern", "Diagram field source table has no diagram pattern: "+src, map[string]any{"source": src})
	}
	return nil
}
