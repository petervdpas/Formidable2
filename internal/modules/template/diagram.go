package template

// A `diagram` field is virtual: it stores nothing and draws the record's own
// data. Its options are fixed value/label rows where value is the role and
// label the bound key: `source` names a root table field, the column roles name
// that table's column keys.
const (
	DiagramSource     = "source"
	DiagramFromEntity = "from_entity"
	DiagramFromAttr   = "from_attr"
	DiagramToEntity   = "to_entity"
	DiagramToAttr     = "to_attr"
	DiagramLabel      = "label"

	DiagramProjectionLineage = "lineage"
)

// diagramColumnRoles is the column-role order; the first two are required.
var diagramColumnRoles = []string{DiagramFromEntity, DiagramToEntity, DiagramFromAttr, DiagramToAttr, DiagramLabel}

var diagramRequiredRoles = map[string]bool{DiagramFromEntity: true, DiagramToEntity: true}

// DiagramBinding is a diagram field resolved against its template: Columns maps
// every column role to its index in the source table, -1 when unbound.
type DiagramBinding struct {
	Projection string
	Source     string
	Columns    map[string]int
}

// diagramRoles reads a diagram field's role -> bound key pairs from its options.
func diagramRoles(f Field) map[string]string {
	out := map[string]string{}
	for _, o := range f.Options {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["value"].(string)
		key, _ := m["label"].(string)
		if role != "" {
			out[role] = key
		}
	}
	return out
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

// DiagramBindingOf resolves diagram field fieldKey of t; ok is false when the
// field is not a diagram or its binding has any error Validate would report.
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
		roles := diagramRoles(f)
		src := roles[DiagramSource]
		var cols map[string]int
		for _, tf := range t.Fields {
			if tf.Key == src && tf.Type == "table" {
				cols = tableColumnIndex(tf)
				break
			}
		}
		b := DiagramBinding{Projection: DiagramProjectionLineage, Source: src, Columns: map[string]int{}}
		for _, role := range diagramColumnRoles {
			b.Columns[role] = -1
			if k := roles[role]; k != "" {
				b.Columns[role] = cols[k]
			}
		}
		return b, true
	}
	return DiagramBinding{}, false
}

// diagramFieldErrors flags diagram fields whose source is not a root table, or
// whose column roles are missing (from/to entity) or name no column of it.
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
	roles := diagramRoles(f)
	src := roles[DiagramSource]
	fail := func(typ, msg string, detail map[string]any) []ValidationError {
		return []ValidationError{{Type: typ, Field: &ff, Index: i, Key: f.Key, Detail: detail, Message: msg}}
	}
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
	if idx < 0 {
		return fail("diagram-field-unknown-source", "Diagram field source is not a field: "+src, map[string]any{"source": src})
	}
	if fields[idx].Type != "table" {
		return fail("diagram-field-source-not-table", "Diagram field source must be a table field: "+src, map[string]any{"source": src, "type": fields[idx].Type})
	}
	if idx < len(canonical) && canonical[idx].LevelScope > 0 {
		return fail("diagram-field-source-not-root", "Diagram field source must be a top-level table, not inside a loop: "+src, map[string]any{"source": src})
	}
	cols := tableColumnIndex(fields[idx])
	var errs []ValidationError
	for _, role := range diagramColumnRoles {
		k := roles[role]
		if k == "" {
			if diagramRequiredRoles[role] {
				errs = append(errs, fail("diagram-field-missing-column", "Diagram field needs a column for "+role, map[string]any{"role": role})...)
			}
			continue
		}
		if _, ok := cols[k]; !ok {
			errs = append(errs, fail("diagram-field-unknown-column", "Diagram field column is not in table "+src+": "+k, map[string]any{"role": role, "column": k, "source": src})...)
		}
	}
	return errs
}
