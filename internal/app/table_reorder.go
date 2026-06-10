package app

import (
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// tableColumnMigrator keeps record data aligned when a template's table field
// has its columns changed. Table cells are stored positionally, so reordering,
// inserting, or removing a column would otherwise leave every record's values
// under the wrong header. On a template update it realigns each affected field's
// stored rows by column key (implements template.UpdateObserver).
type tableColumnMigrator struct{ sto *storage.Manager }

func (mig tableColumnMigrator) OnTemplateUpdated(name string, old, updated *template.Template) error {
	if old == nil || updated == nil || mig.sto == nil {
		return nil
	}
	prev := tableColumnKeys(old)
	for _, f := range updated.Fields {
		if f.Type != "table" {
			continue
		}
		before, ok := prev[f.Key]
		if !ok {
			continue // not a table before (or new field): nothing to realign
		}
		perm, ok := tableColumnRemap(before, columnKeys(f.Options))
		if !ok {
			continue
		}
		if _, err := mig.sto.RemapTableColumns(name, f.Key, perm); err != nil {
			return err
		}
	}
	return nil
}

// tableColumnKeys maps each table field's key to its ordered column keys.
func tableColumnKeys(t *template.Template) map[string][]string {
	out := map[string][]string{}
	for _, f := range t.Fields {
		if f.Type == "table" {
			out[f.Key] = columnKeys(f.Options)
		}
	}
	return out
}

// columnKeys reads the ordered column keys (each option's "value") from a table
// field's options. Returns nil on an unexpected option shape, which callers
// treat as "not safely remappable".
func columnKeys(options []any) []string {
	keys := make([]string, 0, len(options))
	for _, o := range options {
		m, ok := o.(map[string]any)
		if !ok {
			return nil
		}
		v, _ := m["value"].(string)
		keys = append(keys, v)
	}
	return keys
}

// tableColumnRemap computes how to realign positional row cells from the old
// column order to the new one, matching columns by key. perm[i] is the index in
// `before` of the column now at position i, or -1 when that column is new.
//
// ok is false when realignment is unsafe or unnecessary: either column list has
// an empty or duplicate key (matching by key is ambiguous), or the order is
// unchanged (nothing to do). A renamed key reads as remove+add, so its data is
// dropped - the same ambiguity any positional store has.
func tableColumnRemap(before, after []string) ([]int, bool) {
	if !uniqueNonEmpty(before) || !uniqueNonEmpty(after) {
		return nil, false
	}
	if sameOrder(before, after) {
		return nil, false
	}
	idx := make(map[string]int, len(before))
	for i, k := range before {
		idx[k] = i
	}
	perm := make([]int, len(after))
	for i, k := range after {
		if j, ok := idx[k]; ok {
			perm[i] = j
		} else {
			perm[i] = -1
		}
	}
	return perm, true
}

func uniqueNonEmpty(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			return false
		}
		seen[k] = true
	}
	return true
}

func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
