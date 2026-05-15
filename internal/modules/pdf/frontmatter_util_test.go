package pdf

import (
	"errors"
	"strings"
	"testing"
)

// ---------- InjectFrontmatter ----------

func TestInject_EmptyMarkdown_ReturnsScaffold(t *testing.T) {
	got, err := InjectFrontmatter("")
	if err != nil {
		t.Fatalf("Inject empty: %v", err)
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("Inject result missing leading `---`; got:\n%s", got)
	}
	for _, want := range []string{"cover:", "page:", "#toc:", "#footer:", "#signature:"} {
		if !strings.Contains(got, want) {
			t.Errorf("scaffold missing %q", want)
		}
	}
}

func TestInject_PrependsToExistingBody(t *testing.T) {
	body := "# Heading\n\nSome body text.\n"
	got, err := InjectFrontmatter(body)
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if !strings.HasSuffix(got, body) {
		t.Errorf("body not preserved at end; tail = %q",
			got[len(got)-min(80, len(got)):])
	}
}

func TestInject_RefusesWhenFrontmatterExists(t *testing.T) {
	src := "---\ntitle: x\n---\n# body\n"
	_, err := InjectFrontmatter(src)
	if !errors.Is(err, ErrFrontmatterAlreadyPresent) {
		t.Errorf("err = %v, want ErrFrontmatterAlreadyPresent", err)
	}
}

// ---------- MigrateFrontmatter ----------

func TestMigrate_NoFrontmatter_ReturnsVerbatimNoOp(t *testing.T) {
	src := "# just body\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got.Markdown != src {
		t.Errorf("Markdown = %q, want %q", got.Markdown, src)
	}
	if got.HadFrontmatter {
		t.Errorf("HadFrontmatter = true, want false")
	}
}

func TestMigrate_EisvogelToCover(t *testing.T) {
	src := `---
title: Datastroom
subtitle: My Subtitle
author: Team
date: 2026-05-15
version: '1.0'
---
# Body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	wantInOutput := []string{
		"cover:",
		"  title: Datastroom",
		"  subtitle: My Subtitle",
		"  author: Team",
		`  date: "2026-05-15"`, // yaml.v3 quotes date-looking strings
		`  version: "1.0"`,
		"# Body",
	}
	for _, s := range wantInOutput {
		if !strings.Contains(got.Markdown, s) {
			t.Errorf("output missing %q. Full output:\n%s", s, got.Markdown)
		}
	}
	// Mappings should record each eisvogel → picoloom rename.
	expectedMappings := map[string]string{
		"title":    "cover.title",
		"subtitle": "cover.subtitle",
		"author":   "cover.author",
		"date":     "cover.date",
		"version":  "cover.version",
	}
	gotMappings := map[string]string{}
	for _, m := range got.Mappings {
		gotMappings[m.From] = m.To
	}
	for k, v := range expectedMappings {
		if gotMappings[k] != v {
			t.Errorf("mapping[%q] = %q, want %q", k, gotMappings[k], v)
		}
	}
}

func TestMigrate_PapersizeToPageSize(t *testing.T) {
	src := "---\npapersize: a4paper\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "page:\n") || !strings.Contains(got.Markdown, "size: a4") {
		t.Errorf("expected page.size: a4 in output:\n%s", got.Markdown)
	}
}

func TestMigrate_PapersizeUnknownPreserved(t *testing.T) {
	src := "---\npapersize: tabloid\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "legacy:") {
		t.Errorf("unknown papersize should land in legacy:\n%s", got.Markdown)
	}
	if !strings.Contains(got.Markdown, "papersize: tabloid") {
		t.Errorf("legacy block should contain original papersize")
	}
	if len(got.Warnings) == 0 {
		t.Errorf("warnings should mention unknown papersize")
	}
}

func TestMigrate_UnknownKeysPreservedAsLegacy(t *testing.T) {
	src := `---
title: Mine
keywords: '[k1, k2]'
fontsize: 9pt
titlepage-color: '#F8F8F8'
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "legacy:") {
		t.Errorf("expected legacy block, got:\n%s", got.Markdown)
	}
	for _, k := range []string{"keywords:", "fontsize:", "titlepage-color:"} {
		if !strings.Contains(got.Markdown, k) {
			t.Errorf("legacy block missing %q\nFull:\n%s", k, got.Markdown)
		}
	}
	for _, k := range []string{"keywords", "fontsize", "titlepage-color"} {
		found := false
		for _, p := range got.Preserved {
			if p == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Preserved missing %q (have %v)", k, got.Preserved)
		}
	}
}

func TestMigrate_PicoloomBlocksPassThroughUntouched(t *testing.T) {
	src := `---
cover:
  template: classic
  title: Already picoloom
page:
  size: letter
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "template: classic") {
		t.Errorf("picoloom cover should pass through")
	}
	if !strings.Contains(got.Markdown, "size: letter") {
		t.Errorf("picoloom page.size should pass through")
	}
	if len(got.Mappings) != 0 {
		t.Errorf("no mappings expected (already picoloom-shaped); got %+v", got.Mappings)
	}
}

func TestMigrate_PicoloomBeatsEisvogelOnConflict(t *testing.T) {
	// Hybrid: BOTH eisvogel title AND cover.title exist. Picoloom wins,
	// the eisvogel key gets preserved under legacy with a warning.
	src := `---
title: Eisvogel Wins?
cover:
  title: Picoloom Wins
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "title: Picoloom Wins") {
		t.Errorf("picoloom cover.title should be preserved:\n%s", got.Markdown)
	}
	if !strings.Contains(got.Markdown, "legacy:") {
		t.Errorf("eisvogel title should land in legacy block")
	}
	if len(got.Warnings) == 0 {
		t.Errorf("expected a warning about the conflict")
	}
}

func TestMigrate_MalformedFrontmatter(t *testing.T) {
	src := "---\ntitle: [broken yaml\n---\nbody\n"
	_, err := MigrateFrontmatter(src)
	if !errors.Is(err, ErrFrontmatterMalformed) {
		t.Errorf("err = %v, want ErrFrontmatterMalformed", err)
	}
}

func TestMigrate_MissingClosingDashes(t *testing.T) {
	src := "---\ntitle: x\nbody but no closing fence\n"
	_, err := MigrateFrontmatter(src)
	if !errors.Is(err, ErrFrontmatterMalformed) {
		t.Errorf("err = %v, want ErrFrontmatterMalformed", err)
	}
}

func TestMigrate_EmptyFrontmatter_NoOp(t *testing.T) {
	src := "---\n---\n# body\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !got.HadFrontmatter {
		t.Errorf("HadFrontmatter = false, want true (we did see the fences)")
	}
}

func TestService_InjectFrontmatter_DelegatesToManager(t *testing.T) {
	svc := &Service{}
	got, err := svc.InjectFrontmatter("")
	if err != nil {
		t.Fatalf("svc.Inject: %v", err)
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("missing scaffold")
	}
}

func TestMigrate_HandlebarsExpressionsSurviveRoundTrip(t *testing.T) {
	// The audit-controls template from the user's screenshot — exactly
	// the case the first MigrateFrontmatter shipped broken on. yaml.v3
	// chokes on `{` in unquoted scalars because it's a flow-mapping
	// marker. The masker swaps every `{{…}}` for a safe sentinel before
	// parse and restores it after emit.
	src := `---
title: Managementsamenvatting {{field "audit-control-id"}}
subtitle: {{field "audit-control-id"}} - {{field "audit-control-naam"}}
author: Team Integration Services
keywords: '[Aanpak, Management, Samenvatting, {{tags (fieldRaw "audit-control-tags") withHash=false}}]'
fontsize: 10pt
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate (handlebars): %v", err)
	}
	for _, want := range []string{
		`title: Managementsamenvatting {{field "audit-control-id"}}`,
		`subtitle: {{field "audit-control-id"}} - {{field "audit-control-naam"}}`,
		"author: Team Integration Services",
		"legacy:",
		"fontsize: 10pt",
		`{{tags (fieldRaw "audit-control-tags") withHash=false}}`,
	} {
		if !strings.Contains(got.Markdown, want) {
			t.Errorf("missing %q in migrated output:\n%s", want, got.Markdown)
		}
	}
	// No sentinels should leak into the final markdown.
	if strings.Contains(got.Markdown, "__HBS_") {
		t.Errorf("Handlebars sentinel leaked into output:\n%s", got.Markdown)
	}
}

func TestMaskHandlebars_RoundTrip(t *testing.T) {
	src := `title: {{a}} and {{b "with arg"}}
keywords: [{{tags}}, x]
plain: nothing here
nested: {{outer (inner "x") y=true}}
`
	masked, tokens := maskHandlebars(src)
	if strings.Contains(masked, "{{") || strings.Contains(masked, "}}") {
		t.Errorf("mask left {{...}} in output:\n%s", masked)
	}
	if len(tokens) != 4 {
		t.Errorf("token count = %d, want 4", len(tokens))
	}
	got := unmaskHandlebars(masked, tokens)
	if got != src {
		t.Errorf("round trip mismatch.\nwant:\n%s\ngot:\n%s", src, got)
	}
}

func TestMaskHandlebars_NoExpressions(t *testing.T) {
	src := "title: Plain Text\nkeywords: a, b, c\n"
	masked, tokens := maskHandlebars(src)
	if masked != src {
		t.Errorf("plain source mutated: %q", masked)
	}
	if len(tokens) != 0 {
		t.Errorf("plain source produced %d tokens", len(tokens))
	}
}

func TestMaskHandlebars_AdjacentExpressions(t *testing.T) {
	src := "x: {{a}}{{b}}\n"
	masked, tokens := maskHandlebars(src)
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens for adjacent {{a}}{{b}}, got %d (masked: %q)", len(tokens), masked)
	}
}

func TestUnmaskHandlebars_LongTokenIndexes(t *testing.T) {
	// __HBS_1__ must NOT be replaced first if __HBS_10__ also exists —
	// otherwise the longer token's prefix collides.
	tokens := map[string]string{
		"__HBS_1__":  "{{one}}",
		"__HBS_10__": "{{ten}}",
	}
	out := unmaskHandlebars("a __HBS_10__ b __HBS_1__", tokens)
	want := "a {{ten}} b {{one}}"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// TestMigrate_RealAuditControlsTemplate runs MigrateFrontmatter on
// the actual frontmatter the user has in ~/Documents/Example/
// templates/audit-controls.yaml as of 2026-05-16. The earlier
// regression (yaml.v3 chokes on `{`) was discovered against this
// exact template; this test pins the full real-world surface so the
// next refactor can't regress on Handlebars + hash-leading values +
// numeric-looking strings + colons-in-paths all at once.
func TestMigrate_RealAuditControlsTemplate(t *testing.T) {
	src := `---
title: Audit Control
subtitle: {{field "nba-id"}} - {{field "control-measure"}}
author: Team Integration Services
keywords: '[Governance, {{field "nba-id"}}, {{tags (fieldRaw "nba-tags") withHash=false}}]'
titlepage-color: #F8F8F8
titlepage-text-color: 552255
titlepage-rule-color: 552255
titlepage-logo: C:/Projects/team/data/fontys-logo.png
logo-width: 140pt
table-use-row-colors: true
table-row-color: "D3D3D3"
toc: true
toc-title: Inhoudsopgave
toc-own-page: true
---


## {{field "nba-id"}} - {{field "control-measure"}}
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate (real template): %v", err)
	}

	// Mapped → cover.* (verbatim Handlebars preserved).
	mustContain := []string{
		"cover:",
		"title: Audit Control",
		`subtitle: {{field "nba-id"}} - {{field "control-measure"}}`,
		"author: Team Integration Services",
	}
	for _, s := range mustContain {
		if !strings.Contains(got.Markdown, s) {
			t.Errorf("missing %q in migrated output:\n%s", s, got.Markdown)
		}
	}

	// The legacy block must preserve the hash-color (yaml.v3 would
	// otherwise drop `#F8F8F8` as a comment), the numeric-looking
	// color, the colon-in-path logo, AND the Windows path with its
	// `C:` colon-after-letter pattern.
	legacyMustContain := []string{
		"legacy:",
		"F8F8F8",  // hash color preserved (with or without `#`)
		"552255",  // numeric color preserved
		"C:/Projects/team/data/fontys-logo.png", // colon-in-path
		"140pt",
		"Inhoudsopgave",
	}
	for _, s := range legacyMustContain {
		if !strings.Contains(got.Markdown, s) {
			t.Errorf("legacy block missing %q in output:\n%s", s, got.Markdown)
		}
	}

	// Sentinels must not leak through.
	if strings.Contains(got.Markdown, "__HBS_") {
		t.Errorf("Handlebars sentinel leaked through:\n%s", got.Markdown)
	}

	// Body content survives.
	if !strings.Contains(got.Markdown, `## {{field "nba-id"}} - {{field "control-measure"}}`) {
		t.Errorf("body content lost")
	}
}

func TestMigrate_HashLeadingValuePreserved(t *testing.T) {
	// Minimum reproducer for the user's most surprising loss:
	// `titlepage-color: #F8F8F8` would parse as `titlepage-color: null`
	// in raw yaml.v3 because `#F8F8F8` is treated as a trailing comment.
	src := "---\ntitlepage-color: #F8F8F8\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "F8F8F8") {
		t.Errorf("hash-color value lost during migration:\n%s", got.Markdown)
	}
}

func TestMigrate_NumericColorPreserved(t *testing.T) {
	// `552255` as an unquoted scalar parses as int. We don't try to
	// preserve string-ness (no context tells us it's meant as hex),
	// but the digits must survive in the legacy block.
	src := "---\ntitlepage-text-color: 552255\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "552255") {
		t.Errorf("numeric color value lost:\n%s", got.Markdown)
	}
}

func TestMigrate_WindowsPathColonPreserved(t *testing.T) {
	// Defensive: yaml.v3 already handles `C:/path` correctly (colon
	// not followed by space stays in scalar), but pin the behavior
	// so a future preprocessor change can't regress it.
	src := "---\ntitlepage-logo: C:/Projects/team/data/fontys-logo.png\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !strings.Contains(got.Markdown, "C:/Projects/team/data/fontys-logo.png") {
		t.Errorf("Windows path with colon mangled:\n%s", got.Markdown)
	}
}

func TestQuoteHashLeadingValues_DoesNotTouchCommentLines(t *testing.T) {
	// A line that's ONLY a comment (no `key:` before the `#`) must
	// pass through untouched — the regex is anchored to "looks like
	// a key:value with hash-leading value".
	src := "# top-level comment\nkey: value\n# another comment\n"
	got := quoteHashLeadingValues(src)
	if got != src {
		t.Errorf("comment lines should be untouched:\nwant: %q\ngot:  %q", src, got)
	}
}

func TestQuoteHashLeadingValues_HandlesInternalQuotes(t *testing.T) {
	// Single quotes inside the value must be escaped via doubling.
	src := "key: #it's a hex\n"
	got := quoteHashLeadingValues(src)
	if !strings.Contains(got, `'#it''s a hex'`) {
		t.Errorf("internal quote not escaped:\n%s", got)
	}
}

func TestSaveDiskCover_FilesystemErrorWrapped(t *testing.T) {
	// The audit caller asked for this — SaveCover only had validation
	// tests. Now it has an I/O-failure test using memFS's saveErr hook.
	fs := scaffoldedFS(t)
	fs.saveErr = errors.New("disk full")
	html := testValidCoverHTML("Custom")
	err := saveDiskCover(fs, "custom", html)
	if err == nil {
		t.Errorf("expected fs error to bubble up; got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("inner error lost; got: %v", err)
	}
}

func TestResolveCoverTemplateSet_SignatureLoadFailure(t *testing.T) {
	// If signature.html can't be loaded (corrupted FS, deleted after
	// scaffold), ResolveCoverTemplateSet must surface a structured
	// error rather than emit a nil signature into picoloom.
	fs := scaffoldedFS(t)
	fs.loadErr = errors.New("permission denied")
	enabled := true
	fm := &CoverFM{Enabled: &enabled, Template: "classic"}
	_, err := ResolveCoverTemplateSet(fm, "", fs)
	if err == nil {
		t.Errorf("expected error; got nil")
	}
	if !errors.Is(err, ErrSignatureMissing) {
		t.Errorf("err = %v, want wrap of ErrSignatureMissing", err)
	}
}

func TestMigrate_EmptyHandlebars_StillRoundTrips(t *testing.T) {
	// Edge case: `{{}}` is technically a valid Handlebars no-op
	// (renders to ""). The masker should still capture it as a token
	// and the migration shouldn't crash.
	src := "---\ntitle: Before {{}} After\n---\nbody\n"
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate empty-handlebars: %v", err)
	}
	if !strings.Contains(got.Markdown, "Before {{}} After") {
		t.Errorf("empty handlebars expression not preserved:\n%s", got.Markdown)
	}
}

func TestMigrate_HandlebarsInsideQuotedScalar(t *testing.T) {
	// `keywords: '[…, {{tags …}}]'` — Handlebars inside a single-quoted
	// flow-sequence string. The single quotes already protect yaml.v3,
	// but the masker must STILL run to support the surrounding shape;
	// and the unmask step must not double-substitute or lose the
	// expression on round-trip.
	src := `---
keywords: '[Aanpak, Management, {{tags (fieldRaw "x") withHash=false}}]'
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate quoted-handlebars: %v", err)
	}
	if !strings.Contains(got.Markdown, `{{tags (fieldRaw "x") withHash=false}}`) {
		t.Errorf("Handlebars lost from quoted flow sequence:\n%s", got.Markdown)
	}
}

func TestMigrate_HashLeadingValueMultipleKeys(t *testing.T) {
	// Stress: many hash-leading colors in a row. All must survive.
	src := `---
titlepage-color: #F8F8F8
titlepage-text-color: #552255
titlepage-rule-color: #552255
---
body
`
	got, err := MigrateFrontmatter(src)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, want := range []string{"#F8F8F8", "#552255"} {
		if !strings.Contains(got.Markdown, want) {
			t.Errorf("hash value %q lost:\n%s", want, got.Markdown)
		}
	}
}

func TestMerge_SignatureLinksReplaceAtomic(t *testing.T) {
	// Multi-element slice case the existing happy-path tests miss.
	// Per the Stage 3 contract: a non-empty higher-layer slice
	// REPLACES the lower layer's slice atomically (no concat).
	hi := Frontmatter{Signature: &SignatureFM{Links: []LinkFM{{Label: "hi-only", URL: "h"}}}}
	lo := Frontmatter{Signature: &SignatureFM{Links: []LinkFM{
		{Label: "lo-1", URL: "1"},
		{Label: "lo-2", URL: "2"},
	}}}
	merged := Merge(hi, lo)
	if merged.Signature == nil {
		t.Fatalf("Signature nil after merge")
	}
	if len(merged.Signature.Links) != 1 {
		t.Errorf("Links count = %d, want 1 (higher layer replaces atomically); got %+v",
			len(merged.Signature.Links), merged.Signature.Links)
	}
	if merged.Signature.Links[0].Label != "hi-only" {
		t.Errorf("Links[0].Label = %q, want hi-only", merged.Signature.Links[0].Label)
	}
}

func TestService_MigrateFrontmatter_DelegatesToManager(t *testing.T) {
	svc := &Service{}
	got, err := svc.MigrateFrontmatter("---\ntitle: x\n---\nbody\n")
	if err != nil {
		t.Fatalf("svc.Migrate: %v", err)
	}
	if !got.HadFrontmatter || len(got.Mappings) == 0 {
		t.Errorf("expected HadFrontmatter=true with mappings; got %+v", got)
	}
}
