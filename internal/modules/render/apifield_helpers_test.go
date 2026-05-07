package render

import (
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─────────────────────────────────────────────────────────────────────
// Pure helpers — flatten/format functions. No raymond involvement so
// they're easy to test without spinning up a full render pipeline.
// ─────────────────────────────────────────────────────────────────────

func TestScalarOrJSON_Scalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"empty-string", "", ""},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool-true", true, "true"},
		{"bool-false", false, "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scalarOrJSON(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestScalarOrJSON_NonScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"slice", []any{"a", "b"}, `["a","b"]`},
		{"map", map[string]any{"k": "v"}, `{"k":"v"}`},
		{"nested", []any{map[string]any{"a": 1}}, `[{"a":1}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scalarOrJSON(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestEmitAPIColumnBlock_TagsCommaJoined(t *testing.T) {
	src := &template.Field{Type: "tags"}
	got := emitAPIColumnBlock([]any{"a", "b", "c"}, src)
	if got != "a, b, c" {
		t.Errorf("got %q, want %q", got, "a, b, c")
	}
}

func TestEmitAPIColumnBlock_ListBullets(t *testing.T) {
	src := &template.Field{Type: "list"}
	got := emitAPIColumnBlock([]any{"first", "second"}, src)
	want := "- first\n- second"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEmitAPIColumnBlock_ListEmptyIsBlank(t *testing.T) {
	src := &template.Field{Type: "list"}
	if got := emitAPIColumnBlock([]any{}, src); got != "" {
		t.Errorf("empty list: got %q, want \"\"", got)
	}
}

func TestEmitAPIColumnBlock_TableArrayOfArraysWithHeaders(t *testing.T) {
	// Source's options[] declares column metadata — the renderer
	// reads .label for the markdown table header row.
	src := &template.Field{
		Type: "table",
		Options: []any{
			map[string]any{"value": "firstname", "label": "Firstname"},
			map[string]any{"value": "lastname", "label": "Lastname"},
		},
	}
	rows := []any{
		[]any{"John", "Lennon"},
		[]any{"Paul", "McCartney"},
	}
	got := emitAPIColumnBlock(rows, src)
	want := "" +
		"| Firstname | Lastname |\n" +
		"| --- | --- |\n" +
		"| John | Lennon |\n" +
		"| Paul | McCartney |"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEmitAPIColumnBlock_TablePadsShortRows(t *testing.T) {
	src := &template.Field{
		Type: "table",
		Options: []any{
			map[string]any{"value": "a", "label": "A"},
			map[string]any{"value": "b", "label": "B"},
			map[string]any{"value": "c", "label": "C"},
		},
	}
	got := emitAPIColumnBlock([]any{[]any{"x", "y"}}, src)
	if !strings.Contains(got, "| x | y |  |") {
		t.Errorf("expected padded row; got:\n%s", got)
	}
}

func TestEmitAPIColumnBlock_NilSourceFallsBackToJSON(t *testing.T) {
	// When the loader can't resolve the source, we still return
	// something useful — the JSON form of the value.
	got := emitAPIColumnBlock(map[string]any{"k": "v"}, nil)
	if got != `{"k":"v"}` {
		t.Errorf("got %q, want JSON", got)
	}
}

func TestEmitAPIColumnBlock_ScalarSourceUsesScalarOrJSON(t *testing.T) {
	src := &template.Field{Type: "text"}
	if got := emitAPIColumnBlock("hello", src); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEmitAPISection_HeaderAndRows(t *testing.T) {
	host := &template.Field{
		Key:        "testapi",
		Label:      "Testapi",
		Type:       "api",
		Collection: "addresses.yaml",
		Map: []template.APIMap{
			{Key: "name", Label: "NameAlias"},
			{Key: "street", Label: "StreetAlias"},
			{Key: "owners", Label: "OwnersAlias"},
		},
	}
	row := map[string]any{
		"guid":   "g-1",
		"name":   "Buckingham Palace",
		"street": "Buckingham Palace Road",
		"owners": []any{[]any{"Charles", "Windsor"}},
	}
	// Source resolver knows just one of the three columns is non-scalar.
	src := &template.Template{
		Filename: "addresses.yaml",
		Fields: []template.Field{
			{Key: "name", Type: "text", Label: "Name"},
			{Key: "street", Type: "text", Label: "Street"},
			{Key: "owners", Type: "table", Label: "Owners",
				Options: []any{
					map[string]any{"value": "firstname", "label": "Firstname"},
					map[string]any{"value": "lastname", "label": "Lastname"},
				},
			},
		},
	}
	opts := &Options{
		LoadTemplate: func(name string) *template.Template {
			if name == "addresses.yaml" {
				return src
			}
			return nil
		},
	}

	got := emitAPISection(row, host, opts)

	// Wrapper — opens with <section class="api-card" data-source="..."> and
	// closes with </section>. Blank lines around let goldmark fall back to
	// "type 6" HTML block, so inner markdown still parses.
	if !strings.HasPrefix(got, `<section class="api-card" data-source="addresses.yaml">`) {
		t.Errorf("missing card wrapper opener; got:\n%s", got)
	}
	if !strings.HasSuffix(got, "</section>") {
		t.Errorf("missing card wrapper closer; got:\n%s", got)
	}
	// Header
	if !strings.Contains(got, "**Testapi** _(addresses.yaml)_") {
		t.Errorf("missing header; got:\n%s", got)
	}
	// Inline rows for scalars (Map.Label takes precedence over source label)
	if !strings.Contains(got, "- **NameAlias**: Buckingham Palace") {
		t.Errorf("missing scalar inline row; got:\n%s", got)
	}
	if !strings.Contains(got, "- **StreetAlias**: Buckingham Palace Road") {
		t.Errorf("missing scalar inline row; got:\n%s", got)
	}
	// Block row for table — header on its own line, then markdown table block
	if !strings.Contains(got, "- **OwnersAlias**:") {
		t.Errorf("missing table-column header; got:\n%s", got)
	}
	if !strings.Contains(got, "| Firstname | Lastname |") {
		t.Errorf("missing markdown-table header; got:\n%s", got)
	}
	if !strings.Contains(got, "| Charles | Windsor |") {
		t.Errorf("missing markdown-table body row; got:\n%s", got)
	}
}

func TestEmitAPISection_FallsBackToFieldKeyWhenLabelEmpty(t *testing.T) {
	host := &template.Field{
		Key:        "testapi",
		Type:       "api",
		Collection: "addr.yaml",
		Map:        []template.APIMap{{Key: "name"}}, // no Label → fall back to Key
	}
	row := map[string]any{"name": "Alice"}
	got := emitAPISection(row, host, nil)
	// Host header falls back to Key when Label is empty.
	if !strings.Contains(got, "**testapi**") {
		t.Errorf("expected host fallback to key; got:\n%s", got)
	}
	// Column label falls back to Key.
	if !strings.Contains(got, "- **name**: Alice") {
		t.Errorf("expected column fallback to key; got:\n%s", got)
	}
}

func TestEmitAPISection_NilHostFieldReturnsEmpty(t *testing.T) {
	if got := emitAPISection(nil, nil, nil); got != "" {
		t.Errorf("nil hostField should return empty; got %q", got)
	}
}

func TestLoadSourceField_NilOptions(t *testing.T) {
	if loadSourceField(nil, &template.Field{Collection: "x.yaml"}, "k") != nil {
		t.Error("nil opts should return nil")
	}
}

func TestLoadSourceField_NilHostField(t *testing.T) {
	opts := &Options{LoadTemplate: func(string) *template.Template { return nil }}
	if loadSourceField(opts, nil, "k") != nil {
		t.Error("nil hostField should return nil")
	}
}

func TestLoadSourceField_HostMissingCollection(t *testing.T) {
	opts := &Options{LoadTemplate: func(string) *template.Template { return nil }}
	if loadSourceField(opts, &template.Field{}, "k") != nil {
		t.Error("empty Collection should return nil")
	}
}

func TestLoadSourceField_LoaderReturnsNil(t *testing.T) {
	opts := &Options{LoadTemplate: func(string) *template.Template { return nil }}
	host := &template.Field{Collection: "x.yaml"}
	if loadSourceField(opts, host, "k") != nil {
		t.Error("loader miss should return nil")
	}
}

func TestLoadSourceField_KeyNotInSource(t *testing.T) {
	opts := &Options{LoadTemplate: func(string) *template.Template {
		return &template.Template{Fields: []template.Field{{Key: "other", Type: "text"}}}
	}}
	host := &template.Field{Collection: "x.yaml"}
	if loadSourceField(opts, host, "missing") != nil {
		t.Error("key not in source should return nil")
	}
}

func TestLoadSourceField_FoundResolvesField(t *testing.T) {
	opts := &Options{LoadTemplate: func(string) *template.Template {
		return &template.Template{Fields: []template.Field{
			{Key: "name", Type: "text"},
			{Key: "owners", Type: "table"},
		}}
	}}
	host := &template.Field{Collection: "x.yaml"}
	got := loadSourceField(opts, host, "owners")
	if got == nil || got.Type != "table" {
		t.Errorf("got %+v, want table-typed field", got)
	}
}
