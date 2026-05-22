package csv

import "testing"

func TestFormatValue_Boolean(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := FormatValue(tc.in, "boolean"); got != tc.want {
			t.Errorf("FormatValue(%v, boolean) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatValue_NumberRange(t *testing.T) {
	for _, ty := range []string{"number", "range"} {
		// Integer-shaped floats lose the trailing ".0"
		if got := FormatValue(float64(42), ty); got != "42" {
			t.Errorf("FormatValue(42, %s) = %q, want '42'", ty, got)
		}
		if got := FormatValue(3.14, ty); got != "3.14" {
			t.Errorf("FormatValue(3.14, %s) = %q", ty, got)
		}
		if got := FormatValue(int64(7), ty); got != "7" {
			t.Errorf("FormatValue(int64 7, %s) = %q", ty, got)
		}
		if got := FormatValue(nil, ty); got != "" {
			t.Errorf("FormatValue(nil, %s) = %q, want empty", ty, got)
		}
	}
}

func TestFormatValue_TagsListMultioption_Array(t *testing.T) {
	in := []any{"a", "b", "c"}
	want := `["a","b","c"]`
	for _, ty := range []string{"tags", "list", "multioption"} {
		if got := FormatValue(in, ty); got != want {
			t.Errorf("FormatValue([], %s) = %q, want %q", ty, got, want)
		}
	}
}

func TestFormatValue_TagsListMultioption_ScalarFallback(t *testing.T) {
	// Stored as a bare string (legacy data) - return as-is.
	for _, ty := range []string{"tags", "list", "multioption"} {
		if got := FormatValue("hello", ty); got != "hello" {
			t.Errorf("FormatValue(scalar, %s) = %q", ty, got)
		}
	}
}

func TestFormatValue_Table(t *testing.T) {
	in := []any{[]any{"a", "b"}, []any{"c", "d"}}
	want := `[["a","b"],["c","d"]]`
	if got := FormatValue(in, "table"); got != want {
		t.Errorf("FormatValue(table) = %q, want %q", got, want)
	}
	// Non-array → ""
	if got := FormatValue("not an array", "table"); got != "" {
		t.Errorf("FormatValue(bad table) = %q, want empty", got)
	}
	if got := FormatValue(nil, "table"); got != "" {
		t.Errorf("FormatValue(nil table) = %q, want empty", got)
	}
}

func TestFormatValue_Default(t *testing.T) {
	if got := FormatValue("hello", "text"); got != "hello" {
		t.Errorf("FormatValue(text) = %q", got)
	}
	if got := FormatValue(nil, "text"); got != "" {
		t.Errorf("FormatValue(nil text) = %q", got)
	}
	if got := FormatValue(float64(5), "text"); got != "5" {
		t.Errorf("FormatValue(num as text) = %q", got)
	}
}
