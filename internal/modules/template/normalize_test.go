package template

import (
	"reflect"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────
// Normalize - textarea format
//
// Mirrors `schemas/field.schema.js`:
//
//   const textareaFormats = new Set(["markdown", "plain"]);
//   if (field.type === "textarea") {
//     const f = String(field.format || "").toLowerCase();
//     field.format = textareaFormats.has(f) ? f : "markdown";
//   } else {
//     delete field.format;
//   }
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_TextareaMissingFormatDefaultsToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("missing format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaEmptyFormatDefaultsToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: ""}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("empty format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaUnknownFormatFallsBackToMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "html"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("unknown format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaPreservesMarkdown(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "markdown"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("markdown: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_TextareaPreservesPlain(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "plain"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "plain" {
		t.Errorf("plain: want %q, got %q", "plain", got)
	}
}

func TestNormalize_TextareaLowercasesFormat(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "a", Type: "textarea", Format: "MARKDOWN"},
		{Key: "b", Type: "textarea", Format: "Plain"},
	}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("MARKDOWN → want %q, got %q", "markdown", got)
	}
	if got := tpl.Fields[1].Format; got != "plain" {
		t.Errorf("Plain → want %q, got %q", "plain", got)
	}
}

func TestNormalize_TextareaTrimsWhitespace(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "k", Type: "textarea", Format: "  markdown  "}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "markdown" {
		t.Errorf("padded format: want %q, got %q", "markdown", got)
	}
}

func TestNormalize_NonTextareaStripsFormat(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "t", Type: "text", Format: "markdown"},
		{Key: "n", Type: "number", Format: "plain"},
		{Key: "b", Type: "boolean", Format: "anything"},
	}}
	Normalize(tpl)
	for _, f := range tpl.Fields {
		if f.Format != "" {
			t.Errorf("non-textarea %s should have empty Format, got %q", f.Type, f.Format)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Normalize - statistics columns (table only, deduped, validated)
// ─────────────────────────────────────────────────────────────────────

func statTableField(statCols []string, use bool) Field {
	return Field{
		Key:               "items",
		Type:              "table",
		UseInStatistics:   use,
		StatisticsColumns: statCols,
		Options: []any{
			map[string]any{"value": "name", "type": "string"},
			map[string]any{"value": "qty", "type": "number"},
		},
	}
}

func TestNormalize_StatisticsColumns_Dedupes(t *testing.T) {
	tpl := &Template{Fields: []Field{statTableField([]string{"qty", "qty", "name"}, true)}}
	Normalize(tpl)
	got := tpl.Fields[0].StatisticsColumns
	want := []string{"qty", "name"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StatisticsColumns = %v, want %v (deduped, order preserved)", got, want)
	}
}

func TestNormalize_StatisticsColumns_DropsUnknownColumns(t *testing.T) {
	tpl := &Template{Fields: []Field{statTableField([]string{"qty", "ghost"}, true)}}
	Normalize(tpl)
	got := tpl.Fields[0].StatisticsColumns
	if !reflect.DeepEqual(got, []string{"qty"}) {
		t.Errorf("StatisticsColumns = %v, want [qty] (unknown dropped)", got)
	}
}

func TestNormalize_StatisticsColumns_ClearedWhenNotFlagged(t *testing.T) {
	tpl := &Template{Fields: []Field{statTableField([]string{"qty"}, false)}}
	Normalize(tpl)
	if tpl.Fields[0].StatisticsColumns != nil {
		t.Errorf("StatisticsColumns = %v, want nil when use_in_statistics is false", tpl.Fields[0].StatisticsColumns)
	}
}

func TestNormalize_StatisticsColumns_ClearedOnNonTable(t *testing.T) {
	tpl := &Template{Fields: []Field{{
		Key: "x", Type: "number", UseInStatistics: true,
		StatisticsColumns: []string{"qty"},
	}}}
	Normalize(tpl)
	if tpl.Fields[0].StatisticsColumns != nil {
		t.Errorf("StatisticsColumns = %v, want nil on non-table field", tpl.Fields[0].StatisticsColumns)
	}
}

func TestNormalize_StatisticsColumns_AllInvalidBecomesNil(t *testing.T) {
	tpl := &Template{Fields: []Field{statTableField([]string{"ghost", "ghost2"}, true)}}
	Normalize(tpl)
	if tpl.Fields[0].StatisticsColumns != nil {
		t.Errorf("StatisticsColumns = %v, want nil when nothing valid remains", tpl.Fields[0].StatisticsColumns)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Normalize - statistical objects (Statistical Engine specs on a template)
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_Statistics_TrimsAndDropsEmpty(t *testing.T) {
	tpl := &Template{Statistics: []Statistic{
		{Name: "  by-status  ", DSL: `count() by F["status"]`},
		{Name: "", DSL: `count()`},      // no name -> dropped
		{Name: "blank-dsl", DSL: "   "}, // no dsl -> dropped
	}}
	Normalize(tpl)
	if len(tpl.Statistics) != 1 {
		t.Fatalf("got %d statistics, want 1: %+v", len(tpl.Statistics), tpl.Statistics)
	}
	if tpl.Statistics[0].Name != "by-status" {
		t.Errorf("name = %q, want trimmed %q", tpl.Statistics[0].Name, "by-status")
	}
}

func TestNormalize_Statistics_DedupesByNameFirstWins(t *testing.T) {
	tpl := &Template{Statistics: []Statistic{
		{Name: "dup", DSL: `count() by F["a"]`},
		{Name: "dup", DSL: `count() by F["b"]`},
		{Name: "other", DSL: `count()`},
	}}
	Normalize(tpl)
	if len(tpl.Statistics) != 2 {
		t.Fatalf("got %d statistics, want 2 (deduped): %+v", len(tpl.Statistics), tpl.Statistics)
	}
	if tpl.Statistics[0].Name != "dup" || tpl.Statistics[0].DSL != `count() by F["a"]` {
		t.Errorf("first-wins broken: %+v", tpl.Statistics[0])
	}
	if tpl.Statistics[1].Name != "other" {
		t.Errorf("order not preserved: %+v", tpl.Statistics)
	}
}

func TestNormalize_Statistics_KeepsComposite_DropsParentlessOne(t *testing.T) {
	tpl := &Template{Statistics: []Statistic{
		{Name: "in-use", DSL: `count() by Facet["flag"]`},
		{Name: "in-use-by-app", Composite: &StatComposite{
			Parent: "in-use",
			Edges:  []StatCompositeEdge{{Branch: "IN GEBRUIK", Child: "applications"}},
		}},
		{Name: "no-parent", Composite: &StatComposite{}}, // composite without a parent -> dropped
	}}
	Normalize(tpl)
	if len(tpl.Statistics) != 2 {
		t.Fatalf("got %d statistics, want 2 (composite kept, parentless dropped): %+v", len(tpl.Statistics), tpl.Statistics)
	}
	c := tpl.Statistics[1]
	if c.Name != "in-use-by-app" || c.Composite == nil || c.Composite.Parent != "in-use" {
		t.Errorf("composite not preserved: %+v", c)
	}
}

func TestNormalize_Statistics_AllInvalidBecomesNil(t *testing.T) {
	tpl := &Template{Statistics: []Statistic{
		{Name: "", DSL: `count()`},
		{Name: "x", DSL: ""},
	}}
	Normalize(tpl)
	if tpl.Statistics != nil {
		t.Errorf("Statistics = %+v, want nil when nothing valid remains", tpl.Statistics)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Normalize - robustness (unhappy paths)
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_NilTemplateIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Normalize(nil) panicked: %v", r)
		}
	}()
	Normalize(nil)
}

func TestNormalize_NilFieldsIsSafe(t *testing.T) {
	tpl := &Template{Name: "X"}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Normalize on nil Fields panicked: %v", r)
		}
	}()
	Normalize(tpl)
}

func TestNormalize_EmptyFieldsSliceIsSafe(t *testing.T) {
	tpl := &Template{Fields: []Field{}}
	Normalize(tpl)
	if len(tpl.Fields) != 0 {
		t.Errorf("empty fields slice should stay empty")
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveTemplate integration - normalize runs on save
// ─────────────────────────────────────────────────────────────────────

func TestSaveTemplate_NormalizesTextareaFormat(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}

	// Save a textarea field with no format and a non-textarea field
	// carrying a stale format value.
	tpl := &Template{
		Name: "T",
		Fields: []Field{
			{Key: "notes", Type: "textarea"},
			{Key: "title", Type: "text", Format: "markdown"},
		},
	}
	if err := m.SaveTemplate("t.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	loaded, err := m.LoadTemplate("t.yaml")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if got := loaded.Fields[0].Format; got != "markdown" {
		t.Errorf("textarea on disk: want %q format, got %q", "markdown", got)
	}
	if got := loaded.Fields[1].Format; got != "" {
		t.Errorf("text on disk: format should be stripped, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Normalize - Default value type coercion
//
// The frontend FieldEditModal's "Default value" input is text-only and
// has no way to natively produce a number / bool / array. Without
// backend coercion every numeric default would round-trip as the
// string "17", and every new form would inherit a string in a number
// field - the exact "expected number, got string" drift Cleanup
// Storage flagged on the `numpy` field.
//
// Normalize is the authoritative pass: CSV imports, plugin-authored
// templates, and hand-edited YAML all heal on SaveTemplate.
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_NumberDefault_StringCoercesToFloat(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "n", Type: "number", Default: "17"}}}
	Normalize(tpl)
	got := tpl.Fields[0].Default
	if f, ok := got.(float64); !ok || f != 17 {
		t.Errorf("number default: want float64 17, got %v (%T)", got, got)
	}
}

func TestNormalize_NumberDefault_PreservesNumericTypes(t *testing.T) {
	for _, in := range []any{int(7), int64(7), float64(7), float32(7)} {
		tpl := &Template{Fields: []Field{{Key: "n", Type: "number", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if f, ok := got.(float64); !ok || f != 7 {
			t.Errorf("number default from %T(%v): want float64 7, got %v (%T)", in, in, got, got)
		}
	}
}

func TestNormalize_NumberDefault_UnparseableBecomesNil(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "n", Type: "number", Default: "seventeen"}}}
	Normalize(tpl)
	if tpl.Fields[0].Default != nil {
		t.Errorf("garbage number default should clear to nil, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_NumberDefault_EmptyStringClears(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "n", Type: "number", Default: ""}}}
	Normalize(tpl)
	if tpl.Fields[0].Default != nil {
		t.Errorf("empty number default should clear to nil, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_RangeDefault_BehavesLikeNumber(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "r", Type: "range", Default: "42"}}}
	Normalize(tpl)
	got := tpl.Fields[0].Default
	if f, ok := got.(float64); !ok || f != 42 {
		t.Errorf("range default: want float64 42, got %v (%T)", got, got)
	}
}

func TestNormalize_BooleanDefault_StringTruthyVariants(t *testing.T) {
	for _, in := range []string{"true", "True", "TRUE", "yes", "YES", "1", "on", "  true  "} {
		tpl := &Template{Fields: []Field{{Key: "b", Type: "boolean", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if b, ok := got.(bool); !ok || !b {
			t.Errorf("boolean default from %q: want true, got %v (%T)", in, got, got)
		}
	}
}

func TestNormalize_BooleanDefault_StringFalsyVariants(t *testing.T) {
	for _, in := range []string{"false", "False", "no", "0", "off", "  no  "} {
		tpl := &Template{Fields: []Field{{Key: "b", Type: "boolean", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if b, ok := got.(bool); !ok || b {
			t.Errorf("boolean default from %q: want false, got %v (%T)", in, got, got)
		}
	}
}

func TestNormalize_BooleanDefault_PreservesNativeBool(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "b", Type: "boolean", Default: true}}}
	Normalize(tpl)
	if b, ok := tpl.Fields[0].Default.(bool); !ok || !b {
		t.Errorf("native bool default: want true, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_BooleanDefault_GarbageStringClears(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "b", Type: "boolean", Default: "maybe"}}}
	Normalize(tpl)
	if tpl.Fields[0].Default != nil {
		t.Errorf("garbage boolean default should clear to nil, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_TagsDefault_StringSplitsOnCommaSemicolon(t *testing.T) {
	cases := map[string][]any{
		"alpha,beta,gamma":   {"alpha", "beta", "gamma"},
		"alpha; beta ; gamma": {"alpha", "beta", "gamma"},
		"  one  , two":       {"one", "two"},
		"solo":               {"solo"},
	}
	for in, want := range cases {
		tpl := &Template{Fields: []Field{{Key: "t", Type: "tags", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tags default from %q: want %v, got %v (%T)", in, want, got, got)
		}
	}
}

func TestNormalize_MultioptionDefault_SplitSemantics(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "m", Type: "multioption", Default: "a,b"}}}
	Normalize(tpl)
	want := []any{"a", "b"}
	if !reflect.DeepEqual(tpl.Fields[0].Default, want) {
		t.Errorf("multioption default: want %v, got %v", want, tpl.Fields[0].Default)
	}
}

func TestNormalize_ListDefault_SplitSemantics(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "l", Type: "list", Default: "a;b;c"}}}
	Normalize(tpl)
	want := []any{"a", "b", "c"}
	if !reflect.DeepEqual(tpl.Fields[0].Default, want) {
		t.Errorf("list default: want %v, got %v", want, tpl.Fields[0].Default)
	}
}

func TestNormalize_TagsDefault_PreservesArrayShape(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "t", Type: "tags", Default: []any{"a", "b"}}}}
	Normalize(tpl)
	want := []any{"a", "b"}
	if !reflect.DeepEqual(tpl.Fields[0].Default, want) {
		t.Errorf("array default: want preserved %v, got %v", want, tpl.Fields[0].Default)
	}
}

func TestNormalize_TextLikeDefault_PreservesString(t *testing.T) {
	for _, typ := range []string{"text", "textarea", "dropdown", "radio", "file-path", "folder-path", "link", "image"} {
		tpl := &Template{Fields: []Field{{Key: "f", Type: typ, Default: "hello"}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if s, ok := got.(string); !ok || s != "hello" {
			t.Errorf("%s default: want preserved string, got %v (%T)", typ, got, got)
		}
	}
}

func TestNormalize_TextShapeDefault_CoercesNonStringToString(t *testing.T) {
	// link is intentionally absent - link defaults legitimately come
	// in two shapes ({href,text} map or legacy string), so the
	// "coerce non-string to string" rule doesn't apply there.
	types := []string{"text", "textarea", "dropdown", "radio", "file-path", "folder-path", "image"}
	cases := map[any]string{
		42:           "42",
		float64(3.14): "3.14",
		true:         "true",
		false:        "false",
	}
	for _, typ := range types {
		for in, want := range cases {
			tpl := &Template{Fields: []Field{{Key: "f", Type: typ, Default: in}}}
			Normalize(tpl)
			got := tpl.Fields[0].Default
			if s, ok := got.(string); !ok || s != want {
				t.Errorf("%s default from %v (%T): want %q, got %v (%T)",
					typ, in, in, want, got, got)
			}
		}
	}
}

func TestNormalize_TextShapeDefault_CleansInvisibleChars(t *testing.T) {
	// NBSP (U+00A0) becomes a regular space; zero-width / BOM chars
	// (U+200B, U+200C, U+200D, U+2060, U+FEFF) are dropped entirely.
	// These are the typical garbage Word / web rich-text controls
	// leave in pasted strings.
	cases := map[string]string{
		"hello\u00A0world":            "hello world",         // NBSP \u2192 space
		"hello\u200Bworld":            "helloworld",          // zero-width space
		"\uFEFFhello":                 "hello",               // BOM at start
		"a\u200Cb\u200Dc":            "abc",                 // ZWNJ + ZWJ
		"plain":                       "plain",               // unchanged
		"keep \u2014 smart quotes":    "keep \u2014 smart quotes", // em-dash kept
	}
	for in, want := range cases {
		tpl := &Template{Fields: []Field{{Key: "f", Type: "text", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if s, ok := got.(string); !ok || s != want {
			t.Errorf("cleanText on %q: want %q, got %v (%T)", in, want, got, got)
		}
	}
}

func TestNormalize_TextShapeDefault_EmptyStringClears(t *testing.T) {
	// Empty string == no default - let storage.Sanitize fall back to
	// defaultForType ("") on first save rather than persisting "".
	for _, typ := range []string{"text", "textarea", "dropdown", "radio", "file-path", "folder-path", "image"} {
		tpl := &Template{Fields: []Field{{Key: "f", Type: typ, Default: "   "}}}
		Normalize(tpl)
		if tpl.Fields[0].Default != nil {
			t.Errorf("%s default of whitespace: want nil, got %v", typ, tpl.Fields[0].Default)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// Date default coercion - accept ISO, common locale formats, time.Time
// (yaml.v3 native), drop unparseable. Output is always ISO YYYY-MM-DD
// so the integrity analyzer's date check round-trips cleanly.
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_DateDefault_ISOPreserved(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: "2026-06-01"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Default; got != "2026-06-01" {
		t.Errorf("date default: want %q, got %v", "2026-06-01", got)
	}
}

func TestNormalize_DateDefault_LocaleFormatsCoerceToISO(t *testing.T) {
	cases := map[string]string{
		"21/07/2025": "2025-07-21",
		"21-07-2025": "2025-07-21",
		"2026/06/01": "2026-06-01",
	}
	for in, want := range cases {
		tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: in}}}
		Normalize(tpl)
		got := tpl.Fields[0].Default
		if got != want {
			t.Errorf("date default from %q: want %q, got %v", in, want, got)
		}
	}
}

func TestNormalize_DateDefault_TimeTimeCoercesToISO(t *testing.T) {
	// yaml.v3 parses bareword dates as time.Time. Normalize must reduce
	// to the wire shape (string).
	in := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: in}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Default; got != "2026-06-01" {
		t.Errorf("date default from time.Time: want %q, got %v", "2026-06-01", got)
	}
}

func TestNormalize_DateDefault_UnparseableClears(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: "tomorrow"}}}
	Normalize(tpl)
	if tpl.Fields[0].Default != nil {
		t.Errorf("garbage date: want nil, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_DateDefault_EmptyClears(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: "   "}}}
	Normalize(tpl)
	if tpl.Fields[0].Default != nil {
		t.Errorf("empty date: want nil, got %v", tpl.Fields[0].Default)
	}
}

func TestNormalize_DateDefault_IsIdempotent(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "d", Type: "date", Default: "21/07/2025"}}}
	Normalize(tpl)
	first := tpl.Fields[0].Default
	Normalize(tpl)
	if !reflect.DeepEqual(first, tpl.Fields[0].Default) {
		t.Errorf("second Normalize changed date: before=%v after=%v", first, tpl.Fields[0].Default)
	}
}

func TestNormalize_CoerceDefault_IsIdempotent(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "n", Type: "number", Default: "17"},
		{Key: "b", Type: "boolean", Default: "yes"},
		{Key: "t", Type: "tags", Default: "a,b"},
	}}
	Normalize(tpl)
	snapshot := []any{
		tpl.Fields[0].Default,
		tpl.Fields[1].Default,
		tpl.Fields[2].Default,
	}
	Normalize(tpl) // second pass should be a no-op
	got := []any{
		tpl.Fields[0].Default,
		tpl.Fields[1].Default,
		tpl.Fields[2].Default,
	}
	if !reflect.DeepEqual(snapshot, got) {
		t.Errorf("second Normalize changed values: before=%v after=%v", snapshot, got)
	}
}

func TestSaveTemplate_CoercesNumberDefault(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.EnsureTemplateDirectory(); err != nil {
		t.Fatalf("EnsureTemplateDirectory: %v", err)
	}
	tpl := &Template{
		Name: "T",
		Fields: []Field{
			{Key: "n", Type: "number", Default: "17"},
		},
	}
	if err := m.SaveTemplate("t.yaml", tpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	loaded, err := m.LoadTemplate("t.yaml")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	got := loaded.Fields[0].Default
	// On disk YAML round-trip, a numeric default deserializes as int
	// (yaml.v3) - both int and float64 are acceptable; both pass the
	// integrity analyzer's "expected number" check.
	switch g := got.(type) {
	case int:
		if g != 17 {
			t.Errorf("number default on disk: want 17, got %d", g)
		}
	case float64:
		if g != 17 {
			t.Errorf("number default on disk: want 17, got %v", g)
		}
	default:
		t.Errorf("number default on disk: want numeric, got %v (%T)", got, got)
	}
}
