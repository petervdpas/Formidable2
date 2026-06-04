package template

import "testing"

// ── missing-key (every field needs a key) ───────────────────────────

func TestValidate_MissingFieldKeyFlagged(t *testing.T) {
	tpl := &Template{Fields: []Field{
		{Key: "ok", Type: "text"},
		{Key: "", Type: "number"},
	}}
	if errs := Validate(tpl); !hasErr(errs, "missing-field-key") {
		t.Errorf("expected missing-field-key; got %+v", errs)
	}
}

func TestValidate_GuidEmptyKeyNotFlagged(t *testing.T) {
	// guid is auto-keyed to "id" by Normalize, so an empty key is fine here.
	tpl := &Template{Fields: []Field{{Key: "", Type: "guid"}}}
	if errs := Validate(tpl); hasErr(errs, "missing-field-key") {
		t.Errorf("guid empty key must not be flagged; got %+v", errs)
	}
}

// ── ValidateFieldDraft: candidate-scoped validation for the editor ───

func draftTpl() *Template {
	return &Template{
		Facets:   []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `1`}},
		Fields: []Field{
			{Key: "title", Type: "text"},
			{Key: "out", Type: "number"},
		},
	}
}

func TestValidateFieldDraft_CleanNewFieldHasNoErrors(t *testing.T) {
	got := ValidateFieldDraft(draftTpl(), Field{Key: "notes", Type: "text"}, "", true)
	if len(got) != 0 {
		t.Errorf("a clean new field must validate; got %+v", got)
	}
}

func TestValidateFieldDraft_MissingKeyBlocks(t *testing.T) {
	got := ValidateFieldDraft(draftTpl(), Field{Key: "", Type: "text"}, "", true)
	if !hasErr(got, "missing-field-key") {
		t.Errorf("a keyless candidate must report missing-field-key; got %+v", got)
	}
}

func TestValidateFieldDraft_DuplicateKeyBlocks(t *testing.T) {
	// New field reusing an existing key.
	got := ValidateFieldDraft(draftTpl(), Field{Key: "title", Type: "text"}, "", true)
	if !hasErr(got, "duplicate-keys") {
		t.Errorf("a duplicate key must be reported; got %+v", got)
	}
}

func TestValidateFieldDraft_EditKeepingOwnKeyIsNotDuplicate(t *testing.T) {
	// Editing the existing "title" field, keeping its key, must not self-collide.
	got := ValidateFieldDraft(draftTpl(), Field{Key: "title", Type: "text", Label: "Changed"}, "title", false)
	if hasErr(got, "duplicate-keys") {
		t.Errorf("editing a field keeping its key must not be a duplicate; got %+v", got)
	}
}

func TestValidateFieldDraft_FormulaBadBindingBlocks(t *testing.T) {
	got := ValidateFieldDraft(draftTpl(),
		Field{Key: "calc", Type: "formula", FormulaKey: "ghost", TargetKey: "out", Trigger: "save"}, "", true)
	if !hasErr(got, "formula-field-unknown-source") {
		t.Errorf("unknown formula source must be reported; got %+v", got)
	}
}

func TestValidateFieldDraft_IgnoresUnrelatedPreexistingErrors(t *testing.T) {
	// Template already has a problem on another field (duplicate keys among
	// siblings); a clean candidate must not inherit it.
	tpl := &Template{Fields: []Field{
		{Key: "dup", Type: "text"},
		{Key: "dup", Type: "text"},
	}}
	got := ValidateFieldDraft(tpl, Field{Key: "fresh", Type: "text"}, "", true)
	if len(got) != 0 {
		t.Errorf("candidate must not inherit unrelated pre-existing errors; got %+v", got)
	}
}
