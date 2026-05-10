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

func TestDefaultRuleForField_Boolean(t *testing.T) {
	r, err := DefaultRuleForField("boolean")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Kind != KindBoolean {
		t.Errorf("kind: %q", r.Kind)
	}
	if r.BoolValue == nil || *r.BoolValue != true {
		t.Errorf("default boolean value should be true (pointer); got %v", r.BoolValue)
	}
}

func TestDefaultRuleForField_Enum(t *testing.T) {
	r, err := DefaultRuleForField("dropdown")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Kind != KindEnum || r.EnumOp != EnumOpEquals {
		t.Errorf("enum default shape wrong: %+v", r)
	}
	if r.EnumValues == nil || len(r.EnumValues) != 0 {
		t.Errorf("enum values should be empty slice (not nil), got %v", r.EnumValues)
	}
}

func TestDefaultRuleForField_Number(t *testing.T) {
	r, err := DefaultRuleForField("range")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Kind != KindNumber || r.NumberOp != NumberOpEq {
		t.Errorf("number default shape wrong: %+v", r)
	}
	if r.NumberValue == nil || *r.NumberValue != 0 {
		t.Errorf("number value should default to 0 (pointer); got %v", r.NumberValue)
	}
}

func TestDefaultRuleForField_Date(t *testing.T) {
	r, err := DefaultRuleForField("date")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Kind != KindDate || r.DateOp != DateOpIsOverdue {
		t.Errorf("date default shape wrong: %+v", r)
	}
	if r.DateArg != nil {
		t.Errorf("date arg should default to unset; got %v", r.DateArg)
	}
}

func TestDefaultRuleForField_UnknownTypeIsError(t *testing.T) {
	if _, err := DefaultRuleForField("text"); err == nil {
		t.Error("expected error for unsupported field type")
	}
	if _, err := DefaultRuleForField(""); err == nil {
		t.Error("expected error for empty field type")
	}
}

func TestDefaultFieldConfig(t *testing.T) {
	c := DefaultFieldConfig()
	if c.Display {
		t.Error("display should default to false")
	}
	if c.Rules == nil || len(c.Rules) != 0 {
		t.Errorf("rules should be empty (non-nil) slice; got %v", c.Rules)
	}
	if c.Styling == nil {
		t.Error("styling should be non-nil empty map")
	}
}
