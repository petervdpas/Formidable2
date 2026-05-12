package csv

import (
	"strings"
	"testing"
)

func TestApply_None(t *testing.T) {
	if got := Apply("hello", "none", "", ModeStorage); got != "hello" {
		t.Errorf("none = %q, want %q", got, "hello")
	}
	if got := Apply("  spaced  ", "none", "", ModeStorage); got != "  spaced  " {
		t.Errorf("none should be identity, got %q", got)
	}
}

func TestApply_CaseRules(t *testing.T) {
	cases := []struct {
		rule, in, want string
	}{
		{"lowercase", "HeLLo", "hello"},
		{"uppercase", "HeLLo", "HELLO"},
		{"capitalize", "hello world", "Hello World"},
		{"capitalize", "FOO bar", "FOO Bar"},
		{"capitalize", "a-b c", "A-B C"},
		{"trim", "  hi  ", "hi"},
		{"trim+lower", "  HI  ", "hi"},
		{"trim+upper", "  hi  ", "HI"},
		{"trim+cap", "  hello world  ", "Hello World"},
	}
	for _, tc := range cases {
		if got := Apply(tc.in, tc.rule, "", ModeStorage); got != tc.want {
			t.Errorf("%s(%q) = %q, want %q", tc.rule, tc.in, got, tc.want)
		}
	}
}

func TestApply_FirstLastN(t *testing.T) {
	cases := []struct {
		rule, in, param, want string
	}{
		{"first-n", "hello", "3", "hel"},
		{"first-n", "hi", "10", "hi"}, // N > len
		{"first-n", "hello", "0", "hello"},
		{"first-n", "hello", "", "hello"}, // empty param → identity
		{"last-n", "hello", "3", "llo"},
		{"last-n", "hi", "10", "hi"},
		{"last-n", "hello", "0", "hello"},
		{"last-n", "hello", "", "hello"},
	}
	for _, tc := range cases {
		if got := Apply(tc.in, tc.rule, tc.param, ModeStorage); got != tc.want {
			t.Errorf("%s(%q, %q) = %q, want %q", tc.rule, tc.in, tc.param, got, tc.want)
		}
	}
}

func TestApply_FirstLastN_NegativeOrJunk(t *testing.T) {
	// Negative or non-numeric N → no-op (identity), never panic.
	cases := []string{"-3", "abc", "1.5"}
	for _, p := range cases {
		if got := Apply("hello", "first-n", p, ModeStorage); got != "hello" {
			t.Errorf("first-n with bad param %q: got %q, want identity", p, got)
		}
		if got := Apply("hello", "last-n", p, ModeStorage); got != "hello" {
			t.Errorf("last-n with bad param %q: got %q, want identity", p, got)
		}
	}
}

func TestApply_Split(t *testing.T) {
	cases := []struct {
		in, sep, want string
	}{
		{"a,b,c", ",", "a, b, c"},
		{"a , b , c", ",", "a, b, c"}, // trims each
		{"a,b,,c", ",", "a, b, c"},    // drops empties
		{"a;b;c", ";", "a, b, c"},
		{"a,b,c", "", "a, b, c"}, // empty sep → default ","
	}
	for _, tc := range cases {
		if got := Apply(tc.in, "split", tc.sep, ModeStorage); got != tc.want {
			t.Errorf("split(%q, %q) = %q, want %q", tc.in, tc.sep, got, tc.want)
		}
	}
}

func TestApply_BoolMatch(t *testing.T) {
	cases := []struct {
		in, trueVal, want string
	}{
		{"yes", "yes", "true"},
		{"YES", "yes", "true"},
		{" yes ", "yes", "true"},
		{"no", "yes", "false"},
		{"", "yes", "false"},
		{"1", "1", "true"},
	}
	for _, tc := range cases {
		if got := Apply(tc.in, "bool-match", tc.trueVal, ModeStorage); got != tc.want {
			t.Errorf("bool-match(%q, %q) = %q, want %q", tc.in, tc.trueVal, got, tc.want)
		}
	}
}

func TestApply_SplitTable_StorageMode(t *testing.T) {
	got := Apply("a,b;c,d", "split-table", "; ,", ModeStorage)
	// Expect JSON of [["a","b"],["c","d"]]
	want := `[["a","b"],["c","d"]]`
	if got != want {
		t.Errorf("split-table storage = %q, want %q", got, want)
	}
}

func TestApply_SplitTable_DefaultSeps(t *testing.T) {
	// Empty param → row=";" col=","
	got := Apply("a,b;c,d", "split-table", "", ModeStorage)
	want := `[["a","b"],["c","d"]]`
	if got != want {
		t.Errorf("split-table default seps = %q, want %q", got, want)
	}
}

func TestApply_SplitTable_PreviewMode(t *testing.T) {
	got := Apply("a,b;c,d", "split-table", "; ,", ModePreview)
	want := "a, b | c, d"
	if got != want {
		t.Errorf("split-table preview = %q, want %q", got, want)
	}
}

func TestApply_SplitTable_DropsEmptyRows(t *testing.T) {
	got := Apply("a,b;;c,d", "split-table", "; ,", ModeStorage)
	if !strings.Contains(got, `["a","b"]`) || !strings.Contains(got, `["c","d"]`) {
		t.Errorf("split-table dropped rows wrong: %q", got)
	}
	if strings.Contains(got, `[""]`) {
		t.Errorf("split-table left an empty row: %q", got)
	}
}

func TestApply_UnknownRule(t *testing.T) {
	if got := Apply("hello", "lol-not-a-rule", "", ModeStorage); got != "hello" {
		t.Errorf("unknown rule should pass through, got %q", got)
	}
}

func TestApply_EmptyValue(t *testing.T) {
	for _, rule := range Rules() {
		got := Apply("", rule, "", ModeStorage)
		// split-table on empty becomes "[]"; bool-match becomes "false";
		// everything else should yield "" or a stable harmless result.
		switch rule {
		case "split-table":
			if got != "[]" {
				t.Errorf("split-table('') = %q, want %q", got, "[]")
			}
		case "bool-match":
			if got != "false" {
				t.Errorf("bool-match('') = %q, want %q", got, "false")
			}
		default:
			if got != "" {
				t.Errorf("%s('') = %q, want %q", rule, got, "")
			}
		}
	}
}

func TestRules_ContainsAllExpected(t *testing.T) {
	want := []string{
		"none", "lowercase", "uppercase", "capitalize",
		"trim", "trim+lower", "trim+upper", "trim+cap",
		"first-n", "last-n", "split", "bool-match", "split-table",
	}
	got := Rules()
	if len(got) != len(want) {
		t.Fatalf("Rules() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, r := range want {
		if got[i] != r {
			t.Errorf("Rules()[%d] = %q, want %q", i, got[i], r)
		}
	}
}

func TestExcludedFieldTypes(t *testing.T) {
	got := ExcludedFieldTypes()
	want := map[string]bool{
		"loopstart": true, "loopstop": true,
		"image": true, "code": true, "api": true,
	}
	if len(got) != len(want) {
		t.Fatalf("ExcludedFieldTypes len = %d, want %d (%v)", len(got), len(want), got)
	}
	for _, ty := range got {
		if !want[ty] {
			t.Errorf("ExcludedFieldTypes contains unexpected %q", ty)
		}
	}
}
