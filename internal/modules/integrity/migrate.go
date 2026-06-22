package integrity

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// MigrateResult summarizes a field-key rename run across a template's forms.
type MigrateResult struct {
	FormsTouched int `json:"forms_touched"`
	FormsSaved   int `json:"forms_saved"`
	KeysMoved    int `json:"keys_moved"`
}

// RenameCandidates lists the choices for a doctor "move data between keys" run:
// OrphanKeys are data keys the template no longer declares (move FROM), and
// FieldKeys are the template's declared field keys (move TO). Both are read off
// the RAW forms so a top-level orphan a sanitized load would drop still appears.
type RenameCandidates struct {
	OrphanKeys []string `json:"orphan_keys"`
	FieldKeys  []string `json:"field_keys"`
}

// RenameCandidates scans every form's raw data for keys the template no longer
// declares (structure-aware via analyzeForm, so loop nesting is handled and
// link/api value-objects are not mistaken for fields), and pairs them with the
// template's declared field keys.
func (m *Manager) RenameCandidates(templateFilename string) (RenameCandidates, error) {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return RenameCandidates{}, fmt.Errorf("integrity: load template %q: %w", templateFilename, err)
	}
	filenames, err := m.storage.ListForms(templateFilename)
	if err != nil {
		return RenameCandidates{}, fmt.Errorf("integrity: list forms for %q: %w", templateFilename, err)
	}

	rawReader, _ := m.storage.(RawFormReader)
	orphans := map[string]struct{}{}
	for _, fn := range filenames {
		var f *storage.Form
		if rawReader != nil {
			f = rawReader.LoadFormRaw(templateFilename, fn)
		}
		if f == nil {
			f = m.storage.LoadForm(templateFilename, fn)
		}
		if f == nil {
			continue
		}
		for _, iss := range analyzeForm(tpl, f) {
			if iss.Kind == IssueExtraField {
				if k := pathLeaf(iss.Path); k != "" {
					orphans[k] = struct{}{}
				}
			}
		}
	}

	out := make([]string, 0, len(orphans))
	for k := range orphans {
		out = append(out, k)
	}
	sort.Strings(out)
	return RenameCandidates{OrphanKeys: out, FieldKeys: declaredFieldKeys(tpl)}, nil
}

// declaredFieldKeys lists the template's data-bearing field keys (loop keys and
// loop-inner fields included), excluding the loopstop/looper markers, sorted.
func declaredFieldKeys(tpl *template.Template) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range tpl.Fields {
		if f.Type == "loopstop" || f.Type == "looper" || f.Key == "" || seen[f.Key] {
			continue
		}
		seen[f.Key] = true
		out = append(out, f.Key)
	}
	sort.Strings(out)
	return out
}

// MigrateFieldKey renames a data key from oldKey to newKey across every form
// under templateFilename, preserving the value at each occurrence (loop-nested
// included). The template should already declare newKey and no longer declare
// oldKey, so oldKey reads as an orphaned ("extra") data key.
//
// Detection reads the RAW form: a normal load sanitizes data against the
// current template and drops any top-level key it no longer declares, which
// would erase the orphan before it could be moved. The saved draft is based on
// the sanitized load so its meta block (timestamps, identity) is preserved; the
// moved value is grafted onto the new key. Only changed forms are saved, so a
// re-run is a no-op.
func (m *Manager) MigrateFieldKey(templateFilename, oldKey, newKey string) (MigrateResult, error) {
	if m.writer == nil {
		return MigrateResult{}, fmt.Errorf("integrity: MigrateFieldKey called on writer-less manager")
	}
	oldKey = strings.TrimSpace(oldKey)
	newKey = strings.TrimSpace(newKey)
	if oldKey == "" || newKey == "" {
		return MigrateResult{}, fmt.Errorf("integrity: MigrateFieldKey needs both old and new keys")
	}
	if oldKey == newKey {
		return MigrateResult{}, fmt.Errorf("integrity: MigrateFieldKey old and new key are identical (%q)", oldKey)
	}

	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("integrity: load template %q: %w", templateFilename, err)
	}

	filenames, err := m.storage.ListForms(templateFilename)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("integrity: list forms for %q: %w", templateFilename, err)
	}

	rawReader, _ := m.storage.(RawFormReader)

	result := MigrateResult{}
	for _, fn := range filenames {
		// Read the raw form first: it carries the orphaned key. A sanitized
		// load (below) may rewrite the file on disk, dropping the orphan.
		var raw *storage.Form
		if rawReader != nil {
			raw = rawReader.LoadFormRaw(templateFilename, fn)
		}

		base := m.storage.LoadForm(templateFilename, fn)
		if base == nil {
			continue // unreadable form: nothing to migrate, no meta to preserve
		}
		if raw == nil {
			raw = base // no raw reader: loop orphans still surface, top-level may not
		}
		draft := cloneForm(base)

		moved := 0
		// analyzeForm flags each orphaned oldKey as an extra_field with a full
		// path; moving by that path handles top-level fields, loop keys, and
		// per-item loop fields uniformly. The issue list is a snapshot, but each
		// occurrence sits in a distinct parent map, so in-place moves don't
		// interfere with later iterations.
		for _, iss := range analyzeForm(tpl, raw) {
			if iss.Kind != IssueExtraField || pathLeaf(iss.Path) != oldKey {
				continue
			}
			parent, key, err := walkPath(raw.Data, iss.Path, false)
			if err != nil || parent == nil {
				continue
			}
			val, present := parent[key]
			if !present {
				continue
			}
			if applied, _, err := setAtPath(draft.Data, replaceLeaf(iss.Path, newKey), val); err != nil {
				return MigrateResult{}, err
			} else if !applied {
				continue
			}
			// Drop any leftover old key from the draft. Top-level keys are
			// already gone (sanitized away in base); loop items keep theirs.
			_, _, _ = mutateAtPath(draft.Data, iss.Path, func(p map[string]any, k string) error {
				delete(p, k)
				return nil
			})
			moved++
		}

		if moved == 0 {
			continue
		}
		result.FormsTouched++
		result.KeysMoved += moved
		if err := m.writer.SaveForm(context.Background(), templateFilename, fn, draft); err != nil {
			return MigrateResult{}, fmt.Errorf("integrity: save %s: %w", fn, err)
		}
		result.FormsSaved++
	}
	return result, nil
}

// pathLeaf returns the final dotted segment of a data path ("a.b[2].c" -> "c").
// Extra-field paths always end in a bare field key, so no trailing index appears.
func pathLeaf(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// replaceLeaf swaps a path's final dotted segment for newLeaf
// ("x[2].old" -> "x[2].new", "old" -> "new").
func replaceLeaf(path, newLeaf string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[:i+1] + newLeaf
	}
	return newLeaf
}
