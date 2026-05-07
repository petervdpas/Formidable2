package template

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────
// Generator
//
// GenerateMarkdownTemplate(shape, opts, fields) — opts carries the
// per-shape sub-choices:
//
//   - ImgMode:   url   → ![Label]({{imageURL "key"}})
//                inline → ![Label]({{imageBase64 "key"}})
//   - WrapLoops: true  → {{#loop "key"}}     (auto-wrapped iterations)
//                false → {{#loop "key" wrap=false}} (raw iterations)
//
// Frontmatter shape always SKIPS image fields (they don't fit YAML
// metadata) regardless of ImgMode.
// ─────────────────────────────────────────────────────────────────────

func sampleFields() []Field {
	return []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "done", Type: "boolean", Label: "Done"},
		{Key: "priority", Type: "dropdown", Label: "Priority"},
		{Key: "tags", Type: "tags", Label: "Tags"},
		{Key: "items", Type: "list", Label: "Items"},
		{Key: "rows", Type: "table", Label: "Rows"},
		{Key: "cover", Type: "image", Label: "Cover"},
		{Key: "flavors", Type: "multioption", Label: "Flavors"},
	}
}

// defaultOpts mirrors the frontend dialog's defaults: linked-URL image
// mode and auto-wrap for loop iterations.
func defaultOpts() GeneratorOptions {
	return GeneratorOptions{ImgMode: ImgURL, WrapLoops: true}
}

// ─── Empty / unknown ──────────────────────────────────────────────────

func TestGenerate_EmptyFieldsAllShapes(t *testing.T) {
	for _, shape := range []Shape{ShapeReport, ShapeMinimal, ShapeTable, ShapeFrontmatter} {
		for _, mode := range []ImgMode{ImgURL, ImgInline} {
			got := GenerateMarkdownTemplate(shape, GeneratorOptions{ImgMode: mode, WrapLoops: true}, nil)
			if got != "" {
				t.Errorf("shape=%q mode=%q empty fields: want \"\", got %q", shape, mode, got)
			}
		}
	}
}

func TestGenerate_UnknownShapeFallsBackToReport(t *testing.T) {
	got := GenerateMarkdownTemplate("does-not-exist", defaultOpts(), []Field{{Key: "x", Type: "text"}})
	if !strings.Contains(got, "title: Auto-generated Report") {
		t.Fatalf("unknown shape should fall back to report; got:\n%s", got)
	}
}

func TestGenerate_UnknownImgModeFallsBackToURL(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: "bogus", WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `{{imageURL "cover"}}`) {
		t.Errorf("unknown imgMode should fall back to url; got:\n%s", got)
	}
}

// ─── Report shape ─────────────────────────────────────────────────────

func TestGenerate_ReportHasFrontmatterAndDebugLog(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(), sampleFields())

	if !strings.HasPrefix(got, "---\ntitle: Auto-generated Report\n") {
		t.Errorf("report must start with frontmatter; got:\n%s", got)
	}
	if !strings.Contains(got, "toc: true") {
		t.Errorf("report frontmatter missing toc")
	}
	if !strings.Contains(got, `_{{fieldDescription "title"}}_`) {
		t.Errorf("report missing fieldDescription line for 'title'")
	}
	if !strings.Contains(got, "_Debug: Remove this section when your template is complete._") {
		t.Errorf("report missing debug section")
	}
	if !strings.Contains(got, "**title**: `{{json (fieldRaw \"title\")}}`") {
		t.Errorf("report missing debug log for 'title'")
	}
	if !strings.Contains(got, "**priority** _(options)_: `{{json (fieldMeta \"priority\" \"options\")}}`") {
		t.Errorf("report missing options-debug line for dropdown 'priority'")
	}
}

func TestGenerate_ReportBooleanUsesIfElse(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(),
		[]Field{{Key: "done", Type: "boolean", Label: "Done"}})
	if !strings.Contains(got, `{{#if (fieldRaw "done")}}`) {
		t.Errorf("boolean must render with if/else; got:\n%s", got)
	}
}

func TestGenerate_ReportImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageURL "cover"}})`) {
		t.Errorf("url mode: image must use {{imageURL}}; got:\n%s", got)
	}
	if strings.Contains(got, "imageBase64") {
		t.Errorf("url mode: must NOT contain imageBase64 helper")
	}
}

func TestGenerate_ReportImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: ImgInline, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageBase64 "cover"}})`) {
		t.Errorf("inline mode: image must use {{imageBase64}}; got:\n%s", got)
	}
	if strings.Contains(got, "imageURL") {
		t.Errorf("inline mode: must NOT contain imageURL helper")
	}
}

func TestGenerate_ReportListEachBlock(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(),
		[]Field{{Key: "items", Type: "list"}})
	if !strings.Contains(got, `{{#each (fieldRaw "items")}}`) {
		t.Errorf("list must use #each; got:\n%s", got)
	}
}

func TestGenerate_ReportTableHasHeaderAndRowExpansion(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(),
		[]Field{{Key: "rows", Type: "table"}})
	if !strings.Contains(got, `{{#with (fieldMeta "rows" "options") as |headers|}}`) {
		t.Errorf("table must read headers via fieldMeta; got:\n%s", got)
	}
}

func TestGenerate_ReportLoopOpenerStaysBare(t *testing.T) {
	// {{#loop "key"}} is a plain iterator — wrap state is signalled
	// by the presence/absence of {{loopItemBefore}} inside the body,
	// not by hash options on the opener.
	fields := []Field{
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text", Label: "Name"},
		{Key: "items", Type: "loopstop"},
	}
	for _, wrap := range []bool{true, false} {
		got := GenerateMarkdownTemplate(ShapeReport,
			GeneratorOptions{ImgMode: ImgURL, WrapLoops: wrap}, fields)
		if !strings.Contains(got, `{{#loop "items"}}`) {
			t.Errorf("wrap=%v: loop opener should be bare; got:\n%s", wrap, got)
		}
		if !strings.Contains(got, `{{/loop}}`) {
			t.Errorf("wrap=%v: loop close missing; got:\n%s", wrap, got)
		}
	}
}

func TestGenerate_ReportWrapLoopsTrueEmitsBeforeAfterHelpers(t *testing.T) {
	fields := []Field{
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text", Label: "Name"},
		{Key: "items", Type: "loopstop"},
	}
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true}, fields)

	if !strings.Contains(got, `{{loopItemBefore}}`) {
		t.Errorf("wrap=true: missing {{loopItemBefore}}; got:\n%s", got)
	}
	if !strings.Contains(got, `{{loopItemAfter}}`) {
		t.Errorf("wrap=true: missing {{loopItemAfter}}; got:\n%s", got)
	}
}

func TestGenerate_ReportWrapLoopsFalseOmitsBeforeAfterHelpers(t *testing.T) {
	fields := []Field{
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text", Label: "Name"},
		{Key: "items", Type: "loopstop"},
	}
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: false}, fields)

	if strings.Contains(got, `loopItemBefore`) {
		t.Errorf("wrap=false: must NOT contain loopItemBefore; got:\n%s", got)
	}
	if strings.Contains(got, `loopItemAfter`) {
		t.Errorf("wrap=false: must NOT contain loopItemAfter; got:\n%s", got)
	}
}

func TestGenerate_ReportNestedLoopsBothWrapped(t *testing.T) {
	fields := []Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "inner", Type: "loopstart"},
		{Key: "leaf", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "outer", Type: "loopstop"},
	}
	got := GenerateMarkdownTemplate(ShapeReport,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true}, fields)

	// Both nested loops should have a before+after pair; verify by
	// counting occurrences (loose check — at least 2 of each).
	if strings.Count(got, `{{loopItemBefore}}`) < 2 {
		t.Errorf("nested wrap=true: expected at least 2 loopItemBefore; got:\n%s", got)
	}
	if strings.Count(got, `{{loopItemAfter}}`) < 2 {
		t.Errorf("nested wrap=true: expected at least 2 loopItemAfter; got:\n%s", got)
	}
}

// ─── Minimal shape ────────────────────────────────────────────────────

func TestGenerate_MinimalNoFrontmatterNoDebug(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, defaultOpts(), sampleFields())
	if strings.HasPrefix(got, "---") {
		t.Errorf("minimal must NOT start with frontmatter; got:\n%s", got)
	}
	if strings.Contains(got, "Debug: Remove this section") {
		t.Errorf("minimal must NOT include debug section")
	}
}

func TestGenerate_MinimalImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageURL "cover"}})`) {
		t.Errorf("minimal url-mode image: got:\n%s", got)
	}
}

func TestGenerate_MinimalImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal,
		GeneratorOptions{ImgMode: ImgInline, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageBase64 "cover"}})`) {
		t.Errorf("minimal inline-mode image: got:\n%s", got)
	}
}

func TestGenerate_MinimalWrapLoopsTrueEmitsHelpers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true},
		[]Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		})
	if !strings.Contains(got, `{{loopItemBefore}}`) {
		t.Errorf("minimal wrap=true: missing loopItemBefore; got:\n%s", got)
	}
	if !strings.Contains(got, `{{loopItemAfter}}`) {
		t.Errorf("minimal wrap=true: missing loopItemAfter; got:\n%s", got)
	}
}

func TestGenerate_MinimalWrapLoopsFalseOmitsHelpers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: false},
		[]Field{
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		})
	if strings.Contains(got, "loopItemBefore") || strings.Contains(got, "loopItemAfter") {
		t.Errorf("minimal wrap=false: must omit before/after helpers; got:\n%s", got)
	}
}

// ─── Table shape ──────────────────────────────────────────────────────

func TestGenerate_TableHeaderAndRowPerField(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, defaultOpts(), []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "done", Type: "boolean", Label: "Done"},
		{Key: "tags", Type: "tags", Label: "Tags"},
	})
	if !strings.Contains(got, "| Field | Value |") {
		t.Errorf("table missing header row; got:\n%s", got)
	}
	if !strings.Contains(got, `| Title | {{field "title"}} |`) {
		t.Errorf("table missing 'Title' row; got:\n%s", got)
	}
}

func TestGenerate_TableImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable,
		GeneratorOptions{ImgMode: ImgURL, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `| Cover | ![Cover]({{imageURL "cover"}}) |`) {
		t.Errorf("table url-mode image cell: got:\n%s", got)
	}
}

func TestGenerate_TableImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable,
		GeneratorOptions{ImgMode: ImgInline, WrapLoops: true},
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `| Cover | ![Cover]({{imageBase64 "cover"}}) |`) {
		t.Errorf("table inline-mode image cell: got:\n%s", got)
	}
}

func TestGenerate_TableSkipsLoopMarkers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, defaultOpts(), []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text"},
		{Key: "items", Type: "loopstop"},
	})
	if strings.Contains(got, "loopstart") || strings.Contains(got, "loopstop") {
		t.Errorf("table must not surface raw loop markers")
	}
	if !strings.Contains(got, `| items |`) {
		t.Errorf("table should still surface a row for the loop key; got:\n%s", got)
	}
}

// ─── Frontmatter shape ────────────────────────────────────────────────

func TestGenerate_FrontmatterOnly(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, defaultOpts(), []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "done", Type: "boolean"},
		{Key: "tags", Type: "tags"},
	})
	if !strings.HasPrefix(got, "---\n") || !strings.HasSuffix(strings.TrimRight(got, "\n"), "---") {
		t.Errorf("frontmatter shape must be wrapped in --- markers; got:\n%s", got)
	}
	if !strings.Contains(got, `title: {{json (fieldRaw "title")}}`) {
		t.Errorf("frontmatter must contain title key; got:\n%s", got)
	}
}

func TestGenerate_FrontmatterSkipsImageFields(t *testing.T) {
	for _, mode := range []ImgMode{ImgURL, ImgInline} {
		got := GenerateMarkdownTemplate(ShapeFrontmatter,
			GeneratorOptions{ImgMode: mode, WrapLoops: true},
			[]Field{
				{Key: "title", Type: "text"},
				{Key: "cover", Type: "image"},
				{Key: "tags", Type: "tags"},
			})
		if strings.Contains(got, "cover") {
			t.Errorf("mode=%q frontmatter must skip image fields; got:\n%s", mode, got)
		}
	}
}

func TestGenerate_FrontmatterSkipsLoopMarkers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, defaultOpts(), []Field{
		{Key: "title", Type: "text"},
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text"},
		{Key: "items", Type: "loopstop"},
	})
	if strings.Contains(got, "loopstart") || strings.Contains(got, "loopstop") {
		t.Errorf("frontmatter must not surface raw loop markers")
	}
	if !strings.Contains(got, `items: {{json (fieldRaw "items")}}`) {
		t.Errorf("frontmatter should surface the loop key as a single value; got:\n%s", got)
	}
}

// ─── API field ────────────────────────────────────────────────────────
//
// API-typed fields stamp `{guid, ...projected_columns}` into host data
// at picker time; the generator emits {{apiSection}} for shapes that
// expand fields into prose (Report/Minimal), the standard JSON dump
// for the Table shape (one row per host field), and skips the field
// for Frontmatter (it doesn't fit a YAML metadata block).

func TestGenerate_ReportEmitsAPISectionHelper(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(), []Field{
		{Key: "ref", Type: "api", Label: "Reference",
			Collection: "addresses.yaml",
			Map: []APIMap{{Key: "name", Label: "Naam"}}},
	})
	if !strings.Contains(got, `{{apiSection "ref"}}`) {
		t.Errorf("report must emit apiSection for api field; got:\n%s", got)
	}
	// And NOT the generic {{field "ref"}} fallback (which would render
	// the whole {guid, ...} object as JSON in prose).
	if strings.Contains(got, `{{field "ref"}}`) {
		t.Errorf("report must not fall back to generic field helper; got:\n%s", got)
	}
}

func TestGenerate_MinimalEmitsAPISectionHelper(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, defaultOpts(), []Field{
		{Key: "ref", Type: "api", Label: "Reference"},
	})
	if !strings.Contains(got, `{{apiSection "ref"}}`) {
		t.Errorf("minimal must emit apiSection for api field; got:\n%s", got)
	}
}

func TestGenerate_FrontmatterSkipsAPIFields(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, defaultOpts(), []Field{
		{Key: "title", Type: "text"},
		{Key: "ref", Type: "api"},
		{Key: "tags", Type: "tags"},
	})
	if strings.Contains(got, "ref") {
		t.Errorf("frontmatter must skip api fields; got:\n%s", got)
	}
	if !strings.Contains(got, "title:") || !strings.Contains(got, "tags:") {
		t.Errorf("other fields must still surface; got:\n%s", got)
	}
}

func TestGenerate_TableShapeKeepsJSONForAPI(t *testing.T) {
	// Table shape already had a {{json (fieldRaw "k")}} branch for
	// "list", "multioption", "table", "api" — confirm api is still
	// covered after the refactor.
	got := GenerateMarkdownTemplate(ShapeTable, defaultOpts(), []Field{
		{Key: "ref", Type: "api", Label: "Reference"},
	})
	if !strings.Contains(got, `{{json (fieldRaw "ref")}}`) {
		t.Errorf("table must dump api as JSON; got:\n%s", got)
	}
}

// ─── Catalogs ─────────────────────────────────────────────────────────

func TestShapes_ReturnsAllFour(t *testing.T) {
	shapes := Shapes()
	if len(shapes) != 4 {
		t.Fatalf("want 4 shapes, got %d", len(shapes))
	}
}
