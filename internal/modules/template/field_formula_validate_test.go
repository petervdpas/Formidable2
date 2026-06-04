package template

import "testing"

// ── Formula field validation (virtual: writes a formula's output into a target data field) ──

func formulaTpl(fields ...Field) *Template {
	return &Template{
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `F["a"] + F["b"]`}},
		Fields:   fields,
	}
}

func TestValidate_FormulaFieldMissingSource(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", TargetKey: "out", Trigger: "save"},
	)
	if errs := Validate(tpl); !hasErr(errs, "formula-field-missing-source") {
		t.Errorf("expected formula-field-missing-source; got %+v", errs)
	}
}

func TestValidate_FormulaFieldUnknownSource(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "ghost", TargetKey: "out", Trigger: "save"},
	)
	if errs := Validate(tpl); !hasErr(errs, "formula-field-unknown-source") {
		t.Errorf("expected formula-field-unknown-source; got %+v", errs)
	}
}

func TestValidate_FormulaFieldMissingTarget(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "f", Type: "formula", FormulaKey: "total", Trigger: "save"},
	)
	if errs := Validate(tpl); !hasErr(errs, "formula-field-missing-target") {
		t.Errorf("expected formula-field-missing-target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldUnknownTarget(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "ghost", Trigger: "save"},
	)
	if errs := Validate(tpl); !hasErr(errs, "formula-field-unknown-target") {
		t.Errorf("expected formula-field-unknown-target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldTargetCannotBeVirtual(t *testing.T) {
	tpl := &Template{
		Facets:   []Facet{{Key: "status", Icon: "fa-flag", Options: []FacetOption{{Label: "OPEN", Color: "blue"}}}},
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `1`}},
		Fields: []Field{
			{Key: "s", Type: "facet", FacetKey: "status", Default: "OPEN"},
			{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "s", Trigger: "save"},
		},
	}
	// A virtual field has no data slot, so it can't be a formula target.
	if errs := Validate(tpl); !hasErr(errs, "formula-field-unknown-target") {
		t.Errorf("expected formula-field-unknown-target for a virtual target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldBadTrigger(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "later"},
	)
	if errs := Validate(tpl); !hasErr(errs, "formula-field-bad-trigger") {
		t.Errorf("expected formula-field-bad-trigger; got %+v", errs)
	}
}

func TestValidate_FormulaFieldEmptyTriggerAccepted(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out"},
	)
	// Empty trigger is accepted: Normalize coerces it to "save".
	if errs := Validate(tpl); hasErr(errs, "formula-field-bad-trigger") {
		t.Errorf("empty trigger must be accepted; got %+v", errs)
	}
}

func TestValidate_FormulaFieldIncompatibleTarget(t *testing.T) {
	tpl := &Template{
		Formulas: []Formula{{Key: "name", Type: "text", Expression: `F["a"]`}},
		Fields: []Field{
			{Key: "n", Type: "number"},
			{Key: "f", Type: "formula", FormulaKey: "name", TargetKey: "n", Trigger: "save"},
		},
	}
	// A text formula cannot be written into a number field.
	if errs := Validate(tpl); !hasErr(errs, "formula-field-incompatible-target") {
		t.Errorf("expected formula-field-incompatible-target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldCompatibleTextTarget(t *testing.T) {
	tpl := &Template{
		Formulas: []Formula{{Key: "name", Type: "text", Expression: `F["a"]`}},
		Fields: []Field{
			{Key: "full", Type: "text"},
			{Key: "f", Type: "formula", FormulaKey: "name", TargetKey: "full", Trigger: "save"},
		},
	}
	if errs := Validate(tpl); hasErr(errs, "formula-field-incompatible-target") {
		t.Errorf("text->text must be accepted; got %+v", errs)
	}
}

func TestValidate_FormulaFieldNumberAcceptsRange(t *testing.T) {
	tpl := &Template{
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `1`}},
		Fields: []Field{
			{Key: "slider", Type: "range"},
			{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "slider", Trigger: "save"},
		},
	}
	if errs := Validate(tpl); hasErr(errs, "formula-field-incompatible-target") {
		t.Errorf("number->range must be accepted; got %+v", errs)
	}
}

func TestFormulaTargetTypes_CoversEveryFormulaType(t *testing.T) {
	m := FormulaTargetTypes()
	for _, ty := range []string{"number", "text", "date", "bool"} {
		if len(m[ty]) == 0 {
			t.Errorf("FormulaTargetTypes missing entries for %q", ty)
		}
	}
}

func TestValidate_FormulaFieldTargetInLoopRejected(t *testing.T) {
	// A target nested in a loop is an array slot; the whole-form engine can't
	// write a per-iteration scalar there, so it must be rejected.
	tpl := &Template{
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `1`}},
		Fields: []Field{
			{Key: "L", Type: "loopstart"},
			{Key: "inner", Type: "number"},
			{Key: "L", Type: "loopstop"},
			{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "inner", Trigger: "save"},
		},
	}
	if errs := Validate(tpl); !hasErr(errs, "formula-field-target-not-root") {
		t.Errorf("expected formula-field-target-not-root for a looped target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldRootTargetNotFlaggedNotRoot(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "save"},
	)
	if errs := Validate(tpl); hasErr(errs, "formula-field-target-not-root") {
		t.Errorf("a root target must not be flagged not-root; got %+v", errs)
	}
}

func TestValidate_FormulaFieldInLoopWithRootTargetAccepted(t *testing.T) {
	// The formula field itself is whole-form + invisible, so its own placement
	// inside a loop is fine; only the target must be root.
	tpl := &Template{
		Formulas: []Formula{{Key: "total", Type: "number", Expression: `1`}},
		Fields: []Field{
			{Key: "out", Type: "number"},
			{Key: "L", Type: "loopstart"},
			{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "save"},
			{Key: "L", Type: "loopstop"},
		},
	}
	if errs := Validate(tpl); hasErr(errs, "formula-field-target-not-root") {
		t.Errorf("formula field in a loop targeting a root field must be allowed; got %+v", errs)
	}
}

func TestValidate_FormulaFieldLoopedTargetTakesPrecedenceOverType(t *testing.T) {
	// A looped target that is ALSO a type mismatch reports not-root (the more
	// fundamental problem), not incompatible-target.
	tpl := &Template{
		Formulas: []Formula{{Key: "name", Type: "text", Expression: `F["a"]`}},
		Fields: []Field{
			{Key: "L", Type: "loopstart"},
			{Key: "n", Type: "number"},
			{Key: "L", Type: "loopstop"},
			{Key: "f", Type: "formula", FormulaKey: "name", TargetKey: "n", Trigger: "save"},
		},
	}
	errs := Validate(tpl)
	if !hasErr(errs, "formula-field-target-not-root") {
		t.Errorf("expected formula-field-target-not-root; got %+v", errs)
	}
	if hasErr(errs, "formula-field-incompatible-target") {
		t.Errorf("should not also report incompatible-target; got %+v", errs)
	}
}

func TestValidate_FormulaFieldHappyPath(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "a", Type: "number"},
		Field{Key: "b", Type: "number"},
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "load"},
	)
	for _, e := range Validate(tpl) {
		t.Errorf("happy path must validate clean; got %+v", e)
	}
}

// ── Normalize: trigger defaulting + binding hygiene ──────────────────

func TestNormalize_FormulaMissingTriggerDefaultsToSave(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out"},
	)
	Normalize(tpl)
	if got := tpl.Fields[1].Trigger; got != "save" {
		t.Errorf("Trigger = %q, want save", got)
	}
}

func TestNormalize_FormulaTriggerLowercased(t *testing.T) {
	tpl := formulaTpl(
		Field{Key: "out", Type: "number"},
		Field{Key: "f", Type: "formula", FormulaKey: "total", TargetKey: "out", Trigger: "LOAD"},
	)
	Normalize(tpl)
	if got := tpl.Fields[1].Trigger; got != "load" {
		t.Errorf("Trigger = %q, want load", got)
	}
}

func TestNormalize_FormulaBindingsStrippedFromNonFormulaField(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{Key: "x", Type: "text", FormulaKey: "total", TargetKey: "out", Trigger: "save"}},
	}
	Normalize(tpl)
	f := tpl.Fields[0]
	if f.FormulaKey != "" || f.TargetKey != "" || f.Trigger != "" {
		t.Errorf("formula bindings must be stripped from non-formula field; got %+v", f)
	}
}
