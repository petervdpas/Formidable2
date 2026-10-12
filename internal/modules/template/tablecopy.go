package template

// Copying rows between two tables of one template maps cells by column key.

// TableColumnMap returns, per target column, the index of the source column
// with the same key, or -1 when the source has none.
func TableColumnMap(source, target Field) []int {
	src := tableColumnIndex(source)
	out := make([]int, len(target.Options))
	for i, o := range target.Options {
		out[i] = -1
		if m, ok := o.(map[string]any); ok {
			if k, _ := m["value"].(string); k != "" {
				if j, ok := src[k]; ok {
					out[i] = j
				}
			}
		}
	}
	return out
}

func sharesColumn(m []int) bool {
	for _, j := range m {
		if j >= 0 {
			return true
		}
	}
	return false
}

// TableCopySources lists the tables whose rows can be copied into root table
// targetKey: every other root table sharing at least one column key with it.
func TableCopySources(fields []Field, targetKey string) []ItemField {
	out := []ItemField{}
	canonical := assignLevelScopes(fields)
	var target *Field
	for i := range fields {
		if fields[i].Key == targetKey && fields[i].Type == "table" && canonical[i].LevelScope == 0 && targetKey != "" {
			target = &fields[i]
		}
	}
	if target == nil {
		return out
	}
	for i, f := range fields {
		if f.Type != "table" || f.Key == "" || f.Key == targetKey || canonical[i].LevelScope != 0 {
			continue
		}
		if sharesColumn(TableColumnMap(f, *target)) {
			out = append(out, ItemField{Key: f.Key, Label: labelOrKey(f)})
		}
	}
	return out
}
