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
