package pdf

import "testing"

const validCover = `<!--
  formidable-cover: 1
  name: Test
  description: One-line desc.
-->
<section class="cover">
  <h1>{{.Title}}</h1>
</section>
<span data-cover-end></span>`

func TestValidateCover_Valid(t *testing.T) {
	v := ValidateCover(validCover)
	if !v.OK {
		t.Errorf("OK = false; issues = %+v", v.Issues)
	}
	if v.Token == nil || v.Token.Version != 1 {
		t.Errorf("Token = %+v, want version=1", v.Token)
	}
	if v.Token.Name != "Test" {
		t.Errorf("Token.Name = %q, want Test", v.Token.Name)
	}
	if v.Token.Description != "One-line desc." {
		t.Errorf("Token.Description = %q", v.Token.Description)
	}
}

func TestValidateCover_NoMagicLine(t *testing.T) {
	html := `<section class="cover"><h1>{{.Title}}</h1></section><span data-cover-end></span>`
	v := ValidateCover(html)
	if v.OK {
		t.Errorf("OK = true; want false (missing magic line)")
	}
	if !hasIssueCode(v, "no-magic-line") {
		t.Errorf("expected no-magic-line issue, got %+v", v.Issues)
	}
}

func TestValidateCover_CommentWithoutMagicLine(t *testing.T) {
	// Leading comment present, but lacks `formidable-cover:` line.
	html := `<!-- this is just a regular comment -->
<section class="cover"><h1>{{.Title}}</h1></section><span data-cover-end></span>`
	v := ValidateCover(html)
	if v.OK {
		t.Errorf("OK = true; want false")
	}
	if !hasIssueCode(v, "no-magic-line") {
		t.Errorf("expected no-magic-line issue, got %+v", v.Issues)
	}
}

func TestValidateCover_VersionTooHigh(t *testing.T) {
	html := `<!-- formidable-cover: 999 -->
<section class="cover"><h1>{{.Title}}</h1></section><span data-cover-end></span>`
	v := ValidateCover(html)
	if v.OK {
		t.Errorf("OK = true; want false (version too high)")
	}
	if !hasIssueCode(v, "version-unsupported") {
		t.Errorf("expected version-unsupported issue, got %+v", v.Issues)
	}
}

func TestValidateCover_NoCoverEnd(t *testing.T) {
	html := `<!-- formidable-cover: 1 -->
<section class="cover"><h1>{{.Title}}</h1></section>`
	v := ValidateCover(html)
	if v.OK {
		t.Errorf("OK = true; want false (no sentinel)")
	}
	if !hasIssueCode(v, "no-cover-end") {
		t.Errorf("expected no-cover-end issue, got %+v", v.Issues)
	}
}

func TestValidateCover_BrokenTemplate(t *testing.T) {
	// Mismatched template directives.
	html := `<!-- formidable-cover: 1 -->
<section class="cover">{{if .Title}}{{.Title}}</section>
<span data-cover-end></span>`
	v := ValidateCover(html)
	if v.OK {
		t.Errorf("OK = true; want false (broken template)")
	}
	if !hasIssueCode(v, "template-parse") {
		t.Errorf("expected template-parse issue, got %+v", v.Issues)
	}
}

func TestValidateCover_WarningNoTitlePlaceholder(t *testing.T) {
	// All errors absent; warnings should still surface.
	html := `<!-- formidable-cover: 1 -->
<section class="cover">No title here.</section>
<span data-cover-end></span>`
	v := ValidateCover(html)
	if !v.OK {
		t.Errorf("OK = false unexpectedly; issues = %+v", v.Issues)
	}
	if !hasIssueCode(v, "no-title-placeholder") {
		t.Errorf("expected no-title-placeholder warning, got %+v", v.Issues)
	}
}

func TestValidateCover_WarningNoCoverClass(t *testing.T) {
	html := `<!-- formidable-cover: 1 -->
<section><h1>{{.Title}}</h1></section>
<span data-cover-end></span>`
	v := ValidateCover(html)
	if !v.OK {
		t.Errorf("OK = false unexpectedly; issues = %+v", v.Issues)
	}
	if !hasIssueCode(v, "no-cover-class") {
		t.Errorf("expected no-cover-class warning, got %+v", v.Issues)
	}
}

func TestValidateCover_CoverClassSingleQuotes(t *testing.T) {
	html := `<!-- formidable-cover: 1 -->
<section class='cover-page cover'><h1>{{.Title}}</h1></section>
<span data-cover-end></span>`
	v := ValidateCover(html)
	if !v.OK {
		t.Errorf("OK = false; issues = %+v", v.Issues)
	}
	if hasIssueCode(v, "no-cover-class") {
		t.Errorf("single-quoted class=\"cover\" should still satisfy the check")
	}
}

func TestValidateCover_EmbeddedCoversAllValid(t *testing.T) {
	// The seeds we ship MUST pass validation - otherwise scaffolding
	// would put broken files on disk.
	entries, err := coversFS.ReadDir(coversDir)
	if err != nil {
		t.Fatalf("read embedded covers: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == "signature.html" {
			continue // signature has its own magic family; covered separately
		}
		bytes, _ := coversFS.ReadFile(coversDir + "/" + e.Name())
		v := ValidateCover(string(bytes))
		if !v.OK {
			t.Errorf("seed %q fails validation: %+v", e.Name(), v.Issues)
		}
	}
}

// hasIssueCode reports whether any issue in v has the given code.
func hasIssueCode(v CoverValidation, code string) bool {
	for _, i := range v.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
