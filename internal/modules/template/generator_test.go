package template

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────
// Generator
//
// Four shapes × two image modes:
//
//   - report:      frontmatter + per-field heading + value + debug logs
//   - minimal:     per-field heading + value, no frontmatter, no logs
//   - table:       single key/value Markdown table
//   - frontmatter: YAML data block only — image fields are SKIPPED
//
//   - imgURL:    `![Label]({{imageURL "key"}})` — slideout / wiki render
//   - imgInline: `![Label]({{imageBase64 "key"}})` — self-contained docs
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

// ─── Empty / unknown ──────────────────────────────────────────────────

func TestGenerate_EmptyFieldsAllShapes(t *testing.T) {
	for _, shape := range []Shape{ShapeReport, ShapeMinimal, ShapeTable, ShapeFrontmatter} {
		for _, mode := range []ImgMode{ImgURL, ImgInline} {
			got := GenerateMarkdownTemplate(shape, mode, nil)
			if got != "" {
				t.Errorf("shape=%q mode=%q empty fields: want \"\", got %q", shape, mode, got)
			}
		}
	}
}

func TestGenerate_UnknownShapeFallsBackToReport(t *testing.T) {
	got := GenerateMarkdownTemplate("does-not-exist", ImgURL, []Field{{Key: "x", Type: "text"}})
	if !strings.Contains(got, "title: Auto-generated Report") {
		t.Fatalf("unknown shape should fall back to report; got:\n%s", got)
	}
}

func TestGenerate_UnknownImgModeFallsBackToURL(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, "bogus", []Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `{{imageURL "cover"}}`) {
		t.Errorf("unknown imgMode should fall back to url; got:\n%s", got)
	}
}

// ─── Report shape ─────────────────────────────────────────────────────

func TestGenerate_ReportHasFrontmatterAndDebugLog(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL, sampleFields())

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
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL, []Field{{Key: "done", Type: "boolean", Label: "Done"}})
	if !strings.Contains(got, `{{#if (fieldRaw "done")}}`) {
		t.Errorf("boolean must render with if/else; got:\n%s", got)
	}
	if !strings.Contains(got, "✅ Done is checked") {
		t.Errorf("boolean should mention 'is checked'")
	}
	if !strings.Contains(got, "❌ Done is not checked") {
		t.Errorf("boolean should mention 'is not checked'")
	}
}

func TestGenerate_ReportImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageURL "cover"}})`) {
		t.Errorf("url mode: image must use {{imageURL}}; got:\n%s", got)
	}
	if strings.Contains(got, "imageBase64") {
		t.Errorf("url mode: must NOT contain imageBase64 helper")
	}
	if !strings.Contains(got, "_No image uploaded for Cover_") {
		t.Errorf("image must include empty-state branch")
	}
}

func TestGenerate_ReportImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, ImgInline,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageBase64 "cover"}})`) {
		t.Errorf("inline mode: image must use {{imageBase64}}; got:\n%s", got)
	}
	if strings.Contains(got, "imageURL") {
		t.Errorf("inline mode: must NOT contain imageURL helper")
	}
}

func TestGenerate_ReportListEachBlock(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL,
		[]Field{{Key: "items", Type: "list"}})
	if !strings.Contains(got, `{{#each (fieldRaw "items")}}`) {
		t.Errorf("list must use #each; got:\n%s", got)
	}
}

func TestGenerate_ReportTableHasHeaderAndRowExpansion(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL,
		[]Field{{Key: "rows", Type: "table"}})
	if !strings.Contains(got, `{{#with (fieldMeta "rows" "options") as |headers|}}`) {
		t.Errorf("table must read headers via fieldMeta; got:\n%s", got)
	}
	if !strings.Contains(got, `{{#each (fieldRaw "rows")}}`) {
		t.Errorf("table must iterate rows; got:\n%s", got)
	}
}

func TestGenerate_ReportLoopWrapsInnerFields(t *testing.T) {
	fields := []Field{
		{Key: "items", Type: "loopstart"},
		{Key: "name", Type: "text", Label: "Name"},
		{Key: "qty", Type: "number", Label: "Qty"},
		{Key: "items", Type: "loopstop"},
	}
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL, fields)

	if !strings.Contains(got, `{{#loop "items"}}`) {
		t.Errorf("loop start helper missing; got:\n%s", got)
	}
	if !strings.Contains(got, `{{/loop}}`) {
		t.Errorf("loop close missing; got:\n%s", got)
	}
	if !strings.Contains(got, `{{field "name"}}`) {
		t.Errorf("inner field 'name' not rendered inside loop; got:\n%s", got)
	}
	if !strings.Contains(got, `**items_index**: `) {
		t.Errorf("synthetic loop index field missing in inner debug logs; got:\n%s", got)
	}
}

func TestGenerate_ReportNestedLoops(t *testing.T) {
	fields := []Field{
		{Key: "outer", Type: "loopstart"},
		{Key: "inner", Type: "loopstart"},
		{Key: "leaf", Type: "text"},
		{Key: "inner", Type: "loopstop"},
		{Key: "outer", Type: "loopstop"},
	}
	got := GenerateMarkdownTemplate(ShapeReport, ImgURL, fields)

	outerOpen := strings.Index(got, `{{#loop "outer"}}`)
	innerOpen := strings.Index(got, `{{#loop "inner"}}`)
	innerClose := strings.Index(got, `{{/loop}}`)
	outerClose := strings.LastIndex(got, `{{/loop}}`)

	if outerOpen == -1 || innerOpen == -1 || innerClose == -1 || outerClose == -1 {
		t.Fatalf("nested loop tokens missing; got:\n%s", got)
	}
	if !(outerOpen < innerOpen && innerOpen < innerClose && innerClose < outerClose) {
		t.Errorf("nested loop tokens out of order; got:\n%s", got)
	}
}

// ─── Minimal shape ────────────────────────────────────────────────────

func TestGenerate_MinimalNoFrontmatterNoDebug(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, ImgURL, sampleFields())
	if strings.HasPrefix(got, "---") {
		t.Errorf("minimal must NOT start with frontmatter; got:\n%s", got)
	}
	if strings.Contains(got, "Debug: Remove this section") {
		t.Errorf("minimal must NOT include debug section")
	}
	if !strings.Contains(got, "## Title") {
		t.Errorf("minimal must include heading per field")
	}
	if !strings.Contains(got, `{{field "title"}}`) {
		t.Errorf("minimal must include the field reference")
	}
}

func TestGenerate_MinimalImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, ImgURL,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageURL "cover"}})`) {
		t.Errorf("minimal url-mode image: got:\n%s", got)
	}
}

func TestGenerate_MinimalImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, ImgInline,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `![Cover]({{imageBase64 "cover"}})`) {
		t.Errorf("minimal inline-mode image: got:\n%s", got)
	}
}

// ─── Table shape ──────────────────────────────────────────────────────

func TestGenerate_TableHeaderAndRowPerField(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, ImgURL, []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "done", Type: "boolean", Label: "Done"},
		{Key: "tags", Type: "tags", Label: "Tags"},
	})

	if !strings.Contains(got, "| Field | Value |") {
		t.Errorf("table missing header row; got:\n%s", got)
	}
	if !strings.Contains(got, "|-------|-------|") {
		t.Errorf("table missing separator row; got:\n%s", got)
	}
	if !strings.Contains(got, `| Title | {{field "title"}} |`) {
		t.Errorf("table missing 'Title' row; got:\n%s", got)
	}
	if !strings.Contains(got, `| Tags | {{tags (fieldRaw "tags")}} |`) {
		t.Errorf("table missing 'Tags' row using tags helper; got:\n%s", got)
	}
}

func TestGenerate_TableImageURLMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, ImgURL,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `| Cover | ![Cover]({{imageURL "cover"}}) |`) {
		t.Errorf("table url-mode image cell: got:\n%s", got)
	}
}

func TestGenerate_TableImageInlineMode(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, ImgInline,
		[]Field{{Key: "cover", Type: "image", Label: "Cover"}})
	if !strings.Contains(got, `| Cover | ![Cover]({{imageBase64 "cover"}}) |`) {
		t.Errorf("table inline-mode image cell: got:\n%s", got)
	}
}

func TestGenerate_TableSkipsLoopMarkers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, ImgURL, []Field{
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
	if strings.Contains(got, `| name |`) {
		t.Errorf("table must not surface inner-loop fields as top-level rows")
	}
}

// ─── Frontmatter shape ────────────────────────────────────────────────

func TestGenerate_FrontmatterOnly(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, ImgURL, []Field{
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
	if !strings.Contains(got, `done: {{json (fieldRaw "done")}}`) {
		t.Errorf("frontmatter must contain done key; got:\n%s", got)
	}
	if !strings.Contains(got, `tags: {{json (fieldRaw "tags")}}`) {
		t.Errorf("frontmatter must contain tags key; got:\n%s", got)
	}
	if strings.Contains(got, "##") {
		t.Errorf("frontmatter shape must not emit headings")
	}
}

func TestGenerate_FrontmatterSkipsImageFields(t *testing.T) {
	// Images don't fit a YAML metadata block — the user explicitly
	// chose "skip" for image fields in frontmatter shape, regardless
	// of imgMode (url or inline both omit them).
	for _, mode := range []ImgMode{ImgURL, ImgInline} {
		got := GenerateMarkdownTemplate(ShapeFrontmatter, mode, []Field{
			{Key: "title", Type: "text"},
			{Key: "cover", Type: "image"},
			{Key: "tags", Type: "tags"},
		})
		if strings.Contains(got, "cover") {
			t.Errorf("mode=%q frontmatter must skip image fields; got:\n%s", mode, got)
		}
		if !strings.Contains(got, "title:") || !strings.Contains(got, "tags:") {
			t.Errorf("mode=%q frontmatter must keep non-image fields; got:\n%s", mode, got)
		}
	}
}

func TestGenerate_FrontmatterSkipsLoopMarkers(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, ImgURL, []Field{
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
	if strings.Contains(got, "name:") {
		t.Errorf("frontmatter must not surface inner-loop fields as top-level keys")
	}
}

// ─── Catalogs ─────────────────────────────────────────────────────────

func TestShapes_ReturnsAllFour(t *testing.T) {
	shapes := Shapes()
	if len(shapes) != 4 {
		t.Fatalf("want 4 shapes, got %d", len(shapes))
	}
	seen := map[Shape]bool{}
	for _, s := range shapes {
		seen[s.ID] = true
		if s.Label == "" || s.Description == "" {
			t.Errorf("shape %q is missing label or description", s.ID)
		}
	}
	for _, want := range []Shape{ShapeReport, ShapeMinimal, ShapeTable, ShapeFrontmatter} {
		if !seen[want] {
			t.Errorf("Shapes() missing %q", want)
		}
	}
}

func TestImgModes_ReturnsBoth(t *testing.T) {
	modes := ImgModes()
	if len(modes) != 2 {
		t.Fatalf("want 2 image modes, got %d", len(modes))
	}
	seen := map[ImgMode]bool{}
	for _, m := range modes {
		seen[m.ID] = true
		if m.Label == "" || m.Description == "" {
			t.Errorf("mode %q is missing label or description", m.ID)
		}
	}
	for _, want := range []ImgMode{ImgURL, ImgInline} {
		if !seen[want] {
			t.Errorf("ImgModes() missing %q", want)
		}
	}
}
