package template

import (
	"strings"
	"testing"
)

func TestFacetColors_ContainsAll16Tokens(t *testing.T) {
	want := []string{
		"red", "orange", "amber", "yellow", "green", "teal",
		"blue", "purple", "pink", "gray",
		"cyan", "lime", "indigo", "rose", "brown", "slate",
	}
	if len(FacetColors) != len(want) {
		t.Fatalf("len(FacetColors) = %d, want %d", len(FacetColors), len(want))
	}
	for _, c := range want {
		if !IsKnownFacetColor(c) {
			t.Errorf("FacetColors missing %q", c)
		}
	}
}

func TestValidate_Facets_EmptyIsFine(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_Facets_FiveIsFine(t *testing.T) {
	facets := make([]Facet, 0, 5)
	for i := 0; i < 5; i++ {
		facets = append(facets, Facet{
			Key:     "facet_" + asTwoDigit(i),
			Icon:    "fa-flag",
			Options: []FacetOption{{Label: padLabel("F", i), Color: "red"}},
		})
	}
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: facets,
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", summarizeErrors(errs))
	}
}

func TestValidate_Facets_SixIsTooMany(t *testing.T) {
	facets := make([]Facet, 0, 6)
	for i := 0; i < 6; i++ {
		facets = append(facets, Facet{
			Key:     "facet_" + asTwoDigit(i),
			Icon:    "fa-flag",
			Options: []FacetOption{{Label: padLabel("F", i), Color: "red"}},
		})
	}
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: facets,
	}
	if !hasErrType(Validate(tmpl), "too-many-facets") {
		t.Fatalf("expected too-many-facets, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_UnknownIconRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{
			Key:     "status",
			Icon:    "fa-rocket",
			Options: []FacetOption{{Label: "OPEN", Color: "red"}},
		}},
	}
	if !hasErrType(Validate(tmpl), "unknown-facet-icon") {
		t.Fatalf("expected unknown-facet-icon, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestGetFacetMeta_FullContract(t *testing.T) {
	m := GetFacetMeta()
	if m.MaxFacets != MaxFacets {
		t.Errorf("MaxFacets = %d, want %d", m.MaxFacets, MaxFacets)
	}
	if m.MaxOptionsPerFacet != MaxOptionsPerFacet {
		t.Errorf("MaxOptionsPerFacet = %d, want %d", m.MaxOptionsPerFacet, MaxOptionsPerFacet)
	}
	if len(m.Colors) != len(FacetColorList) {
		t.Errorf("Colors len = %d, want %d", len(m.Colors), len(FacetColorList))
	}
	for i, c := range FacetColorList {
		if m.Colors[i] != c {
			t.Errorf("Colors[%d] = %q, want %q (display order matters)", i, m.Colors[i], c)
		}
	}
	if len(m.Icons) != len(FacetIconList) {
		t.Errorf("Icons len = %d, want %d", len(m.Icons), len(FacetIconList))
	}
	for i, ic := range FacetIconList {
		if m.Icons[i] != ic {
			t.Errorf("Icons[%d] = %q, want %q (display order matters)", i, m.Icons[i], ic)
		}
	}
	if m.KeyPattern != FacetKeyPattern {
		t.Errorf("KeyPattern = %q, want %q", m.KeyPattern, FacetKeyPattern)
	}
	if m.LabelPattern != FacetLabelPattern {
		t.Errorf("LabelPattern = %q, want %q", m.LabelPattern, FacetLabelPattern)
	}
	// IconSVGs must carry one spec for every key in the icon palette
	// — the frontend reads this once at boot and renders inline SVG
	// for every facet UI without a second round-trip.
	if len(m.IconSVGs) != len(FacetIconList) {
		t.Errorf("IconSVGs len = %d, want %d", len(m.IconSVGs), len(FacetIconList))
	}
	for _, key := range FacetIconList {
		spec, ok := m.IconSVGs[key]
		if !ok {
			t.Errorf("IconSVGs missing %q", key)
			continue
		}
		if spec.ViewBox == "" || spec.Path == "" {
			t.Errorf("IconSVGs[%q] is incomplete: %#v", key, spec)
		}
	}
}

func TestGetFacetMeta_ReturnsCopies(t *testing.T) {
	m := GetFacetMeta()
	m.Colors[0] = "MUTATED"
	if FacetColorList[0] == "MUTATED" {
		t.Errorf("FacetColorList was mutated through returned snapshot")
	}
	// IconSVGs is a fresh map — mutating it must not poison the
	// package-level catalog.
	flagBefore := FacetIconSVGs["fa-flag"]
	m.IconSVGs["fa-flag"] = FacetIconSpec{ViewBox: "MUTATED", Path: "X"}
	if FacetIconSVGs["fa-flag"] != flagBefore {
		t.Errorf("FacetIconSVGs mutated through returned snapshot")
	}
}

func TestFacetIcons_ContainsAll16(t *testing.T) {
	want := []string{
		"fa-flag", "fa-check", "fa-star", "fa-heart",
		"fa-bookmark", "fa-bell", "fa-shirt", "fa-circle-info",
		"fa-triangle-exclamation", "fa-circle-question", "fa-user", "fa-clock",
		"fa-tag", "fa-bug", "fa-gear", "fa-fire",
	}
	if len(FacetIcons) != len(want) {
		t.Fatalf("len(FacetIcons) = %d, want %d", len(FacetIcons), len(want))
	}
	for _, icon := range want {
		if !IsKnownFacetIcon(icon) {
			t.Errorf("FacetIcons missing %q", icon)
		}
	}
}

func TestValidate_Facets_DuplicateKeyRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{
			{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "red"}}},
			{Key: "status", Icon: "fa-check", Options: []FacetOption{{Label: "DONE", Color: "green"}}},
		},
	}
	if !hasErrType(Validate(tmpl), "duplicate-facet-key") {
		t.Fatalf("expected duplicate-facet-key, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_KeyMustBeSlug(t *testing.T) {
	cases := []struct {
		key string
		ok  bool
	}{
		{"status", true},
		{"task_status", true},
		{"task-status", true},
		{"size9", true},
		{"Status", false},      // uppercase
		{"9status", false},     // leading digit
		{"_status", false},     // leading underscore
		{"status!", false},     // bad char
		{"", false},            // empty
		{"status name", false}, // space
	}
	for _, tc := range cases {
		tmpl := &Template{
			Name: "T", Filename: "t.yaml",
			Fields: []Field{{Key: "title", Type: "text"}},
			Facets: []Facet{{Key: tc.key, Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "red"}}}},
		}
		errs := Validate(tmpl)
		got := !hasErrType(errs, "invalid-facet-key")
		if got != tc.ok {
			t.Errorf("key %q: ok=%v, want %v (errs=%v)", tc.key, got, tc.ok, summarizeErrors(errs))
		}
	}
}

func TestValidate_Facets_IconRequired(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{Key: "status", Icon: "", Options: []FacetOption{{Label: "OPEN", Color: "red"}}}},
	}
	if !hasErrType(Validate(tmpl), "missing-facet-icon") {
		t.Fatalf("expected missing-facet-icon, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_OptionsRequired(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: nil}},
	}
	if !hasErrType(Validate(tmpl), "empty-facet-options") {
		t.Fatalf("expected empty-facet-options, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_TooManyOptions(t *testing.T) {
	opts := make([]FacetOption, 0, 17)
	for i := 0; i < 17; i++ {
		opts = append(opts, FacetOption{Label: padLabel("O", i), Color: "red"})
	}
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: opts}},
	}
	if !hasErrType(Validate(tmpl), "too-many-facet-options") {
		t.Fatalf("expected too-many-facet-options, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_DuplicateLabelWithinFacetRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []FacetOption{
				{Label: "OPEN", Color: "red"},
				{Label: "OPEN", Color: "blue"},
			},
		}},
	}
	if !hasErrType(Validate(tmpl), "duplicate-facet-label") {
		t.Fatalf("expected duplicate-facet-label, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_DuplicateLabelAcrossFacetsOK(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{
			{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "DONE", Color: "red"}}},
			{Key: "review", Icon: "fa-user", Options: []FacetOption{{Label: "DONE", Color: "green"}}},
		},
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("labels may repeat across facets, got %v", summarizeErrors(errs))
	}
}

func TestValidate_Facets_LabelMustBeUpper(t *testing.T) {
	cases := []struct {
		label string
		ok    bool
	}{
		{"FLASH", true},
		{"NO FLAG", true},
		{"FLASH-1", true},
		{"FLAG_X", true},
		{"FLAG2", true},
		{"flash", false},
		{"Flash", false},
		{"1FLASH", false},
		{"FLASH!", false},
		{"", false},
		{" FLASH", false},
	}
	for _, tc := range cases {
		tmpl := &Template{
			Name: "T", Filename: "t.yaml",
			Fields: []Field{{Key: "title", Type: "text"}},
			Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: tc.label, Color: "red"}}}},
		}
		errs := Validate(tmpl)
		got := !hasErrType(errs, "invalid-facet-label")
		if got != tc.ok {
			t.Errorf("label %q: ok=%v, want %v (errs=%v)", tc.label, got, tc.ok, summarizeErrors(errs))
		}
	}
}

func TestValidate_Facets_ColorMustBeKnown(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "FLASH", Color: "crimson"}}}},
	}
	if !hasErrType(Validate(tmpl), "unknown-facet-color") {
		t.Fatalf("expected unknown-facet-color, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_ColorEmptyRejected(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "FLASH", Color: ""}}}},
	}
	if !hasErrType(Validate(tmpl), "unknown-facet-color") {
		t.Fatalf("expected unknown-facet-color, got %v", summarizeErrors(Validate(tmpl)))
	}
}

func TestValidate_Facets_ColorsMayRepeat(t *testing.T) {
	tmpl := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []FacetOption{
				{Label: "FLASH", Color: "red"},
				{Label: "URGENT", Color: "red"},
			},
		}},
	}
	if errs := Validate(tmpl); len(errs) != 0 {
		t.Fatalf("colors may repeat within a facet, got %v", summarizeErrors(errs))
	}
}

func TestFacets_YAMLRoundTrip(t *testing.T) {
	src := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
		Facets: []Facet{
			{
				Key:  "status",
				Icon: "fa-flag",
				Options: []FacetOption{
					{Label: "FLASH", Color: "red"},
					{Label: "IMMEDIATE", Color: "orange"},
				},
			},
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
	if len(got.Facets) != 1 || got.Facets[0].Key != "status" {
		t.Fatalf("facets = %+v, want one facet keyed 'status'", got.Facets)
	}
	if len(got.Facets[0].Options) != 2 {
		t.Fatalf("options len = %d, want 2", len(got.Facets[0].Options))
	}
	if got.Facets[0].Options[0] != src.Facets[0].Options[0] {
		t.Errorf("[0] = %+v, want %+v", got.Facets[0].Options[0], src.Facets[0].Options[0])
	}
	if got.Facets[0].Options[1] != src.Facets[0].Options[1] {
		t.Errorf("[1] = %+v, want %+v", got.Facets[0].Options[1], src.Facets[0].Options[1])
	}
}

func TestFacets_YAMLOmitsEmpty(t *testing.T) {
	src := &Template{
		Name: "T", Filename: "t.yaml",
		Fields: []Field{{Key: "title", Type: "text"}},
	}
	b, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "facets") {
		t.Errorf("empty Facets should not appear in YAML, got:\n%s", b)
	}
}

func TestFacets_LegacyFlagDefinitionsMigrate(t *testing.T) {
	legacy := []byte(`
name: T
filename: t.yaml
fields:
  - key: title
    type: text
flag_definitions:
  - label: NOT IN USE
    color: red
  - label: IN USE
    color: green
`)
	var got Template
	if err := unmarshalYAML(legacy, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Facets) != 1 {
		t.Fatalf("facets len = %d, want 1 synthetic facet", len(got.Facets))
	}
	f := got.Facets[0]
	if f.Key != "flag" || f.Icon != "fa-flag" {
		t.Errorf("synthetic facet = {Key:%q, Icon:%q}, want {flag, fa-flag}", f.Key, f.Icon)
	}
	if len(f.Options) != 2 {
		t.Fatalf("options len = %d, want 2", len(f.Options))
	}
	if f.Options[0] != (FacetOption{Label: "NOT IN USE", Color: "red"}) {
		t.Errorf("[0] = %+v, want NOT IN USE/red", f.Options[0])
	}
	if f.Options[1] != (FacetOption{Label: "IN USE", Color: "green"}) {
		t.Errorf("[1] = %+v, want IN USE/green", f.Options[1])
	}
}

func TestFacets_NewShapeWinsOverLegacy(t *testing.T) {
	mixed := []byte(`
name: T
filename: t.yaml
fields:
  - key: title
    type: text
facets:
  - key: status
    icon: fa-heart
    options:
      - label: ALPHA
        color: blue
flag_definitions:
  - label: SHOULD_IGNORE
    color: red
`)
	var got Template
	if err := unmarshalYAML(mixed, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Facets) != 1 || got.Facets[0].Key != "status" {
		t.Fatalf("new shape should win; got %+v", got.Facets)
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

func padLabel(prefix string, n int) string {
	if n < 10 {
		return prefix + "0" + string(rune('0'+n))
	}
	return prefix + string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func asTwoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
