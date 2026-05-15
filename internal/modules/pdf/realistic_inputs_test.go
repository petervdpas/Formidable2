package pdf

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	picoloom "github.com/alnah/picoloom/v2"
)

// auditControlsRenderedMD is what the renderer hands to ParseFrontmatter
// for the user's audit-controls.yaml template AFTER raymond expansion.
// Frozen here as a literal so the test isn't coupled to the on-disk
// template file. Mirrors the real fixture from
// ~/Documents/Example/templates/audit-controls.yaml with Handlebars
// already substituted.
const auditControlsRenderedMD = `---
title: Audit Control
subtitle: BC.01 - Beheersmaatregel toetsen
author: Team Integration Services
keywords:
  - Governance
  - BC.01
titlepage-color: F8F8F8
titlepage-logo: C:/Projects/team/data/fontys-logo.png
toc: true
toc-title: Inhoudsopgave
---


## BC.01 - Beheersmaatregel toetsen

Dit is de beschrijving van de audit-control *BC.01 - Beheersmaatregel toetsen*.
`

// ---------- ParseFrontmatter realistic inputs ----------

func TestParseFrontmatter_RealAuditControlsRenderedOutput(t *testing.T) {
	// Audit-controls rendered output uses eisvogel shape, including
	// `toc: true` (boolean). Picoloom's TOCFM is a structured block,
	// so yaml.Unmarshal returns ErrFrontmatterMalformed. The contract
	// is that ParseFrontmatter REPORTS this (render.go logs a warn
	// and continues with zero FM) but the body still survives. The
	// fail path being graceful is what stops a user with a not-yet-
	// migrated template from getting a hard render error.
	fm, body, err := ParseFrontmatter(auditControlsRenderedMD)
	if !errors.Is(err, ErrFrontmatterMalformed) {
		t.Errorf("err = %v, want ErrFrontmatterMalformed (eisvogel toc:true mismatches picoloom TOCFM)", err)
	}
	// Body survives the malformed-FM path (per the contract: render
	// proceeds on best-effort with the verbatim source).
	if !strings.Contains(body, "## BC.01 - Beheersmaatregel toetsen") {
		t.Errorf("body content lost on malformed-FM path:\n%s", body)
	}
	if fm.Style != "" {
		t.Errorf("Style = %q, want empty (zero FM on malformed)", fm.Style)
	}
}

func TestParseFrontmatter_BodyContainsCloseFenceLikeLine(t *testing.T) {
	// A literal `---` line inside the body must not confuse the
	// fence detector. The regex anchors `^---$` to a line start, so
	// the FIRST `^---$` after the open is the close — anything past
	// it is body, even if it has more `---` lines.
	src := `---
title: First Fence
---

# Body heading

---

End of body.
`
	fm, body, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	// The typed struct doesn't know the unknown key 'title', so
	// confirm the body retains both the H1 and the in-body `---`.
	_ = fm
	if !strings.Contains(body, "# Body heading") || !strings.Contains(body, "End of body.") {
		t.Errorf("body content lost or truncated:\n%s", body)
	}
	if !strings.Contains(body, "\n---\n") {
		t.Errorf("body should contain an in-body `---` line:\n%s", body)
	}
}

func TestParseFrontmatter_BodyContainsLiteralHandlebars(t *testing.T) {
	// A documentation template might show literal Handlebars syntax
	// in the body (e.g. teaching `{{field "x"}}`). The body is treated
	// as opaque markdown — Handlebars in the body never gets YAML-
	// parsed, so this should be a no-op safety case.
	src := "---\nstyle: technical\n---\n\nUse `{{field \"x\"}}` to insert a field.\n"
	fm, body, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Style != "technical" {
		t.Errorf("Style = %q, want technical", fm.Style)
	}
	if !strings.Contains(body, `{{field "x"}}`) {
		t.Errorf("Handlebars-in-body lost: %q", body)
	}
}

func TestParseFrontmatter_LongFrontmatter(t *testing.T) {
	// A wide frontmatter with every cover field populated + a long
	// description. Smoke-test that nothing in the regex or yaml
	// pass scales poorly.
	keys := []string{
		"title", "subtitle", "logo", "author", "authorTitle",
		"organization", "date", "version", "clientName", "projectName",
		"documentType", "documentID", "description", "department",
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("style: technical\n")
	b.WriteString("cover:\n")
	for _, k := range keys {
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString(": ")
		for j := 0; j < 50; j++ {
			b.WriteString("xx ")
		}
		b.WriteString("\n")
	}
	b.WriteString("---\nbody\n")
	if _, _, err := ParseFrontmatter(b.String()); err != nil {
		t.Errorf("long frontmatter parse failed: %v", err)
	}
}

func TestParseFrontmatter_EmptyBodyAfterClose(t *testing.T) {
	src := "---\nstyle: technical\n---\n"
	fm, body, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm.Style != "technical" {
		t.Errorf("Style lost when body is empty")
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

// ---------- Manager.Export realistic-input pipeline ----------

func TestExport_RealisticRenderedFrontmatter_TolerantWhenEisvogel(t *testing.T) {
	// End-to-end smoke: feed the audit-controls-style RENDERED
	// markdown (eisvogel shape — `toc: true`, etc.) through
	// Manager.Export. The frontmatter parse FAILS (typed schema
	// mismatch), but render.go must continue with empty FM and still
	// hand the body to picoloom. This is the user-friendly "your
	// frontmatter isn't migrated yet, but we'll still produce a PDF"
	// path.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["audit.yaml"] = "/storage/audit"
	rdr.md["audit.yaml|bc01.meta.json"] = auditControlsRenderedMD

	res, err := m.Export("audit.yaml", "bc01.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export real input: %v", err)
	}
	if res.Bytes <= 0 {
		t.Errorf("result.Bytes = %d", res.Bytes)
	}
	if cf.last == nil {
		t.Fatalf("converter not invoked")
	}
	// Body fed to picoloom must include the rendered H2 heading.
	if !strings.Contains(cf.last.seen.Markdown, "## BC.01 - Beheersmaatregel toetsen") {
		t.Errorf("converter markdown lost the heading:\n%s", cf.last.seen.Markdown)
	}
	// SourceDir defaults to the template storage dir so relative-path
	// images resolve.
	if cf.last.seen.SourceDir != "/storage/audit" {
		t.Errorf("SourceDir = %q, want /storage/audit", cf.last.seen.SourceDir)
	}
	// Cover stays nil since the typed FM never populated.
	if cf.last.seen.Cover != nil {
		t.Errorf("Cover should be nil on malformed-FM path; got %+v", cf.last.seen.Cover)
	}
}

func TestExport_RealisticRenderedFrontmatter_MigratedShape(t *testing.T) {
	// Same audit-controls source but POST-migrate: picoloom-shape
	// cover block populates the typed FM, the cover sub-block lands
	// in picoloom.Input.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["audit.yaml"] = "/storage/audit"
	rdr.md["audit.yaml|bc01.meta.json"] = `---
style: technical
cover:
  enabled: true
  template: classic
  title: Audit Control
  subtitle: BC.01 - Beheersmaatregel toetsen
  author: Team Integration Services
---


## BC.01
`
	res, err := m.Export("audit.yaml", "bc01.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export migrated input: %v", err)
	}
	if res.Bytes <= 0 {
		t.Errorf("result.Bytes = %d", res.Bytes)
	}
	if cf.last == nil {
		t.Fatalf("converter not invoked")
	}
	if cf.last.seen.Cover == nil {
		t.Fatalf("Cover nil after migrated shape")
	}
	if cf.last.seen.Cover.Title != "Audit Control" {
		t.Errorf("Cover.Title = %q, want Audit Control", cf.last.seen.Cover.Title)
	}
	if cf.style != "technical" {
		t.Errorf("converter style = %q, want technical", cf.style)
	}
}

func TestExport_UnknownCoverTemplate_SurfacesTypedError(t *testing.T) {
	// User picks a cover that's not on disk (renamed, deleted, typo).
	// The error must be the typed CodeCoverTemplateInvalid so the
	// frontend toast knows what to say.
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["audit.yaml"] = "/storage/audit"
	rdr.md["audit.yaml|bc01.meta.json"] = "---\nstyle: technical\n---\n# body\n"

	_, err := m.Export("audit.yaml", "bc01.meta.json", ExportOpts{
		CoverTemplate: "does-not-exist-on-disk",
	})
	if err == nil {
		t.Fatalf("expected error for missing cover")
	}
	var ee *ExportError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *ExportError, got %T %v", err, err)
	}
	if ee.Code != CodeCoverTemplateInvalid {
		t.Errorf("Code = %q, want %q", ee.Code, CodeCoverTemplateInvalid)
	}
	// Stage is captured via telemetry, not on ExportError; assert
	// via LastExport().LastFailure.Stage instead.
	failure := m.LastExport().LastFailure
	if failure == nil {
		t.Fatalf("expected LastFailure telemetry")
	}
	if failure.Stage != "resolve_cover" {
		t.Errorf("failure stage = %q, want resolve_cover", failure.Stage)
	}
}

func TestExport_PicoloomConvertError_PropagatesToCodeViaConvert(t *testing.T) {
	// Different from the existing "convert error" happy-bad-path
	// test — this one uses a real picoloom sentinel (ErrCoverRender)
	// so the code-mapper path is exercised, not just the wrap.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["t.yaml"] = "/s"
	rdr.md["t.yaml|x.meta.json"] = "---\nstyle: technical\n---\n# body\n"
	cf.convertOverride = func(_ context.Context, _ picoloom.Input) (*picoloom.ConvertResult, error) {
		return nil, picoloom.ErrCoverRender
	}

	_, err := m.Export("t.yaml", "x.meta.json", ExportOpts{})
	var ee *ExportError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExportError, got %v", err)
	}
	if ee.Code != CodeCoverTemplateInvalid {
		t.Errorf("Code = %q, want %q", ee.Code, CodeCoverTemplateInvalid)
	}
}

func TestExport_RealisticBodySize(t *testing.T) {
	// Stress: render produces a multi-KB body (think a populated audit
	// control with all maturity levels). Pipeline shouldn't fall over.
	var body strings.Builder
	body.WriteString("---\nstyle: technical\n---\n")
	for i := 0; i < 200; i++ {
		body.WriteString("## Section\n\nLorem ipsum dolor sit amet. ")
		body.WriteString("Consectetur adipiscing elit. ")
		body.WriteString("Sed do eiusmod tempor incididunt.\n\n")
	}
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["t.yaml"] = "/s"
	rdr.md["t.yaml|big.meta.json"] = body.String()

	started := time.Now()
	res, err := m.Export("t.yaml", "big.meta.json", ExportOpts{})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("Export big: %v", err)
	}
	if res.Bytes <= 0 {
		t.Errorf("Bytes = %d", res.Bytes)
	}
	// Loose ceiling — race-detector mode plus 200 sections shouldn't
	// take more than a few seconds. Catches accidental quadratic
	// pipeline introductions.
	if elapsed > 5*time.Second {
		t.Errorf("Export of 200-section body took %v; perf regression?", elapsed)
	}
}

// ---------- ResolveExportDefaults realistic inputs ----------

func TestResolveExportDefaults_EisvogelStyleSourceShowsNoPicoloomDefaults(t *testing.T) {
	// User opens the dialog on an audit-controls-shape template.
	// Frontmatter has eisvogel keys, NOT picoloom shape — so resolver
	// reports no Theme and no CoverTemplate. The dialog can then label
	// the dropdown defaults as "no theme — picoloom built-in".
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["audit.yaml"] = "/storage/audit"
	rdr.md["audit.yaml|bc01.meta.json"] = auditControlsRenderedMD

	got, err := m.ResolveExportDefaults("audit.yaml", "bc01.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "" {
		t.Errorf("Theme = %q, want empty (eisvogel source has no picoloom style)", got.Theme)
	}
	if got.CoverTemplate != "" {
		t.Errorf("CoverTemplate = %q, want empty", got.CoverTemplate)
	}
	if got.CoverDisabled {
		t.Errorf("CoverDisabled = true, want false (cover.enabled never appears in eisvogel)")
	}
}

func TestResolveExportDefaults_HandlebarsInRenderedOutput(t *testing.T) {
	// Defensive: the renderer is SUPPOSED to expand Handlebars
	// before the output hits ParseFrontmatter. But if a malformed
	// expression slipped through unrendered, the resolver shouldn't
	// crash — ParseFrontmatter would return malformed and the
	// resolver would proceed with manifest-only defaults.
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["t.yaml"] = "/s"
	rdr.md["t.yaml|x.meta.json"] = "---\nstyle: {{unexpanded}}\n---\n# body\n"

	got, err := m.ResolveExportDefaults("t.yaml", "x.meta.json")
	// Either the parser tolerates `{{unexpanded}}` as an unquoted
	// scalar (mid-value `{` is fine in YAML; the parser actually
	// reads it as the literal `{{unexpanded}}` string) or it returns
	// ErrFrontmatterMalformed — we don't crash either way.
	if err != nil {
		t.Logf("err = %v (acceptable — malformed source falls through to manifest)", err)
		return
	}
	if got.Theme == "" {
		t.Logf("Theme parsed as empty; resolver continued gracefully")
	}
}

