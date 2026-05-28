package render

import (
	"strings"
	"testing"
)

func TestValidate_Empty(t *testing.T) {
	r := ValidateMarkdownTemplate("")
	if !r.OK || len(r.Diagnostics) != 0 {
		t.Fatalf("empty input must be OK with no diagnostics; got %+v", r)
	}
	r = ValidateMarkdownTemplate("   \n\t  ")
	if !r.OK || len(r.Diagnostics) != 0 {
		t.Fatalf("whitespace-only input must be OK; got %+v", r)
	}
}

func TestValidate_PlainContent(t *testing.T) {
	r := ValidateMarkdownTemplate("# Heading\n\nJust prose.\n")
	if !r.OK || len(r.Diagnostics) != 0 {
		t.Fatalf("plain content must validate cleanly; got %+v", r)
	}
}

func TestValidate_KnownHelpersOK(t *testing.T) {
	src := `# {{field "title"}}

{{#each (fieldRaw "tags")}}
- {{this}}
{{/each}}

{{#if (fieldRaw "items")}}
yes
{{else}}
no
{{/if}}
`
	r := ValidateMarkdownTemplate(src)
	if !r.OK {
		t.Fatalf("expected OK, got diagnostics: %+v", r.Diagnostics)
	}
	if len(r.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", r.Diagnostics)
	}
}

func TestValidate_ParseError_SlashElse(t *testing.T) {
	src := `{{#if x}}
A
{{/else}}
B
{{/if}}
`
	r := ValidateMarkdownTemplate(src)
	if r.OK {
		t.Fatalf("expected NOT OK, got %+v", r)
	}
	if len(r.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	d := r.Diagnostics[0]
	if d.Severity != SeverityError {
		t.Errorf("severity: want error, got %q", d.Severity)
	}
	if d.Line == 0 {
		t.Errorf("expected non-zero Line, got 0; message=%q", d.Message)
	}
	if !strings.Contains(strings.ToLower(d.Message), "parse error") {
		t.Errorf("message should mention parse error; got %q", d.Message)
	}
}

func TestValidate_UnknownHelper_Warning(t *testing.T) {
	src := `{{filed "title"}}`
	r := ValidateMarkdownTemplate(src)
	if !r.OK {
		t.Fatalf("unknown helper is a warning, not an error; got %+v", r)
	}
	if len(r.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d (%+v)", len(r.Diagnostics), r.Diagnostics)
	}
	d := r.Diagnostics[0]
	if d.Severity != SeverityWarning {
		t.Errorf("severity: want warning, got %q", d.Severity)
	}
	if d.Helper != "filed" {
		t.Errorf("helper: want filed, got %q", d.Helper)
	}
}

func TestValidate_BareLookupNotFlagged(t *testing.T) {
	src := `{{title}} and {{some.path}}`
	r := ValidateMarkdownTemplate(src)
	if !r.OK || len(r.Diagnostics) != 0 {
		t.Fatalf("bare lookups must not be flagged; got %+v", r)
	}
}

func TestValidate_UnknownBlockHelper(t *testing.T) {
	src := `{{#wat items}}x{{/wat}}`
	r := ValidateMarkdownTemplate(src)
	if !r.OK || len(r.Diagnostics) != 1 {
		t.Fatalf("expected 1 warning, got %+v", r)
	}
	d := r.Diagnostics[0]
	if d.Severity != SeverityWarning || d.Helper != "wat" {
		t.Errorf("unexpected diagnostic: %+v", d)
	}
	if !strings.Contains(d.Message, "block helper") {
		t.Errorf("block warning should mention 'block helper': %q", d.Message)
	}
}

func TestValidate_DedupesRepeatedUnknown(t *testing.T) {
	src := `{{filed "a"}} {{filed "b"}} {{filed "c"}}`
	r := ValidateMarkdownTemplate(src)
	if len(r.Diagnostics) != 1 {
		t.Fatalf("expected dedupe to 1 diagnostic, got %d (%+v)", len(r.Diagnostics), r.Diagnostics)
	}
}

func TestValidate_SubexpressionHelperChecked(t *testing.T) {
	src := `{{#if (filed "x")}}A{{/if}}`
	r := ValidateMarkdownTemplate(src)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Helper != "filed" {
		t.Fatalf("subexpression helper must be flagged; got %+v", r.Diagnostics)
	}
}

func TestValidate_CatalogHelperVariety(t *testing.T) {
	src := `{{yamlList (fieldRaw "tags")}}
{{dateFormat (field "due") "Mon, 02 Jan 2006" "nl"}}
{{#loop "members"}}{{field "name"}}{{/loop}}
`
	r := ValidateMarkdownTemplate(src)
	if !r.OK || len(r.Diagnostics) != 0 {
		t.Fatalf("catalog helpers must validate cleanly; got %+v", r.Diagnostics)
	}
}
