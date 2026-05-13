package csv

// Table-column coerce path used by the paste-data dialog on table fields.
// FormFieldTable.vue's column types ("string"/"number"/"date"/"bool"/
// "dropdown") differ from Coerce's wire vocabulary ("boolean"/"number"
// /"date"/"dropdown"); this layer maps between the two and reuses
// matchOption for dropdown columns so pasted labels resolve to values.

// TableColumn is the per-column spec the frontend hands over for a
// paste-coerce call. Choices carry the dropdown's option list as
// {value,label} maps — already pre-parsed from the template's
// pipe-separated `choices` string on the Vue side.
type TableColumn struct {
	Type    string `json:"type"`
	Choices []any  `json:"choices"`
}

func CoerceTableRows(cols []TableColumn, rows [][]string) [][]any {
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		outRow := make([]any, len(cols))
		for ci, col := range cols {
			if ci < len(row) {
				outRow[ci] = coerceTableCell(row[ci], col)
			} else {
				outRow[ci] = emptyTableCell(col.Type)
			}
		}
		out = append(out, outRow)
	}
	return out
}

func coerceTableCell(raw string, col TableColumn) any {
	switch col.Type {
	case "bool":
		return Coerce(raw, "boolean", nil)
	case "number":
		return Coerce(raw, "number", nil)
	case "date":
		return Coerce(raw, "date", nil)
	case "dropdown":
		return Coerce(raw, "dropdown", col.Choices)
	default:
		return raw
	}
}

func emptyTableCell(t string) any {
	switch t {
	case "bool":
		return false
	case "number":
		return float64(0)
	default:
		return ""
	}
}
