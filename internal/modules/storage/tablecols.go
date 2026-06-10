package storage

import (
	"context"
	"reflect"
)

// RemapTableColumns realigns every record's stored cells for one table field
// after its column definitions change. Table data is positional (one row is a
// cell array in column order), so adding, removing, or reordering a column in
// the template would otherwise leave existing values under the wrong header.
//
// perm has one entry per NEW column: perm[i] is the index of that column in the
// OLD order, or -1 for a column that did not exist before (its cell is blanked).
// A column dropped from the new order simply has no perm entry, so its old cell
// is discarded. Records whose rows are already aligned are left untouched (no
// re-stamp). Returns the number of records rewritten.
func (m *Manager) RemapTableColumns(templateFilename, fieldKey string, perm []int) (int, error) {
	if fieldKey == "" || len(perm) == 0 {
		return 0, nil
	}
	files, err := m.ListForms(templateFilename)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, df := range files {
		form := m.LoadForm(templateFilename, df)
		if form == nil {
			continue
		}
		if !remapTableInData(form.Data, fieldKey, perm) {
			continue
		}
		if r := m.SaveFormExact(context.Background(), templateFilename, df, *form); r.Success {
			changed++
		}
	}
	return changed, nil
}

// remapTableInData realigns the field's rows wherever they occur in the record:
// at the top level, or inside loop rows (and nested loops). Field keys are unique
// within a template, so locating the field by key is unambiguous. Loop data is an
// array of row maps under the loop key, so the walk recurses into every array
// whose elements are maps. Returns whether anything changed.
func remapTableInData(node map[string]any, fieldKey string, perm []int) bool {
	changed := false
	if rows, ok := node[fieldKey].([]any); ok && len(rows) > 0 {
		next := remapTableRows(rows, perm)
		if !reflect.DeepEqual(rows, next) {
			node[fieldKey] = next
			changed = true
		}
	}
	for k, v := range node {
		if k == fieldKey {
			continue // the field's own cell rows are not loop rows; don't descend
		}
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		for _, e := range arr {
			if row, ok := e.(map[string]any); ok {
				if remapTableInData(row, fieldKey, perm) {
					changed = true
				}
			}
		}
	}
	return changed
}

// remapTableRows rebuilds each cell row to the new column order. Non-array rows
// pass through unchanged; a perm entry of -1 (or one pointing past a short,
// ragged row) yields an empty cell.
func remapTableRows(rows []any, perm []int) []any {
	out := make([]any, len(rows))
	for ri, r := range rows {
		cells, ok := r.([]any)
		if !ok {
			out[ri] = r
			continue
		}
		nr := make([]any, len(perm))
		for i, src := range perm {
			if src >= 0 && src < len(cells) {
				nr[i] = cells[src]
			} else {
				nr[i] = ""
			}
		}
		out[ri] = nr
	}
	return out
}
