package template

import (
	"strings"
	"testing"
)

// Generator dispatch for virtual (data-less) field types. The first
// virtual type is "facet"; its value lives in meta.facets[FacetKey]
// not in form.data, so the generator must emit {{virtual-field "key"}}
// (the render-side helper resolves the projection) instead of the
// default {{field "key"}}, which would always render empty.
//
// Kept separate from generator_test.go so all virtual concerns stay
// grouped — same pattern as the rest of the virtual-field tests.

func facetFieldsSample() []Field {
	return []Field{
		{Key: "title", Type: "text", Label: "Title"},
		{Key: "status_inline", Type: "facet", FacetKey: "status", Label: "Status", Format: "radio"},
	}
}

// ── Report shape ─────────────────────────────────────────────────────

func TestGenerate_ReportFacetEmitsVirtualFieldHelper(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(), facetFieldsSample())
	if !strings.Contains(got, `{{virtual-field "status_inline"}}`) {
		t.Errorf("expected virtual-field helper for facet; got:\n%s", got)
	}
	if strings.Contains(got, `{{field "status_inline"}}`) {
		t.Errorf("facet must NOT fall through to {{field}}; got:\n%s", got)
	}
}

// Debug log for a facet field would log fieldRaw which is always null
// for a virtual field. The generator should either skip the log or
// emit one based on {{virtual-field}}. We pick "skip" - virtual fields
// have no raw data to surface for debugging.
func TestGenerate_ReportFacetSkipsDebugLog(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeReport, defaultOpts(), facetFieldsSample())
	if strings.Contains(got, `(fieldRaw "status_inline")`) {
		t.Errorf("facet field has no data slot - fieldRaw log must be omitted; got:\n%s", got)
	}
}

// ── Minimal shape ────────────────────────────────────────────────────

func TestGenerate_MinimalFacetEmitsVirtualFieldHelper(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeMinimal, defaultOpts(), facetFieldsSample())
	if !strings.Contains(got, `{{virtual-field "status_inline"}}`) {
		t.Errorf("expected virtual-field helper for facet in minimal shape; got:\n%s", got)
	}
}

// ── Table shape ──────────────────────────────────────────────────────

func TestGenerate_TableFacetUsesVirtualFieldHelper(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeTable, defaultOpts(), facetFieldsSample())
	if !strings.Contains(got, `{{virtual-field "status_inline"}}`) {
		t.Errorf("table row for facet must use virtual-field; got:\n%s", got)
	}
	if strings.Contains(got, `{{field "status_inline"}}`) {
		t.Errorf("facet row must NOT fall through to {{field}}; got:\n%s", got)
	}
}

// ── Frontmatter shape ────────────────────────────────────────────────

// Facets are small string labels and DO fit a YAML metadata block, so
// the frontmatter shape includes them - unlike image/api which produce
// binary or denormalised-object shapes that don't.
func TestGenerate_FrontmatterFacetIncluded(t *testing.T) {
	got := GenerateMarkdownTemplate(ShapeFrontmatter, defaultOpts(), facetFieldsSample())
	if !strings.Contains(got, `status_inline:`) {
		t.Errorf("frontmatter must emit a key for the facet field; got:\n%s", got)
	}
	if !strings.Contains(got, `{{virtual-field "status_inline"}}`) {
		t.Errorf("frontmatter value must come from virtual-field helper; got:\n%s", got)
	}
	if strings.Contains(got, `(fieldRaw "status_inline")`) {
		t.Errorf("frontmatter must NOT read fieldRaw for a virtual field; got:\n%s", got)
	}
}
