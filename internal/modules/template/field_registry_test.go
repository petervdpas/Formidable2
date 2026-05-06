package template

import "testing"

// ── unknown type ─────────────────────────────────────────────────────

func TestValidate_UnknownTypeFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "x", Type: "wat"},
		},
	})
	if !hasErr(errs, "unknown-field-type") {
		t.Errorf("expected unknown-field-type; got %+v", errs)
	}
}

func TestValidate_KnownTypesNotFlaggedAsUnknown(t *testing.T) {
	known := []string{
		"guid", "loopstart", "loopstop", "looper",
		"text", "boolean", "dropdown", "multioption", "radio",
		"textarea", "latex", "number", "range", "date",
		"list", "table", "image", "link", "tags", "code", "api",
	}
	for _, ty := range known {
		f := Field{Key: "k", Type: ty}
		// api needs Collection or it'll trip the api-collection-required
		// rule — set it so we only test the unknown-type path here.
		if ty == "api" {
			f.Collection = "c"
		}
		// Loopstart needs a matching loopstop to pass loop pairing.
		fields := []Field{f}
		if ty == "loopstart" {
			fields = append(fields, Field{Key: "k", Type: "loopstop"})
		}
		errs := Validate(&Template{Fields: fields})
		if hasErr(errs, "unknown-field-type") {
			t.Errorf("type %q should be known; got %+v", ty, errs)
		}
	}
}

// ── forbidden attributes per type ────────────────────────────────────

func TestValidate_ForbiddenFormatOnNonTextarea(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "n", Type: "number", Format: "markdown"}},
	})
	if !hasForbidden(errs, "n", "format") {
		t.Errorf("expected forbidden-attribute(format) on number; got %+v", errs)
	}
}

func TestValidate_ForbiddenCodeGroupOnText(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "x", Type: "text", RunMode: "manual"}},
	})
	if !hasForbidden(errs, "x", "code") {
		t.Errorf("expected forbidden-attribute(code) on text; got %+v", errs)
	}
}

func TestValidate_ForbiddenLatexGroupOnText(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "x", Type: "text", UseFenced: boolPtr(true)}},
	})
	if !hasForbidden(errs, "x", "latex") {
		t.Errorf("expected forbidden-attribute(latex) on text; got %+v", errs)
	}
}

func TestValidate_ForbiddenApiGroupOnText(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "x", Type: "text", Collection: "c"}},
	})
	if !hasForbidden(errs, "x", "api") {
		t.Errorf("expected forbidden-attribute(api) on text; got %+v", errs)
	}
}

func TestValidate_ForbiddenCollapsibleOnGuid(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "g", Type: "guid", Collapsible: boolPtr(true)}},
	})
	if !hasForbidden(errs, "g", "collapsible") {
		t.Errorf("expected forbidden-attribute(collapsible) on guid; got %+v", errs)
	}
}

func TestValidate_ForbiddenReadonlyOnBoolean(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "b", Type: "boolean", Readonly: true}},
	})
	if !hasForbidden(errs, "b", "readonly") {
		t.Errorf("expected forbidden-attribute(readonly) on boolean; got %+v", errs)
	}
}

func TestValidate_ForbiddenSummaryFieldOnNonLoopstart(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "x", Type: "text", SummaryField: "name"}},
	})
	if !hasForbidden(errs, "x", "summary_field") {
		t.Errorf("expected forbidden-attribute(summary_field) on text; got %+v", errs)
	}
}

// ── happy paths (no false positives) ─────────────────────────────────

func TestValidate_LoopstartWithSummaryField_OK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "items", Type: "loopstart", SummaryField: "name"},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop"},
		},
	})
	if anyForbiddenFor(errs, "items") {
		t.Errorf("loopstart.summary_field is allowed; got %+v", errs)
	}
}

func TestValidate_TextareaWithFormat_OK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "body", Type: "textarea", Format: "markdown"}},
	})
	if anyForbiddenFor(errs, "body") {
		t.Errorf("textarea.format is allowed; got %+v", errs)
	}
}

func TestValidate_CodeWithRunMode_OK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "c", Type: "code", RunMode: "manual"}},
	})
	if anyForbiddenFor(errs, "c") {
		t.Errorf("code.run_mode is allowed; got %+v", errs)
	}
}

func TestValidate_LatexWithRows_OK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "l", Type: "latex", Rows: intPtr(8), UseFenced: boolPtr(true)}},
	})
	if anyForbiddenFor(errs, "l") {
		t.Errorf("latex.rows + use_fenced are allowed; got %+v", errs)
	}
}

func TestValidate_ApiWithMap_OK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{
			Key: "a", Type: "api", Collection: "c",
			Map: []APIMap{{Key: "k"}}, UsePicker: boolPtr(true),
		}},
	})
	if anyForbiddenFor(errs, "a") {
		t.Errorf("api fields are allowed their group; got %+v", errs)
	}
}

func TestValidate_BasicSeedTemplatePasses(t *testing.T) {
	errs := Validate(basicTemplate())
	for _, e := range errs {
		t.Errorf("seed should validate cleanly; got %+v", e)
	}
}

func TestValidate_ListAndTableAllowCollapsible(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "li", Type: "list", Collapsible: boolPtr(true)},
			{Key: "tb", Type: "table", Collapsible: boolPtr(true),
				Options: []any{map[string]any{"value": "c1", "label": "C1"}}},
		},
	})
	if anyForbiddenFor(errs, "li") || anyForbiddenFor(errs, "tb") {
		t.Errorf("collapsible is allowed on list/table; got %+v", errs)
	}
}

// ── public registry surface ──────────────────────────────────────────

func TestAllFieldTypes_StableOrderIncludesEveryRegistryEntry(t *testing.T) {
	got := AllFieldTypes()
	if len(got) != len(fieldTypeRegistry) {
		t.Fatalf("AllFieldTypes returned %d entries; registry has %d",
			len(got), len(fieldTypeRegistry))
	}
	got2 := AllFieldTypes()
	for i := range got {
		if got[i].ID != got2[i].ID {
			t.Errorf("non-deterministic order at i=%d: %q vs %q",
				i, got[i].ID, got2[i].ID)
		}
	}
}

func TestAllFieldTypes_ReturnsDefensiveCopy(t *testing.T) {
	got := AllFieldTypes()
	if len(got) == 0 {
		t.Fatal("empty registry")
	}
	got[0].ForbiddenAttributes = append(got[0].ForbiddenAttributes, "tampered")
	fresh := AllFieldTypes()
	for _, a := range fresh[0].ForbiddenAttributes {
		if a == "tampered" {
			t.Errorf("registry was mutated through returned slice")
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func hasErr(errs []ValidationError, kind string) bool {
	for _, e := range errs {
		if e.Type == kind {
			return true
		}
	}
	return false
}

func hasForbidden(errs []ValidationError, key, attr string) bool {
	for _, e := range errs {
		if e.Type != "forbidden-attribute" {
			continue
		}
		if e.Key != key {
			continue
		}
		if e.Detail == nil {
			continue
		}
		if got, _ := e.Detail["attr"].(string); got == attr {
			return true
		}
	}
	return false
}

func anyForbiddenFor(errs []ValidationError, key string) bool {
	for _, e := range errs {
		if e.Type == "forbidden-attribute" && e.Key == key {
			return true
		}
	}
	return false
}
