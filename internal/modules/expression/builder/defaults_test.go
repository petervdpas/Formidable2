package builder

import "testing"

func TestKindForField(t *testing.T) {
	cases := []struct {
		in   string
		want RuleKind
		ok   bool
	}{
		{"boolean", KindBoolean, true},
		{"BOOLEAN", KindBoolean, true},
		{"dropdown", KindEnum, true},
		{"radio", KindEnum, true},
		{"number", KindNumber, true},
		{"range", KindNumber, true},
		{"date", KindDate, true},
		{"text", "", false},
		{"", "", false},
		{"unknown", "", false},
	}
	for _, c := range cases {
		got, ok := KindForField(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("KindForField(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDefaultPredicateForField_Boolean(t *testing.T) {
	p, err := DefaultPredicateForField("boolean", "check")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindBoolean || p.FieldKey != "check" {
		t.Errorf("shape: %+v", p)
	}
	if p.BoolValue == nil || *p.BoolValue != true {
		t.Errorf("BoolValue should default to true; got %v", p.BoolValue)
	}
}

func TestDefaultPredicateForField_Enum(t *testing.T) {
	p, err := DefaultPredicateForField("dropdown", "size")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindEnum || p.FieldKey != "size" || p.EnumOp != EnumOpEquals {
		t.Errorf("shape: %+v", p)
	}
	if p.EnumValues == nil || len(p.EnumValues) != 0 {
		t.Errorf("EnumValues should be empty (non-nil) slice; got %v", p.EnumValues)
	}
}

func TestDefaultPredicateForField_Number(t *testing.T) {
	p, err := DefaultPredicateForField("range", "score")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindNumber || p.NumberOp != NumberOpEq {
		t.Errorf("shape: %+v", p)
	}
	if p.NumberValue == nil || *p.NumberValue != 0 {
		t.Errorf("NumberValue should default to 0; got %v", p.NumberValue)
	}
}

func TestDefaultPredicateForField_Date(t *testing.T) {
	p, err := DefaultPredicateForField("date", "due")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.Kind != KindDate || p.DateOp != DateOpIsOverdue {
		t.Errorf("shape: %+v", p)
	}
	if p.DateArg != nil {
		t.Errorf("DateArg should default to unset; got %v", p.DateArg)
	}
}

func TestDefaultPredicateForField_UnknownTypeIsError(t *testing.T) {
	if _, err := DefaultPredicateForField("text", "k"); err == nil {
		t.Error("expected error for unsupported field type")
	}
	if _, err := DefaultPredicateForField("", "k"); err == nil {
		t.Error("expected error for empty field type")
	}
}

func TestDefaultPredicateForField_EmptyFieldKeyIsError(t *testing.T) {
	if _, err := DefaultPredicateForField("boolean", ""); err == nil {
		t.Error("expected error for empty field key")
	}
	if _, err := DefaultPredicateForField("boolean", "   "); err == nil {
		t.Error("expected error for whitespace-only field key")
	}
}

func TestDefaultRule(t *testing.T) {
	r := DefaultRule()
	if r.Predicates == nil || len(r.Predicates) != 0 {
		t.Errorf("predicates should be empty (non-nil) slice; got %v", r.Predicates)
	}
	if r.Outcome.Text != nil || r.Outcome.Color != "" || r.Outcome.Bg != "" || len(r.Outcome.Classes) != 0 {
		t.Errorf("outcome should be empty; got %+v", r.Outcome)
	}
}

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Rules == nil || len(c.Rules) != 0 {
		t.Errorf("rules should be empty (non-nil) slice; got %v", c.Rules)
	}
	if c.Default.Text != nil || c.Default.Color != "" {
		t.Errorf("default outcome should be empty; got %+v", c.Default)
	}
}
