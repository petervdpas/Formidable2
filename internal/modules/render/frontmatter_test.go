package render

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseFrontmatter_Empty(t *testing.T) {
	fm, body, err := ParseFrontmatter("")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fm != nil {
		t.Errorf("want nil frontmatter, got %v", fm)
	}
	if body != "" {
		t.Errorf("want empty body, got %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	src := "# hello\n\nbody text\n"
	fm, body, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fm != nil {
		t.Errorf("want nil frontmatter, got %v", fm)
	}
	if body != src {
		t.Errorf("body mismatch: got %q", body)
	}
}

func TestParseFrontmatter_Valid(t *testing.T) {
	src := "---\ntitle: Hello\ncount: 3\n---\n# body\n"
	fm, body, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fm == nil {
		t.Fatalf("want frontmatter, got nil")
	}
	if got := fm["title"]; got != "Hello" {
		t.Errorf("title = %v, want Hello", got)
	}
	if got := fm["count"]; got != 3 {
		t.Errorf("count = %v, want 3", got)
	}
	if body != "# body\n" {
		t.Errorf("body = %q, want %q", body, "# body\n")
	}
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	src := "---\n: not: valid: yaml:\n  a\n---\nbody\n"
	fm, body, err := ParseFrontmatter(src)
	if err == nil {
		t.Errorf("want error for invalid yaml, got nil")
	}
	if fm != nil {
		t.Errorf("want nil frontmatter on err, got %v", fm)
	}
	// Body falls back to original markdown so caller can still render.
	if body != src {
		t.Errorf("body should fall back to source on err, got %q", body)
	}
}

func TestBuildFrontmatter_Empty(t *testing.T) {
	got := BuildFrontmatter(nil, "body\n")
	if got != "body\n" {
		t.Errorf("want passthrough body when frontmatter empty, got %q", got)
	}
	got = BuildFrontmatter(map[string]any{}, "body\n")
	if got != "body\n" {
		t.Errorf("empty map should also passthrough, got %q", got)
	}
}

func TestBuildFrontmatter_RoundTrip(t *testing.T) {
	src := map[string]any{"title": "Hello", "count": 3}
	out := BuildFrontmatter(src, "# body\n")
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("missing leading delimiter: %q", out)
	}
	if !strings.Contains(out, "title: Hello") {
		t.Errorf("missing title: %q", out)
	}
	if !strings.Contains(out, "count: 3") {
		t.Errorf("missing count: %q", out)
	}
	if !strings.HasSuffix(out, "# body\n") {
		t.Errorf("body not preserved at end: %q", out)
	}

	// Round-trip back through Parse.
	fm, body, err := ParseFrontmatter(out)
	if err != nil {
		t.Fatalf("unexpected err round-tripping: %v", err)
	}
	if fm["title"] != "Hello" || fm["count"] != 3 {
		t.Errorf("round-trip lost data: %v", fm)
	}
	if body != "# body\n" {
		t.Errorf("round-trip body mismatch: %q", body)
	}
}

func TestBuildFrontmatter_SequencesEmittedFlowStyle(t *testing.T) {
	// Sequences must land inline (tags: [a, b, c]) rather than yaml.v3's
	// default block style. This drives setSequenceStyle's SequenceNode arm.
	out := BuildFrontmatter(map[string]any{"tags": []any{"a", "b", "c"}}, "body\n")
	if !strings.Contains(out, "tags: [a, b, c]") {
		t.Errorf("sequence not in flow style: %q", out)
	}
	if strings.Contains(out, "- a") {
		t.Errorf("sequence leaked block style: %q", out)
	}
}

func TestBuildFrontmatter_NestedSequenceFlowStyle(t *testing.T) {
	// A sequence nested under a mapping must also be flattened to flow
	// style, covering the recursive descent in setSequenceStyle.
	out := BuildFrontmatter(map[string]any{
		"meta": map[string]any{"langs": []any{"go", "vue"}},
	}, "body\n")
	if !strings.Contains(out, "langs: [go, vue]") {
		t.Errorf("nested sequence not in flow style: %q", out)
	}
}

func TestFilterFrontmatter(t *testing.T) {
	src := map[string]any{"a": 1, "b": 2, "c": 3}
	got := FilterFrontmatter(src, []string{"a", "c", "missing"})
	want := map[string]any{"a": 1, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %v, want %v", got, want)
	}
}

func TestFilterFrontmatter_NilKeys(t *testing.T) {
	got := FilterFrontmatter(map[string]any{"a": 1}, nil)
	if len(got) != 0 {
		t.Errorf("nil keep list should yield empty map, got %v", got)
	}
}
