package storage

import (
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// TableCopyResult is the outcome of CopyTableRows: Value is the target table's
// new rows, Added how many were appended, and UnknownReferences the codes that
// landed in a reference column without matching an item of its target loop
// (kept in the cell, listed once each, so the author can fix them).
type TableCopyResult struct {
	Value             any      `json:"value"`
	Added             int      `json:"added"`
	UnknownReferences []string `json:"unknown_references"`
}

// CopyTableRows returns target table's rows plus source table's rows, both read
// from the saved record, with source cells mapped onto the target column of the
// same key. A cell landing in a reference column is rewritten to its target
// loop items' spelling ("q-01;C-03" -> "Q-01, C-03"); a code matching no item
// is kept and reported. A source row is added only when it maps to at least one
// non-empty cell and no identical row is already there, so repeating the copy
// adds nothing. Same read-only, pointer-driven contract as SortFieldValue.
func (m *Manager) CopyTableRows(templateFilename, datafile, targetKey, sourceKey string) (TableCopyResult, error) {
	var none TableCopyResult
	if targetKey == sourceKey {
		return none, fmt.Errorf("cannot copy table %q onto itself", targetKey)
	}
	target, ok := m.findField(templateFilename, targetKey)
	if !ok {
		return none, fmt.Errorf("unknown field %q", targetKey)
	}
	source, ok := m.findField(templateFilename, sourceKey)
	if !ok {
		return none, fmt.Errorf("unknown field %q", sourceKey)
	}
	for _, f := range []template.Field{target, source} {
		if f.Type != "table" {
			return none, fmt.Errorf("field %q (type %q) is not a table", f.Key, f.Type)
		}
	}
	colMap := template.TableColumnMap(source, target)
	shared := false
	for _, j := range colMap {
		shared = shared || j >= 0
	}
	if !shared {
		return none, fmt.Errorf("tables %q and %q have no column key in common", sourceKey, targetKey)
	}
	form := m.LoadForm(templateFilename, datafile)
	if form == nil {
		return none, fmt.Errorf("record %q not found (save it first)", datafile)
	}
	refs := m.referenceSpellings(templateFilename, target, form.Data)
	res := TableCopyResult{UnknownReferences: []string{}}
	unknownSeen := map[string]bool{}
	out := append([]any{}, asSlice(form.Data[targetKey])...)
	seen := map[string]bool{}
	for _, r := range out {
		seen[rowSignature(asSlice(r), len(colMap))] = true
	}
	for _, r := range asSlice(form.Data[sourceKey]) {
		cells, ok := r.([]any)
		if !ok {
			continue
		}
		row := make([]any, len(colMap))
		filled := false
		for i, j := range colMap {
			row[i] = ""
			if j >= 0 && j < len(cells) && cells[j] != nil {
				row[i] = cells[j]
				filled = filled || toStr(cells[j]) != ""
			}
		}
		sig := ""
		var unknown []string
		if filled {
			for i, known := range refs {
				row[i], unknown = normaliseReferenceCell(toStr(row[i]), known, unknown)
			}
			sig = rowSignature(row, len(colMap))
		}
		if !filled || seen[sig] {
			continue
		}
		seen[sig] = true
		out = append(out, row)
		res.Added++
		for _, code := range unknown {
			if k := strings.ToLower(code); !unknownSeen[k] {
				unknownSeen[k] = true
				res.UnknownReferences = append(res.UnknownReferences, code)
			}
		}
	}
	res.Value = out
	return res, nil
}

// referenceSpellings maps each reference column index of table (those with a
// valid target) to its loop items' codes, lower-cased -> as written.
func (m *Manager) referenceSpellings(templateFilename string, table template.Field, data map[string]any) map[int]map[string]string {
	out := map[int]map[string]string{}
	loops := template.ReferenceTargets(m.fieldsFor(templateFilename))
	for idx, target := range template.ReferenceColumns(table) {
		loop, ok := loops[target]
		if !ok {
			continue
		}
		known := map[string]string{}
		for _, it := range asSlice(data[loop]) {
			if item, ok := it.(map[string]any); ok {
				if v := strings.TrimSpace(toStr(item[target])); v != "" {
					if _, dup := known[strings.ToLower(v)]; !dup {
						known[strings.ToLower(v)] = v
					}
				}
			}
		}
		out[idx] = known
	}
	return out
}

// normaliseReferenceCell rewrites a cell's codes to the known spelling, joined
// ", "; codes matching nothing are kept as typed and appended to unknown.
func normaliseReferenceCell(cell string, known map[string]string, unknown []string) (string, []string) {
	codes := template.SplitReferenceCodes(cell)
	seen := map[string]bool{}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		k := strings.ToLower(code)
		if spelled, ok := known[k]; ok {
			code = spelled
		} else {
			unknown = append(unknown, code)
		}
		if !seen[strings.ToLower(code)] {
			seen[strings.ToLower(code)] = true
			out = append(out, code)
		}
	}
	return strings.Join(out, ", "), unknown
}

// TableCopySources lists the tables of the template whose rows can be copied
// into root table targetKey (other root tables sharing a column key with it).
func (m *Manager) TableCopySources(templateFilename, targetKey string) []template.ItemField {
	return template.TableCopySources(m.fieldsFor(templateFilename), targetKey)
}

// rowSignature identifies a row by the text of its first n cells.
func rowSignature(cells []any, n int) string {
	parts := make([]string, n)
	for i := range parts {
		if i < len(cells) {
			parts[i] = toStr(cells[i])
		}
	}
	return strings.Join(parts, "\x00")
}
