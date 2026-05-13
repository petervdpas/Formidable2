package integrity

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// StorageWriter is the post-Fix write surface. Production code passes
// the storage manager's "save the whole envelope, don't merge prev"
// adapter; tests pass a stub that mirrors the saved form back into the
// in-memory store so the post-fix re-analyze sees the new shape.
//
// The interface intentionally takes the full *storage.Form rather than
// just data, because meta-targeting strategies (MintUUID, Restamp)
// need to commit a mutated meta block alongside the data.
type StorageWriter interface {
	SaveForm(templateFilename, datafile string, form *storage.Form) error
}

// SetWriter installs the writer the Fix pipeline uses. Manager built
// without a writer can still Analyze; FixTemplate returns an error if
// called on a writer-less manager.
func (m *Manager) SetWriter(w StorageWriter) { m.writer = w }

// FixTemplate applies a FixPlan to the issues currently in
// templateFilename's storage. Each Plan item targets one issue kind
// with one strategy; issues whose kind isn't in the plan are left
// untouched. After mutating, the function writes only the forms whose
// data or meta actually changed, then re-runs Analyze to populate
// FixResult.ScannedAfter so the frontend can show "still N to go".
func (m *Manager) FixTemplate(templateFilename string, plan FixPlan) (FixResult, error) {
	if m.writer == nil {
		return FixResult{}, fmt.Errorf("integrity: FixTemplate called on writer-less manager")
	}

	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return FixResult{}, fmt.Errorf("integrity: load template %q: %w", templateFilename, err)
	}

	if err := validatePlan(plan); err != nil {
		return FixResult{}, err
	}

	strategyByKind := map[IssueKind]FixStrategy{}
	for _, it := range plan.Items {
		strategyByKind[it.Kind] = it.Strategy
	}

	filenames, err := m.storage.ListForms(templateFilename)
	if err != nil {
		return FixResult{}, fmt.Errorf("integrity: list forms for %q: %w", templateFilename, err)
	}

	result := FixResult{}

	for _, fn := range filenames {
		original := m.storage.LoadForm(templateFilename, fn)
		if original == nil {
			// Unreadable in the analyzer's terms — every plan strategy
			// is moot here, count it once and move on.
			if _, has := strategyByKind[IssueUnreadable]; has {
				result.Outcomes = append(result.Outcomes, FixOutcome{
					Filename: fn,
					Skipped:  1,
					Notes:    []string{"form unreadable — repair must happen outside the app"},
				})
				result.Skipped++
			}
			continue
		}

		// Clone the form so we can mutate freely without disturbing
		// the storage view between iterations.
		draft := cloneForm(original)
		issues := analyzeForm(tpl, draft)

		outcome := FixOutcome{Filename: fn}
		for _, iss := range issues {
			strat, want := strategyByKind[iss.Kind]
			if !want || strat == FixSkip {
				outcome.Skipped++
				continue
			}
			applied, note, err := applyStrategy(tpl, draft, iss, strat)
			if err != nil {
				return FixResult{}, err
			}
			if applied {
				outcome.Applied++
			} else {
				outcome.Skipped++
			}
			if note != "" {
				outcome.Notes = append(outcome.Notes, note)
			}
		}

		if outcome.Applied > 0 {
			if err := m.writer.SaveForm(templateFilename, fn, draft); err != nil {
				return FixResult{}, fmt.Errorf("integrity: save %s: %w", fn, err)
			}
			outcome.Saved = true
			result.FormsSaved++
		}

		if outcome.Applied > 0 || outcome.Skipped > 0 {
			result.FormsTouched++
			result.Outcomes = append(result.Outcomes, outcome)
			result.Applied += outcome.Applied
			result.Skipped += outcome.Skipped
		}
	}

	sort.Slice(result.Outcomes, func(i, j int) bool {
		return result.Outcomes[i].Filename < result.Outcomes[j].Filename
	})

	// Re-analyze so the caller can show "X still remain".
	after, err := m.AnalyzeTemplate(templateFilename)
	if err == nil {
		result.ScannedAfter = after.IssueCount
	}

	return result, nil
}

func validatePlan(plan FixPlan) error {
	for _, it := range plan.Items {
		switch it.Strategy {
		case FixStrip, FixFillDefault, FixCoerce, FixClear,
			FixMintUUID, FixRestamp, FixSkip:
		default:
			return fmt.Errorf(
				"integrity: unknown strategy %q for kind %q", it.Strategy, it.Kind,
			)
		}
	}
	return nil
}

// applyStrategy mutates `draft` in-place to repair `iss`. Returns
// (true, note, nil) when the mutation succeeded; (false, note, nil)
// when the strategy was inapplicable to this specific issue (e.g.
// Coerce on an unparseable value). Hard errors bubble out.
func applyStrategy(tpl *template.Template, draft *storage.Form, iss Issue, strat FixStrategy) (bool, string, error) {
	switch strat {
	case FixSkip:
		return false, "", nil

	case FixStrip:
		// Only meaningful for extra_field.
		if iss.Kind != IssueExtraField {
			return false, "strip only applies to extra_field", nil
		}
		return mutateAtPath(draft.Data, iss.Path, func(parent map[string]any, key string) error {
			delete(parent, key)
			return nil
		})

	case FixFillDefault:
		if iss.Kind != IssueMissingField {
			return false, "fill_default only applies to missing_field", nil
		}
		f := lookupField(tpl, iss.Path)
		if f == nil {
			return false, fmt.Sprintf("no template field for %q", iss.Path), nil
		}
		return setAtPath(draft.Data, iss.Path, defaultForFieldType(f.Type))

	case FixCoerce:
		f := lookupField(tpl, iss.Path)
		if f == nil {
			return false, fmt.Sprintf("no template field for %q", iss.Path), nil
		}
		return mutateAtPath(draft.Data, iss.Path, func(parent map[string]any, key string) error {
			coerced, ok := coerceForFieldType(f.Type, parent[key])
			if !ok {
				return errCoerceFailed
			}
			parent[key] = coerced
			return nil
		})

	case FixClear:
		f := lookupField(tpl, iss.Path)
		if f == nil {
			return false, fmt.Sprintf("no template field for %q", iss.Path), nil
		}
		return setAtPath(draft.Data, iss.Path, defaultForFieldType(f.Type))

	case FixMintUUID:
		if iss.Kind != IssueMetaMissing || iss.Path != "meta.id" {
			return false, "mint_uuid only applies to meta.id", nil
		}
		draft.Meta.ID = uuid.NewString()
		return true, "", nil

	case FixRestamp:
		if iss.Kind != IssueMetaBadFormat {
			return false, "restamp only applies to meta_bad_format", nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		switch iss.Path {
		case "meta.created":
			draft.Meta.Created.At = now
		case "meta.updated":
			draft.Meta.Updated.At = now
		case "meta.flag_state":
			// "Restamp" on flag_state = clear the stale label; the
			// concrete change is different but the user intent is the
			// same ("make this meta key valid").
			draft.Meta.FlagState = ""
		default:
			return false, fmt.Sprintf("unsupported meta path %q", iss.Path), nil
		}
		return true, "", nil
	}
	return false, "", fmt.Errorf("integrity: unhandled strategy %q", strat)
}

var errCoerceFailed = mutationError("coerce failed")

type mutationError string

func (e mutationError) Error() string { return string(e) }

// mutateAtPath walks `data` following a dotted/indexed path
// ("items[0].name") and runs `op` on the leaf's parent map. Returns
// (true, "", nil) on success. If `op` returns errCoerceFailed the
// helper reports (false, "coerce failed", nil); any other error bubbles.
func mutateAtPath(data map[string]any, path string, op func(parent map[string]any, key string) error) (bool, string, error) {
	parent, key, err := walkPath(data, path, false)
	if err != nil {
		return false, fmt.Sprintf("walk %q: %v", path, err), nil
	}
	if parent == nil {
		return false, fmt.Sprintf("path %q not reachable", path), nil
	}
	if err := op(parent, key); err != nil {
		if err == errCoerceFailed {
			return false, "coerce failed for " + path, nil
		}
		return false, "", err
	}
	return true, "", nil
}

func setAtPath(data map[string]any, path string, value any) (bool, string, error) {
	parent, key, err := walkPath(data, path, true)
	if err != nil {
		return false, fmt.Sprintf("walk %q: %v", path, err), nil
	}
	if parent == nil {
		return false, fmt.Sprintf("path %q not reachable", path), nil
	}
	parent[key] = value
	return true, "", nil
}

// walkPath descends `data` to the leaf's parent. "items[0].name" lands
// on `data["items"].([]any)[0].(map[string]any)`, leafKey="name".
// `createMissing` controls whether intermediate keys that the parent
// doesn't have should error or be created on the fly (needed for
// FillDefault on a missing top-level key).
func walkPath(data map[string]any, path string, createMissing bool) (map[string]any, string, error) {
	segments, err := tokenizePath(path)
	if err != nil {
		return nil, "", err
	}
	var cur any = data
	for i, seg := range segments[:len(segments)-1] {
		switch s := seg.(type) {
		case string:
			parent, ok := cur.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("segment %d: not a map", i)
			}
			next, exists := parent[s]
			if !exists {
				if createMissing {
					nm := map[string]any{}
					parent[s] = nm
					cur = nm
					continue
				}
				return nil, "", fmt.Errorf("segment %d: key %q missing", i, s)
			}
			cur = next
		case int:
			arr, ok := cur.([]any)
			if !ok {
				return nil, "", fmt.Errorf("segment %d: not an array", i)
			}
			if s < 0 || s >= len(arr) {
				return nil, "", fmt.Errorf("segment %d: index %d out of range", i, s)
			}
			cur = arr[s]
		}
	}

	last := segments[len(segments)-1]
	leafKey, ok := last.(string)
	if !ok {
		return nil, "", fmt.Errorf("path %q must end in a map key", path)
	}
	parent, ok := cur.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("leaf parent for %q is not a map", path)
	}
	return parent, leafKey, nil
}

// tokenizePath splits "items[0].name" into ["items", 0, "name"]. The
// path grammar matches what analyzeForm produces (dots + bracketed
// integers); no escaping needed because field keys are slug-shaped.
func tokenizePath(path string) ([]any, error) {
	out := []any{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch c {
		case '.':
			flush()
		case '[':
			flush()
			end := strings.IndexByte(path[i+1:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed bracket at %d in %q", i, path)
			}
			idx, err := strconv.Atoi(path[i+1 : i+1+end])
			if err != nil {
				return nil, fmt.Errorf("non-integer index in %q", path)
			}
			out = append(out, idx)
			i += end + 1
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	if len(out) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	return out, nil
}

// lookupField finds the template.Field whose Key matches the leaf of
// `path`. Loop-nested paths resolve by walking back from the leaf —
// the analyzer emits paths shaped "loopKey[idx].innerKey", so the leaf
// segment is the field key.
func lookupField(tpl *template.Template, path string) *template.Field {
	segs, err := tokenizePath(path)
	if err != nil || len(segs) == 0 {
		return nil
	}
	leaf, ok := segs[len(segs)-1].(string)
	if !ok {
		return nil
	}
	for i := range tpl.Fields {
		if tpl.Fields[i].Key == leaf {
			return &tpl.Fields[i]
		}
	}
	return nil
}

// defaultForFieldType mirrors storage.Sanitize's defaultForType. Kept
// local so the integrity module doesn't reach into storage internals.
func defaultForFieldType(t string) any {
	switch t {
	case "boolean":
		return false
	case "number":
		return float64(0)
	case "range":
		return float64(50)
	case "multioption", "list", "table", "tags":
		return []any{}
	case "api":
		return nil
	default:
		return ""
	}
}

// coerceForFieldType attempts to fit `v` into the field's declared type.
// Returns (newValue, true) on success, (_, false) when the conversion
// is unsafe and the strategy should be reported as skipped.
func coerceForFieldType(fieldType string, v any) (any, bool) {
	switch fieldType {
	case "text", "textarea", "dropdown", "radio",
		"file-path", "folder-path", "image", "guid":
		return fmt.Sprint(v), true

	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		for _, layout := range []string{
			"2006-01-02",
			"02/01/2006",
			"01/02/2006",
			"02-01-2006",
			"01-02-2006",
			"2006/01/02",
		} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Format("2006-01-02"), true
			}
		}
		return nil, false

	case "boolean":
		switch x := v.(type) {
		case bool:
			return x, true
		case string:
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "true", "yes", "1", "on":
				return true, true
			case "false", "no", "0", "off":
				return false, true
			}
		}
		return nil, false

	case "number", "range":
		switch x := v.(type) {
		case float64:
			return x, true
		case int:
			return float64(x), true
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				return nil, false
			}
			return f, true
		}
		return nil, false

	case "tags", "multioption", "list":
		switch x := v.(type) {
		case []any:
			return x, true
		case string:
			parts := strings.FieldsFunc(x, func(r rune) bool {
				return r == ',' || r == ';'
			})
			out := make([]any, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				out = append(out, p)
			}
			return out, true
		}
		return nil, false

	case "link":
		// Tolerant of both shapes — analyzer only flagged this when the
		// value was something else entirely.
		switch x := v.(type) {
		case string:
			return map[string]any{"href": x, "text": ""}, true
		case map[string]any:
			return x, true
		}
		return nil, false
	}
	// Unknown / structurally rich types (table, api): no automatic
	// coercion. Use FixClear instead.
	return nil, false
}

// cloneForm makes a deep-ish copy of a form so applyStrategy can mutate
// freely. We don't need a true deep copy because the integrity flow
// only mutates the top-level data map, the meta struct, and the inner
// maps of loop items.
func cloneForm(f *storage.Form) *storage.Form {
	if f == nil {
		return nil
	}
	out := *f
	out.Data = cloneMap(f.Data)
	return &out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneMap(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = cloneValue(item)
		}
		return out
	}
	return v
}
