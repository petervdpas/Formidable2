package mermaid

import "testing"

func TestValidate_DetectsAndCanonicalisesType(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"flowchart", "flowchart TD\n  A-->B", "flowchart"},
		{"graph canonicalised", "graph LR\n  A-->B", "flowchart"},
		{"gantt", "gantt\n  title A\n  dateFormat YYYY-MM-DD\n  section S\n  Task: a1, 2014-01-01, 30d", "gantt"},
		{"sequence", "sequenceDiagram\n  Alice->>Bob: hi", "sequence"},
		{"state canonicalised", "stateDiagram-v2\n  [*] --> S", "state"},
		{"class", "classDiagram\n  class Foo", "class"},
		{"er", "erDiagram\n  CUSTOMER ||--o{ ORDER : places", "er"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Validate(c.source)
			if !got.OK {
				t.Fatalf("OK = false, want true; errors = %+v", got.Errors)
			}
			if got.DiagramType != c.want {
				t.Fatalf("DiagramType = %q, want %q", got.DiagramType, c.want)
			}
			if len(got.Errors) != 0 {
				t.Fatalf("Errors = %+v, want none", got.Errors)
			}
		})
	}
}

func TestValidate_EmptyIsOKWithNoType(t *testing.T) {
	for _, src := range []string{"", "   ", "\n\n  \n"} {
		got := Validate(src)
		if !got.OK || got.DiagramType != "" || len(got.Errors) != 0 {
			t.Fatalf("Validate(%q) = %+v, want OK with empty type and no errors", src, got)
		}
	}
}

func TestValidate_BlanksLeadingFrontmatter(t *testing.T) {
	const body = "gantt\n  title A\n  dateFormat YYYY-MM-DD\n  section S\n  Task: a1, 2014-01-01, 30d"
	got := Validate("---\ntitle: X\n---\n" + body)
	if !got.OK || got.DiagramType != "gantt" {
		t.Fatalf("got %+v, want OK gantt (frontmatter should be blanked, not rejected)", got)
	}
}

func TestValidate_SkipsComments(t *testing.T) {
	got := Validate("%% a note\nsequenceDiagram\n  Alice->>Bob: hi")
	if !got.OK || got.DiagramType != "sequence" {
		t.Fatalf("got %+v, want OK sequence", got)
	}
}

func TestValidate_UnknownTypeIsParseError(t *testing.T) {
	got := Validate("asdf qwer\n  zxcv")
	if got.OK || got.DiagramType != "" {
		t.Fatalf("got %+v, want not-OK with empty type", got)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("Errors = %+v, want exactly one", got.Errors)
	}
	if e := got.Errors[0]; e.Code != codeParseError || e.Severity != "error" {
		t.Fatalf("issue = %+v, want code=%s severity=error", e, codeParseError)
	}
}

func TestValidate_LiftsLineNumberFromParseError(t *testing.T) {
	got := Validate("journey\n  title T\n  section S\n  Task: 9: Me")
	if got.OK || len(got.Errors) != 1 {
		t.Fatalf("got %+v, want one parse error", got)
	}
	e := got.Errors[0]
	if e.Line != 4 {
		t.Fatalf("issue line = %d, want 4 (lifted from \"line 4:\" prefix)", e.Line)
	}
	if e.Message == "" || e.Message[0] == 'l' && len(e.Message) > 5 && e.Message[:5] == "line " {
		t.Fatalf("message still carries the line prefix: %q", e.Message)
	}
}
