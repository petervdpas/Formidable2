package integrity

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Per-field-type coverage. For every known field type, fixtures
// declare:
//
//   - happy: a value the field stores in normal use (round-tripped
//     through the on-disk JSON form).
//   - unhappy: a value that is unambiguously the wrong shape (different
//     Go type entirely) - must produce IssueTypeMismatch (or
//     IssueBadDateFormat for date strings that fail parse).
//   - empty: the zero value Sanitize would default to. Always allowed.
//
// Tests run the analyzer against a one-field template with one form
// holding the value, then assert on the issue list.
//
// The shapes here are not invented - they are sourced from the matching
// FormField<Type>.vue component's modelValue contract. Keep this list
// in lockstep when a field's wire format changes.

type fieldTypeCase struct {
	fieldType  string
	happy      any
	unhappy    any
	wantKind   IssueKind // expected on unhappy
	emptyValue any       // value Sanitize uses for an unset field
	skipEmpty  bool      // for types where "nil unset" needs its own coverage
}

func fieldTypeCases() []fieldTypeCase {
	return []fieldTypeCase{
		{fieldType: "text", happy: "hi", unhappy: float64(7), wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "textarea", happy: "long body", unhappy: true, wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "dropdown", happy: "option-a", unhappy: float64(3), wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "radio", happy: "yes", unhappy: []any{"yes"}, wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "file-path", happy: "/tmp/x.txt", unhappy: map[string]any{"x": 1}, wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "folder-path", happy: "/var/log", unhappy: 5, wantKind: IssueTypeMismatch, emptyValue: ""},
		{fieldType: "image", happy: "screen.png", unhappy: float64(0), wantKind: IssueTypeMismatch, emptyValue: ""},
		// guid's data value must mirror meta.id (the harness sets meta.id
		// to "fixture-id"), so the happy value matches it; an empty guid
		// is the guid_unsynced drift covered by its own test below.
		{fieldType: "guid", happy: "fixture-id", unhappy: 42, wantKind: IssueTypeMismatch, emptyValue: "", skipEmpty: true},

		{fieldType: "date", happy: "2026-06-01", unhappy: float64(20260601), wantKind: IssueTypeMismatch, emptyValue: ""},

		{fieldType: "boolean", happy: true, unhappy: "yes", wantKind: IssueTypeMismatch, emptyValue: false},

		{fieldType: "number", happy: float64(7), unhappy: "seven", wantKind: IssueTypeMismatch, emptyValue: float64(0)},
		{fieldType: "range", happy: float64(42), unhappy: "high", wantKind: IssueTypeMismatch, emptyValue: float64(50)},

		{fieldType: "tags", happy: []any{"a", "b"}, unhappy: "a,b", wantKind: IssueTypeMismatch, emptyValue: []any{}},
		{fieldType: "multioption", happy: []any{"x"}, unhappy: "x", wantKind: IssueTypeMismatch, emptyValue: []any{}},
		{fieldType: "list", happy: []any{"one", "two"}, unhappy: "one", wantKind: IssueTypeMismatch, emptyValue: []any{}},
		{fieldType: "table", happy: []any{[]any{"r1c1", "r1c2"}}, unhappy: "[1,2]", wantKind: IssueTypeMismatch, emptyValue: []any{}},

		// link accepts map OR legacy string - both pass; only deeply wrong
		// types fail. Happy and emptyValue cover the canonical shape;
		// link's "legacy string" path has its own dedicated test below.
		{fieldType: "link", happy: map[string]any{"href": "https://x", "text": "X"}, unhappy: float64(7), wantKind: IssueTypeMismatch, emptyValue: map[string]any{"href": "", "text": ""}},

		// api: a reference id string (single) and a list of ids (to-many) are valid;
		// the legacy {id|guid, ...} snapshot map is tolerated. A deeply wrong type
		// (number/bool) is drift and is flagged. nil (unset) is covered separately.
		{fieldType: "api", happy: "abc-123", unhappy: float64(7), wantKind: IssueTypeMismatch, emptyValue: nil, skipEmpty: true},

		// slide: an object with a blocks array; each block is an existing-typed
		// value plus geometry. A deeply wrong type (number) is drift; the empty
		// value is the {blocks:[]} Sanitize seeds.
		{
			fieldType: "slide",
			happy: map[string]any{"blocks": []any{
				map[string]any{"id": "b1", "kind": "text", "content": "hi",
					"x": float64(0), "y": float64(0), "w": float64(100), "h": float64(80)},
			}},
			unhappy:    float64(7),
			wantKind:   IssueTypeMismatch,
			emptyValue: map[string]any{"blocks": []any{}},
		},

		// event: an object with ISO start/end, a kind, and a resource. A deeply
		// wrong type (number) is drift; the empty value is what Sanitize seeds.
		{
			fieldType:  "event",
			happy:      map[string]any{"start": "2026-06-27", "end": "2026-08-16", "kind": "task", "resource": "dev", "description": "x"},
			unhappy:    float64(7),
			wantKind:   IssueTypeMismatch,
			emptyValue: map[string]any{"start": "", "end": "", "kind": "task", "resource": "", "description": ""},
		},

		// project: an object with a board name (the axis lives in field options,
		// not record data). A deeply wrong type (number) is drift; the empty
		// value is what Sanitize seeds.
		{
			fieldType:  "project",
			happy:      map[string]any{"name": "HR2DAY connector"},
			unhappy:    float64(7),
			wantKind:   IssueTypeMismatch,
			emptyValue: map[string]any{"name": ""},
		},
	}
}

func runFieldTypeCase(t *testing.T, tc fieldTypeCase, value any) Report {
	t.Helper()
	tpl := &template.Template{
		Name:     "OneField",
		Filename: "one.yaml",
		Fields:   []template.Field{{Key: "v", Type: tc.fieldType}},
	}
	form := &storage.Form{
		Meta: storage.FormMeta{
			ID:      "fixture-id", // satisfies the guid-field meta check
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
		},
		Data: map[string]any{"v": value},
	}
	m := newM(t, tpl, map[string]*storage.Form{"a.meta.json": form})
	r, err := m.AnalyzeTemplate("one.yaml")
	if err != nil {
		t.Fatalf("[%s] analyze: %v", tc.fieldType, err)
	}
	return r
}

func TestFieldType_HappyPathProducesNoIssues(t *testing.T) {
	for _, tc := range fieldTypeCases() {
		t.Run(tc.fieldType, func(t *testing.T) {
			r := runFieldTypeCase(t, tc, tc.happy)
			if r.IssueCount != 0 {
				t.Errorf("expected 0 issues for happy %s=%v, got %d: %+v",
					tc.fieldType, tc.happy, r.IssueCount, r.Forms)
			}
		})
	}
}

func TestFieldType_UnhappyPathProducesExpectedIssue(t *testing.T) {
	for _, tc := range fieldTypeCases() {
		t.Run(tc.fieldType, func(t *testing.T) {
			r := runFieldTypeCase(t, tc, tc.unhappy)
			if r.IssueCount == 0 {
				t.Fatalf("expected an issue for unhappy %s=%v, got none",
					tc.fieldType, tc.unhappy)
			}
			findIssue(t, r, "a.meta.json", tc.wantKind, "v")
		})
	}
}

func TestFieldType_EmptyValueIsAllowed(t *testing.T) {
	for _, tc := range fieldTypeCases() {
		if tc.skipEmpty {
			continue
		}
		t.Run(tc.fieldType, func(t *testing.T) {
			r := runFieldTypeCase(t, tc, tc.emptyValue)
			if r.IssueCount != 0 {
				t.Errorf("empty %s=%v should not flag, got %d: %+v",
					tc.fieldType, tc.emptyValue, r.IssueCount, r.Forms)
			}
		})
	}
}

// Dedicated coverage for shapes that have a canonical + legacy form.
// These are the false-positive regressions we just hit in CH.06 where
// `audit-control-link` in a loop tripped on link={href,text} maps.

func TestFieldType_Link_AcceptsCanonicalMap(t *testing.T) {
	r := runFieldTypeCase(t,
		fieldTypeCase{fieldType: "link"},
		map[string]any{"href": "https://example.com", "text": "Example"})
	mustZeroIssues(t, r)
}

func TestFieldType_Link_AcceptsLegacyString(t *testing.T) {
	r := runFieldTypeCase(t,
		fieldTypeCase{fieldType: "link"},
		"https://legacy.example.com")
	mustZeroIssues(t, r)
}

func TestFieldType_Link_AcceptsEmptyMap(t *testing.T) {
	r := runFieldTypeCase(t,
		fieldTypeCase{fieldType: "link"},
		map[string]any{"href": "", "text": ""})
	mustZeroIssues(t, r)
}

func TestFieldType_Guid_EmptyDataIsUnsyncedDrift(t *testing.T) {
	// An empty guid data field while meta.id is set is the exact bug the
	// cleanup tool repairs: the data block never mirrored the canonical id.
	r := runFieldTypeCase(t,
		fieldTypeCase{fieldType: "guid"},
		"")
	findIssue(t, r, "a.meta.json", IssueGuidUnsynced, "v")
}

func TestFieldType_API_AcceptsNil(t *testing.T) {
	// `api` defaults to nil for an unset row picker - sanitize.go's
	// defaultForType returns nil specifically for this type so the UI
	// can distinguish "no pick yet" from a stamped {guid, ...}.
	r := runFieldTypeCase(t,
		fieldTypeCase{fieldType: "api"},
		nil)
	mustZeroIssues(t, r)
}

// And the loop-context regression itself: a link inside a loop with a
// map value must not flag. This is the exact shape from CH.06.

func TestFieldType_Link_InsideLoop(t *testing.T) {
	tpl := &template.Template{
		Name: "LoopLinks", Filename: "loop-links.yaml",
		Fields: []template.Field{
			{Key: "links", Type: "loopstart"},
			{Key: "link", Type: "link"},
			{Key: "links", Type: "loopstop"},
		},
	}
	form := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
		},
		Data: map[string]any{
			"links": []any{
				map[string]any{"link": map[string]any{"href": "https://a", "text": "A"}},
				map[string]any{"link": map[string]any{"href": "https://b", "text": "B"}},
			},
		},
	}
	m := newM(t, tpl, map[string]*storage.Form{"a.meta.json": form})
	r, err := m.AnalyzeTemplate("loop-links.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mustZeroIssues(t, r)
}
