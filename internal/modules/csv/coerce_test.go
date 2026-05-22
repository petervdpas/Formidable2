package csv

import (
	"reflect"
	"testing"
)

// Helper to build a list of {value, label} option maps.
func opts(pairs ...[2]string) []any {
	out := make([]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{"value": p[0], "label": p[1]})
	}
	return out
}

func TestCoerce_Boolean(t *testing.T) {
	cases := map[string]bool{
		"true": true, "TRUE": true, "1": true, "yes": true, "YES": true, "on": true,
		"false": false, "0": false, "no": false, "off": false, "": false, "garbage": false,
	}
	for in, want := range cases {
		got := Coerce(in, "boolean", nil)
		if got != want {
			t.Errorf("Coerce(%q, boolean) = %v, want %v", in, got, want)
		}
	}
}

func TestCoerce_Number(t *testing.T) {
	if got := Coerce("42", "number", nil); got != float64(42) {
		t.Errorf("Coerce(42, number) = %v (%T), want 42", got, got)
	}
	if got := Coerce("-3.14", "number", nil); got != -3.14 {
		t.Errorf("Coerce(-3.14, number) = %v, want -3.14", got)
	}
	if got := Coerce("garbage", "number", nil); got != float64(0) {
		t.Errorf("Coerce(garbage, number) = %v, want 0", got)
	}
	if got := Coerce("", "number", nil); got != float64(0) {
		t.Errorf("Coerce('', number) = %v, want 0", got)
	}
}

func TestCoerce_Range(t *testing.T) {
	// Range falls back to 50, not 0, when value is non-numeric.
	if got := Coerce("garbage", "range", nil); got != float64(50) {
		t.Errorf("Coerce(garbage, range) = %v, want 50", got)
	}
	if got := Coerce("77", "range", nil); got != float64(77) {
		t.Errorf("Coerce(77, range) = %v, want 77", got)
	}
}

func TestCoerce_Date(t *testing.T) {
	if got := Coerce("2026-05-12", "date", nil); got != "2026-05-12" {
		t.Errorf("Coerce(date) = %v, want passthrough", got)
	}
	if got := Coerce("  ", "date", nil); got != "" {
		t.Errorf("Coerce(blank date) = %v, want empty", got)
	}
}

func TestCoerce_Dropdown(t *testing.T) {
	options := opts(
		[2]string{"us", "United States"},
		[2]string{"nl", "Netherlands"},
	)
	// Match by value
	if got := Coerce("us", "dropdown", options); got != "us" {
		t.Errorf("Coerce(us) = %v", got)
	}
	// Match by label (case-insensitive)
	if got := Coerce("netherlands", "dropdown", options); got != "nl" {
		t.Errorf("Coerce(netherlands) = %v, want 'nl'", got)
	}
	// No match → raw value
	if got := Coerce("germany", "dropdown", options); got != "germany" {
		t.Errorf("Coerce(germany) = %v, want passthrough", got)
	}
}

func TestCoerce_Radio_SameAsDropdown(t *testing.T) {
	options := opts([2]string{"a", "Alpha"}, [2]string{"b", "Beta"})
	if got := Coerce("alpha", "radio", options); got != "a" {
		t.Errorf("Coerce(radio) = %v, want 'a'", got)
	}
}

func TestCoerce_Multioption(t *testing.T) {
	options := opts([2]string{"a", "Alpha"}, [2]string{"b", "Beta"})
	got := Coerce("alpha,b", "multioption", options)
	want := []any{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(multioption) = %v, want %v", got, want)
	}
}

func TestCoerce_TagsList(t *testing.T) {
	// Comma-separated
	for _, ty := range []string{"tags", "list"} {
		got := Coerce("foo, bar, baz", ty, nil)
		want := []any{"foo", "bar", "baz"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Coerce(%s, comma) = %v, want %v", ty, got, want)
		}
	}
	// JSON array also accepted
	got := Coerce(`["x","y"]`, "tags", nil)
	want := []any{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(tags, json) = %v, want %v", got, want)
	}
}

func TestCoerce_Table_ValidJSON(t *testing.T) {
	got := Coerce(`[["a","b"],["c","d"]]`, "table", nil)
	want := []any{[]any{"a", "b"}, []any{"c", "d"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(table) = %v, want %v", got, want)
	}
}

func TestCoerce_Table_MalformedFallsToEmpty(t *testing.T) {
	got := Coerce("not json", "table", nil)
	want := []any{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(bad table) = %v, want []", got)
	}
	got = Coerce("", "table", nil)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(empty table) = %v, want []", got)
	}
	// JSON but not an array → []
	got = Coerce(`{"k":"v"}`, "table", nil)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Coerce(object as table) = %v, want []", got)
	}
}

func TestCoerce_DefaultPassthrough(t *testing.T) {
	if got := Coerce("hello", "text", nil); got != "hello" {
		t.Errorf("Coerce(text) = %v, want passthrough", got)
	}
	// Default trims
	if got := Coerce("  hi  ", "text", nil); got != "hi" {
		t.Errorf("Coerce(text trim) = %v, want 'hi'", got)
	}
}

func TestCoercePreview_Boolean(t *testing.T) {
	if got := CoercePreview("yes", "boolean", nil); got != "true" {
		t.Errorf("CoercePreview(yes) = %q", got)
	}
	if got := CoercePreview("nope", "boolean", nil); got != "false" {
		t.Errorf("CoercePreview(nope) = %q", got)
	}
}

func TestCoercePreview_Multioption_JoinsCommaSpace(t *testing.T) {
	options := opts([2]string{"a", "Alpha"}, [2]string{"b", "Beta"})
	if got := CoercePreview("alpha,b", "multioption", options); got != "a, b" {
		t.Errorf("CoercePreview(multioption) = %q, want 'a, b'", got)
	}
}

func TestCoercePreview_TagsList(t *testing.T) {
	if got := CoercePreview("a, b, c", "tags", nil); got != "a, b, c" {
		t.Errorf("CoercePreview(tags) = %q", got)
	}
}

func TestCoercePreview_NumberInvalidShowsFallback(t *testing.T) {
	if got := CoercePreview("foo", "number", nil); got != "0" {
		t.Errorf("CoercePreview(bad number) = %q", got)
	}
	if got := CoercePreview("foo", "range", nil); got != "50" {
		t.Errorf("CoercePreview(bad range) = %q", got)
	}
	if got := CoercePreview("12.5", "number", nil); got != "12.5" {
		t.Errorf("CoercePreview(12.5) = %q", got)
	}
}

// matchOption and parseAsList are internal helpers - tested through Coerce
// above, but a couple of focused unit checks keep regressions obvious.

func TestMatchOption_ScalarStringOption(t *testing.T) {
	// Some templates store options as bare strings rather than {value,label}.
	options := []any{"alpha", "beta"}
	if got := matchOption("ALPHA", options); got != "alpha" {
		t.Errorf("matchOption(scalar) = %v, want 'alpha'", got)
	}
}

func TestMatchOption_EmptyOptionsReturnsRaw(t *testing.T) {
	if got := matchOption("anything", nil); got != "anything" {
		t.Errorf("matchOption(nil opts) = %v", got)
	}
}

func TestParseAsList_JSONArrayPreferred(t *testing.T) {
	got := parseAsList(`["x", "y"]`)
	want := []any{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseAsList(json) = %v", got)
	}
}

func TestParseAsList_CommaSemicolonPipe(t *testing.T) {
	for _, in := range []string{"a,b,c", "a;b;c", "a|b|c", "a, b ; c"} {
		got := parseAsList(in)
		want := []any{"a", "b", "c"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseAsList(%q) = %v", in, got)
		}
	}
}

func TestParseAsList_EmptyIsEmpty(t *testing.T) {
	if got := parseAsList(""); len(got) != 0 {
		t.Errorf("parseAsList('') = %v, want []", got)
	}
}
