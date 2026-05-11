package template

import (
	"strings"
	"testing"
)

func TestFlagColors_ContainsAll16Tokens(t *testing.T) {
	want := []string{
		"red", "orange", "amber", "yellow", "green", "teal",
		"blue", "purple", "pink", "gray",
		"cyan", "lime", "indigo", "rose", "brown", "slate",
	}
	if len(FlagColors) != len(want) {
		t.Fatalf("len(FlagColors) = %d, want %d", len(FlagColors), len(want))
	}
	for _, c := range want {
		if _, ok := FlagColors[c]; !ok {
			t.Errorf("FlagColors missing %q", c)
		}
	}
}

func TestValidate_FlagDefinitions_EmptyIsFine(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FlagDefinitions_SixteenIsFine(t *testing.T) {
	defs := make([]FlagDefinition, 0, 16)
	for i := 0; i < 16; i++ {
		defs = append(defs, FlagDefinition{Label: pad("F", i), Color: "red"})
	}
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields:          []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: defs,
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_FlagDefinitions_SeventeenIsTooMany(t *testing.T) {
	defs := make([]FlagDefinition, 0, 17)
	for i := 0; i < 17; i++ {
		defs = append(defs, FlagDefinition{Label: pad("F", i), Color: "red"})
	}
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields:          []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: defs,
	}
	errs := Validate(tmpl)
	if !hasErrType(errs, "too-many-flag-definitions") {
		t.Fatalf("expected too-many-flag-definitions, got %v", summarizeErrors(errs))
	}
}

func TestValidate_FlagDefinitions_DuplicateLabelRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: []FlagDefinition{
			{Label: "FLASH", Color: "red"},
			{Label: "FLASH", Color: "blue"},
		},
	}
	errs := Validate(tmpl)
	if !hasErrType(errs, "duplicate-flag-label") {
		t.Fatalf("expected duplicate-flag-label, got %v", summarizeErrors(errs))
	}
}

func TestValidate_FlagDefinitions_LabelMustBeUpper(t *testing.T) {
	cases := []struct {
		label string
		ok    bool
	}{
		{"FLASH", true},
		{"NO FLAG", true},
		{"FLASH-1", true},
		{"FLAG_X", true},
		{"FLAG2", true},
		{"flash", false},        // lowercase
		{"Flash", false},        // mixed
		{"1FLASH", false},       // leading digit
		{"FLASH!", false},       // bad char
		{"", false},             // empty
		{" FLASH", false},       // leading space
	}
	for _, tc := range cases {
		tmpl := &Template{
			Name: "T", Filename: "t.yaml",
			Fields:          []Field{{Key: "title", Type: "text"}},
			FlagDefinitions: []FlagDefinition{{Label: tc.label, Color: "red"}},
		}
		errs := Validate(tmpl)
		got := !hasErrType(errs, "invalid-flag-label")
		if got != tc.ok {
			t.Errorf("label %q: ok=%v, want %v (errs=%v)", tc.label, got, tc.ok, summarizeErrors(errs))
		}
	}
}

func TestValidate_FlagDefinitions_ColorMustBeKnown(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields:          []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: []FlagDefinition{{Label: "FLASH", Color: "crimson"}},
	}
	errs := Validate(tmpl)
	if !hasErrType(errs, "unknown-flag-color") {
		t.Fatalf("expected unknown-flag-color, got %v", summarizeErrors(errs))
	}
}

func TestValidate_FlagDefinitions_ColorEmptyRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields:          []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: []FlagDefinition{{Label: "FLASH", Color: ""}},
	}
	errs := Validate(tmpl)
	if !hasErrType(errs, "unknown-flag-color") {
		t.Fatalf("expected unknown-flag-color, got %v", summarizeErrors(errs))
	}
}

func TestValidate_FlagDefinitions_ColorsMayRepeat(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: []FlagDefinition{
			{Label: "FLASH", Color: "red"},
			{Label: "URGENT", Color: "red"},
		},
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("colors may repeat across labels, got %v", summarizeErrors(errs))
	}
}

func TestFlagDefinitions_YAMLRoundTrip(t *testing.T) {
	src := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		FlagDefinitions: []FlagDefinition{
			{Label: "FLASH", Color: "red"},
			{Label: "IMMEDIATE", Color: "orange"},
		},
	}
	b, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Template
	if err := unmarshalYAML(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.FlagDefinitions) != 2 {
		t.Fatalf("flag_definitions len = %d, want 2", len(got.FlagDefinitions))
	}
	if got.FlagDefinitions[0] != src.FlagDefinitions[0] {
		t.Errorf("[0] = %+v, want %+v", got.FlagDefinitions[0], src.FlagDefinitions[0])
	}
	if got.FlagDefinitions[1] != src.FlagDefinitions[1] {
		t.Errorf("[1] = %+v, want %+v", got.FlagDefinitions[1], src.FlagDefinitions[1])
	}
}

func TestFlagDefinitions_YAMLOmitsEmpty(t *testing.T) {
	src := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
	}
	b, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "flag_definitions") {
		t.Errorf("empty FlagDefinitions should not appear in YAML, got:\n%s", b)
	}
}

func hasErrType(errs []ValidationError, want string) bool {
	for _, e := range errs {
		if e.Type == want {
			return true
		}
	}
	return false
}

func pad(prefix string, n int) string {
	// Build a unique uppercase label like "F00", "F01", ... that fits the regex.
	if n < 10 {
		return prefix + "0" + string(rune('0'+n))
	}
	return prefix + string(rune('0'+n/10)) + string(rune('0'+n%10))
}
