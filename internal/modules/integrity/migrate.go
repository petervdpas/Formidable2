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

// RenameKey is one rename endpoint with its storage shape ("string" | "number"
// | "boolean" | "array" | "object"). A rename preserves the field type, so the
// frontend only pairs an orphan with a target of the same shape.
type RenameKey struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

// RenameCandidates lists the choices for a doctor "move data between keys" run,
// scoped to TOP-LEVEL keys. The classification is deterministic, from the
// sanitization invariant (a declared field is present in every record):
//
//   - Orphans (move FROM): a key the template does NOT declare that is present
//     in 100% of records. That can only be a renamed field - the template
//     changed but the records were not re-sanitized, so all still carry the old
//     key. A key below 100% presence is a removed field (sanitize already
//     dropped it from the re-saved records) and is left for Strip, never moved.
//   - Targets (move TO): a real data field (never a virtual facet/formula, never
//     a loop) that holds no data in ANY record - the untouched destination a
//     rename leaves behind. A field with data anywhere is live; moving onto it
//     would overwrite, so it is never a target.
//
// Both sides anchor on the template; data is only counted (presence, emptiness,
// shape). Read off RAW forms: a sanitized load fills defaults and drops
// top-level orphans, hiding both signals.
type RenameCandidates struct {
	Orphans []RenameKey `json:"orphans"`
	Targets []RenameKey `json:"targets"`
}

func (m *Manager) RenameCandidates(templateFilename string) (RenameCandidates, error) {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return RenameCandidates{}, fmt.Errorf("integrity: load template %q: %w", templateFilename, err)
	}
	filenames, err := m.storage.ListForms(templateFilename)
	if err != nil {
		return RenameCandidates{}, fmt.Errorf("integrity: list forms for %q: %w", templateFilename, err)
	}

	// Template = source of truth. Walk its top-level fields once: declaredKeys is
	// every top-level key it declares (so anything else in data is an orphan), and
	// targetType is the real data fields a rename could land on (type by key).
	declaredKeys := map[string]bool{}
	targetType := map[string]string{}
	for i := 0; i < len(tpl.Fields); i++ {
		fld := tpl.Fields[i]
		switch fld.Type {
		case "loopstart":
			declaredKeys[fld.Key] = true                // the loop array is a top-level key
			i = matchLoopstop(tpl.Fields, i+1, fld.Key) // jump past the loop body (inner fields aren't top-level)
		case "loopstop", "looper":
			// markers, not data
		default:
			if fld.Key == "" {
				continue
			}
			declaredKeys[fld.Key] = true
			if !template.IsVirtualFieldType(fld.Type) { // facet/formula hold no data of their own
				targetType[fld.Key] = fld.Type
			}
		}
	}

	rawReader, _ := m.storage.(RawFormReader)
	total := 0                         // records actually read
	orphanShape := map[string]string{} // orphan key -> shape ("" until a value reveals it)
	orphanPresent := map[string]int{}  // orphan key -> records the key appears in
	occupied := map[string]bool{}      // declared field that holds data in at least one record
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
		total++
		for k, v := range f.Data {
			if !declaredKeys[k] {
				orphanPresent[k]++
				if _, seen := orphanShape[k]; !seen {
					orphanShape[k] = ""
				}
				if orphanShape[k] == "" {
					if sh := shapeOfValue(v); sh != "" {
						orphanShape[k] = sh
					}
				}
				continue
			}
			if _, isTarget := targetType[k]; isTarget && isNonEmptyValue(v) {
				occupied[k] = true
			}
		}
	}

	// A Move source is an undeclared key present in EVERY record (100%): that is
	// the sanitization signature of a renamed field (template changed, records
	// not yet re-sanitized, so all still carry the old key). Below 100% is a
	// removed field (sanitize already dropped it from the re-saved records) and
	// gets stripped, never moved.
	orphans := make([]RenameKey, 0, len(orphanShape))
	for k, sh := range orphanShape {
		if total > 0 && orphanPresent[k] == total {
			orphans = append(orphans, RenameKey{Key: k, Kind: sh})
		}
	}
	targets := []RenameKey{}
	for k, typ := range targetType {
		if occupied[k] {
			continue // holds data somewhere: a live field, never a rename target
		}
		targets = append(targets, RenameKey{Key: k, Kind: shapeOfFieldType(typ)})
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Key < orphans[j].Key })
	sort.Slice(targets, func(i, j int) bool { return targets[i].Key < targets[j].Key })
	return RenameCandidates{Orphans: orphans, Targets: targets}, nil
}

// shapeOfValue maps a stored JSON value to its storage shape; "" for nil/unknown.
func shapeOfValue(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return ""
}

// shapeOfFieldType maps a declared field type to the same storage shapes, so a
// target field can be compared against an orphan value's shape.
func shapeOfFieldType(t string) string {
	switch t {
	case "number", "range", "sequence":
		return "number"
	case "boolean":
		return "boolean"
	case "list", "tags", "multioption", "table":
		return "array"
	case "link", "api":
		return "object"
	default: // text, textarea, date, dropdown, radio, guid, image, *-path, mermaid
		return "string"
	}
}

// isNonEmptyValue reports whether a stored value counts as data (so a field is a
// live field, not an empty rename target). Absent (nil), "", and empty
// array/map are empty; a present scalar (number, bool) counts as data.
func isNonEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
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
