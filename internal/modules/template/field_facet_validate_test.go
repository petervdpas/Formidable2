package template

import "testing"

// ── Facet field validation ──────────────────────────────────────────

func TestValidate_FacetFieldMissingBinding(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "status_inline", Type: "facet"}},
	}
	errs := Validate(tpl)
	if !hasErr(errs, "facet-field-missing-key") {
		t.Errorf("expected facet-field-missing-key; got %+v", errs)
	}
}

func TestValidate_FacetFieldUnknownBinding(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "ghost"}},
	}
	errs := Validate(tpl)
	if !hasErr(errs, "facet-field-unknown-key") {
		t.Errorf("expected facet-field-unknown-key; got %+v", errs)
	}
}

func TestValidate_FacetFieldBadFormat(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "status", Format: "toggle"}},
	}
	errs := Validate(tpl)
	if !hasErr(errs, "facet-field-bad-format") {
		t.Errorf("expected facet-field-bad-format; got %+v", errs)
	}
}

func TestValidate_FacetFieldHappyPath(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{
			{Key: "title", Type: "text"},
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
		},
	}
	errs := Validate(tpl)
	for _, e := range errs {
		t.Errorf("happy path must validate clean; got %+v", e)
	}
}

func TestValidate_FacetFieldDropdownFormatAccepted(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "status", Format: "dropdown"}},
	}
	errs := Validate(tpl)
	if hasErr(errs, "facet-field-bad-format") {
		t.Errorf("dropdown format must be accepted; got %+v", errs)
	}
}

func TestValidate_FacetFieldEmptyFormatAccepted(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "status"}},
	}
	errs := Validate(tpl)
	if hasErr(errs, "facet-field-bad-format") {
		t.Errorf("empty format must be accepted (defaults to radio); got %+v", errs)
	}
}

// ── facet_key attribute is forbidden on non-facet types ─────────────

func TestValidate_ForbiddenFacetKeyOnText(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "x", Type: "text", FacetKey: "status"}},
	})
	if !hasForbidden(errs, "x", "facet_key") {
		t.Errorf("expected forbidden-attribute(facet_key) on text; got %+v", errs)
	}
}

// ── Normalize: facet field Format coercion ──────────────────────────

func TestNormalize_FacetMissingFormatDefaultsToRadio(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "s", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "s"}},
	}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "radio" {
		t.Errorf("Format = %q, want radio", got)
	}
}

func TestNormalize_FacetUnknownFormatFallsBackToRadio(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "s", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "s", Format: "toggle"}},
	}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "radio" {
		t.Errorf("Format = %q, want radio", got)
	}
}

func TestNormalize_FacetDropdownLowercased(t *testing.T) {
	tpl := &Template{
		Facets: []Facet{{Key: "s", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Fields: []Field{{Key: "f", Type: "facet", FacetKey: "s", Format: "DROPDOWN"}},
	}
	Normalize(tpl)
	if got := tpl.Fields[0].Format; got != "dropdown" {
		t.Errorf("Format = %q, want dropdown", got)
	}
}

func TestNormalize_FacetKeyStrippedFromNonFacetField(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "x", Type: "text", FacetKey: "status"}},
	}
	Normalize(tpl)
	if got := tpl.Fields[0].FacetKey; got != "" {
		t.Errorf("FacetKey on text must be stripped; got %q", got)
	}
}
