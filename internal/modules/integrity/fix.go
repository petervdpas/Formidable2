package integrity

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// StorageWriter is the post-Fix write surface; the full *storage.Form lets meta strategies (MintUUID, Restamp) commit a mutated meta block.
type StorageWriter interface {
	SaveForm(ctx context.Context, templateFilename, datafile string, form *storage.Form) error
}

// SetWriter installs the writer the Fix pipeline uses (Analyze works without it; FixTemplate errors).
func (m *Manager) SetWriter(w StorageWriter) { m.writer = w }

// FixTemplate applies a FixPlan, writing only the forms that actually changed, then re-analyzes
// to populate ScannedAfter. Issues whose kind isn't in the plan are left untouched.
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

	dupGuids := m.duplicateGuidByFile(templateFilename, tpl, filenames)

	for _, fn := range filenames {
		original := m.storage.LoadForm(templateFilename, fn)
		if original == nil {
			// Unreadable: every strategy is moot, count once and move on.
			if _, has := strategyByKind[IssueUnreadable]; has {
				result.Outcomes = append(result.Outcomes, FixOutcome{
					Filename: fn,
					Skipped:  1,
					Notes:    []string{"form unreadable - repair must happen outside the app"},
				})
				result.Skipped++
			}
			continue
		}

		// Clone so we can mutate without disturbing the storage view between iterations.
		draft := cloneForm(original)
		issues := analyzeForm(tpl, draft)
		issues = append(issues, m.guidSyncIssues(templateFilename, fn, tpl, draft.Meta, draft.Data)...)
		issues = append(issues, m.facetSeedingIssues(templateFilename, fn, tpl)...)
		if g, ok := dupGuids[fn]; ok {
			issues = append(issues, Issue{Kind: IssueDuplicateGuid, Path: "meta.id", Value: g})
		}

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
			if err := m.writer.SaveForm(context.Background(), templateFilename, fn, draft); err != nil {
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
			FixMintUUID, FixSyncGuid, FixRestamp, FixSeedFacet, FixSkip:
		default:
			return fmt.Errorf(
				"integrity: unknown strategy %q for kind %q", it.Strategy, it.Kind,
			)
		}
	}
	return nil
}

// applyStrategy mutates draft in-place to repair iss; (true,...) on success, (false,...) when inapplicable, error bubbles.
func applyStrategy(tpl *template.Template, draft *storage.Form, iss Issue, strat FixStrategy) (bool, string, error) {
	switch strat {
	case FixSkip:
		return false, "", nil

	case FixStrip:
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
		// date_anomaly has no safe conversion (it didn't fit the inferred format); never guess.
		if iss.Kind == IssueDateAnomaly {
			return false, "date anomaly: fix by hand or Clear", nil
		}
		// bad_date_format (incl. table cells) goes through leafAccess; cells carry inferred ISO in Suggest.
		if iss.Kind == IssueBadDateFormat {
			get, set, err := leafAccess(draft.Data, iss.Path)
			if err != nil {
				return false, fmt.Sprintf("walk %q: %v", iss.Path, err), nil
			}
			if iss.Suggest != "" {
				set(iss.Suggest)
				return true, "", nil
			}
			coerced, ok := coerceForFieldType("date", get())
			if !ok {
				return false, "coerce failed for " + iss.Path, nil
			}
			set(coerced)
			return true, "", nil
		}
		// Table-cell mismatch: coerce against the column type (leaf is an array index, not a field).
		if colType, ok := columnTypeForTablePath(tpl, iss.Path); ok {
			get, set, err := leafAccess(draft.Data, iss.Path)
			if err != nil {
				return false, fmt.Sprintf("walk %q: %v", iss.Path, err), nil
			}
			coerced, ok := coerceForFieldType(columnCoerceType(colType), get())
			if !ok {
				return false, "coerce failed for " + iss.Path, nil
			}
			set(coerced)
			return true, "", nil
		}
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
		// A bad/anomalous date clears to "" via leafAccess (handles both top-level fields and table cells).
		if iss.Kind == IssueBadDateFormat || iss.Kind == IssueDateAnomaly {
			_, set, err := leafAccess(draft.Data, iss.Path)
			if err != nil {
				return false, fmt.Sprintf("walk %q: %v", iss.Path, err), nil
			}
			set("")
			return true, "", nil
		}
		if colType, ok := columnTypeForTablePath(tpl, iss.Path); ok {
			_, set, err := leafAccess(draft.Data, iss.Path)
			if err != nil {
				return false, fmt.Sprintf("walk %q: %v", iss.Path, err), nil
			}
			set(defaultForFieldType(columnCoerceType(colType)))
			return true, "", nil
		}
		f := lookupField(tpl, iss.Path)
		if f == nil {
			return false, fmt.Sprintf("no template field for %q", iss.Path), nil
		}
		return setAtPath(draft.Data, iss.Path, defaultForFieldType(f.Type))

	case FixMintUUID:
		switch iss.Kind {
		case IssueMetaMissing:
			if iss.Path != "meta.id" {
				return false, "mint_uuid only applies to meta.id", nil
			}
			// One rule (storage.CanonicalGuid): data leads, so copy an existing data
			// id onto meta rather than minting a fresh one; mint only when both empty.
			gk := guidFieldKeyOf(tpl)
			dataID := ""
			if gk != "" {
				dataID, _ = draft.Data[gk].(string)
			}
			id := storage.CanonicalGuid(true, dataID, draft.Meta.ID)
			draft.Meta.ID = id
			if gk != "" {
				draft.Data[gk] = id
			}
			return true, "", nil
		case IssueDuplicateGuid:
			// Uniqueness is a separate invariant from the canonical-guid rule: a
			// shared guid must be replaced with a fresh one in both data and meta.
			fresh := uuid.NewString()
			if gk := guidFieldKeyOf(tpl); gk != "" {
				draft.Data[gk] = fresh
			}
			draft.Meta.ID = fresh
			return true, "", nil
		default:
			return false, "mint_uuid only applies to meta.id / duplicate_guid", nil
		}

	case FixSyncGuid:
		if iss.Kind != IssueGuidUnsynced {
			return false, "sync_guid only applies to guid_unsynced", nil
		}
		// One rule (storage.CanonicalGuid): data leads, meta mirrors it, and an empty
		// field backfills from meta. No mint here: an unsynced issue means at least
		// one side is set, and an all-empty pair is reported as an error.
		field, _ := draft.Data[iss.Path].(string)
		id := storage.CanonicalGuid(false, field, draft.Meta.ID)
		if id == "" {
			return false, "guid field and meta.id are both empty", nil
		}
		draft.Meta.ID = id
		return setAtPath(draft.Data, iss.Path, id)

	case FixRestamp:
		if iss.Kind != IssueMetaBadFormat {
			return false, "restamp only applies to meta_bad_format", nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		switch {
		case iss.Path == "meta.created":
			draft.Meta.Created.At = now
		case iss.Path == "meta.updated":
			draft.Meta.Updated.At = now
		case strings.HasPrefix(iss.Path, "meta.facets."):
			// .selected clears just the option; the bare key drops the whole facet entry.
			rest := strings.TrimPrefix(iss.Path, "meta.facets.")
			key, suffix, _ := strings.Cut(rest, ".")
			if suffix == "selected" {
				state := draft.Meta.Facets[key]
				state.Selected = ""
				draft.Meta.Facets[key] = state
			} else {
				delete(draft.Meta.Facets, key)
			}
		default:
			return false, fmt.Sprintf("unsupported meta path %q", iss.Path), nil
		}
		return true, "", nil

	case FixSeedFacet:
		if iss.Kind != IssueFacetUnseeded {
			return false, "seed_facet only applies to facet_unseeded", nil
		}
		key := strings.TrimPrefix(iss.Path, "meta.facets.")
		if draft.Meta.Facets == nil {
			draft.Meta.Facets = map[string]storage.FacetState{}
		}
		draft.Meta.Facets[key] = storage.FacetState{Set: true, Selected: iss.Suggest}
		return true, "", nil
	}
	return false, "", fmt.Errorf("integrity: unhandled strategy %q", strat)
}

var errCoerceFailed = mutationError("coerce failed")

type mutationError string

func (e mutationError) Error() string { return string(e) }

// mutateAtPath walks the dotted/indexed path and runs op on the leaf's parent map; errCoerceFailed reports a skip, other errors bubble.
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

// walkPath descends to the leaf's parent map; createMissing creates absent intermediate keys (needed for FillDefault).
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

// columnTypeForTablePath resolves the column type a table-cell path addresses; ("", false) when not a table-cell shape.
// Table fields are flat in tpl.Fields even when loop-nested, so the table is the key just before [row][col].
func columnTypeForTablePath(tpl *template.Template, path string) (string, bool) {
	segs, err := tokenizePath(path)
	if err != nil || len(segs) < 3 {
		return "", false
	}
	col, ok := segs[len(segs)-1].(int)
	if !ok {
		return "", false
	}
	if _, ok := segs[len(segs)-2].(int); !ok {
		return "", false
	}
	tableKey, ok := segs[len(segs)-3].(string)
	if !ok {
		return "", false
	}
	for i := range tpl.Fields {
		if tpl.Fields[i].Key == tableKey && tpl.Fields[i].Type == "table" {
			if col < 0 || col >= len(tpl.Fields[i].Options) {
				return "", false
			}
			return columnType(tpl.Fields[i].Options[col]), true
		}
	}
	return "", false
}

// columnCoerceType maps a table column type onto the field-type vocabulary ("bool"->"boolean", "string"->"text").
func columnCoerceType(colType string) string {
	switch colType {
	case "bool":
		return "boolean"
	case "string":
		return "text"
	default:
		return colType
	}
}

// leafAccess returns a getter/setter for the leaf, supporting an array-index leaf ("table[0][1]") as well as
// a map-key leaf. cloneForm deep-copies the row slices so writes through the setter persist into draft.Data.
func leafAccess(data map[string]any, path string) (func() any, func(any), error) {
	segs, err := tokenizePath(path)
	if err != nil {
		return nil, nil, err
	}
	var cur any = data
	for i, seg := range segs[:len(segs)-1] {
		switch s := seg.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("segment %d: not a map", i)
			}
			next, exists := m[s]
			if !exists {
				return nil, nil, fmt.Errorf("segment %d: key %q missing", i, s)
			}
			cur = next
		case int:
			arr, ok := cur.([]any)
			if !ok {
				return nil, nil, fmt.Errorf("segment %d: not an array", i)
			}
			if s < 0 || s >= len(arr) {
				return nil, nil, fmt.Errorf("segment %d: index %d out of range", i, s)
			}
			cur = arr[s]
		}
	}

	switch last := segs[len(segs)-1].(type) {
	case string:
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("leaf parent for %q is not a map", path)
		}
		return func() any { return m[last] }, func(v any) { m[last] = v }, nil
	case int:
		arr, ok := cur.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("leaf parent for %q is not an array", path)
		}
		if last < 0 || last >= len(arr) {
			return nil, nil, fmt.Errorf("leaf index %d out of range for %q", last, path)
		}
		return func() any { return arr[last] }, func(v any) { arr[last] = v }, nil
	}
	return nil, nil, fmt.Errorf("bad leaf in %q", path)
}

// tokenizePath splits "items[0].name" into ["items", 0, "name"]; slug-shaped keys need no escaping.
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

// lookupField finds the Field whose Key matches the path's leaf segment (the field key, even loop-nested).
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

// defaultForFieldType mirrors storage.Sanitize's defaultForType, kept local to avoid reaching into storage internals.
func defaultForFieldType(t string) any {
	switch t {
	case "boolean":
		return false
	case "number", "sequence":
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

// coerceForFieldType fits v into the field's type; (_, false) when the conversion is unsafe (reported as skipped).
func coerceForFieldType(fieldType string, v any) (any, bool) {
	switch fieldType {
	case "text", "textarea", "mermaid", "dropdown", "radio",
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

	case "number", "range", "sequence":
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
		switch x := v.(type) {
		case string:
			return map[string]any{"href": x, "text": ""}, true
		case map[string]any:
			return x, true
		}
		return nil, false
	}
	// Structurally rich types (table, api): no automatic coercion. Use FixClear.
	return nil, false
}

// cloneForm deep-copies the data map (and nested loop items) so applyStrategy can mutate freely.
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
