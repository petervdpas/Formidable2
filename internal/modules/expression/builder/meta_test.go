package builder

import "testing"

func TestOperatorsForKind_Boolean(t *testing.T) {
	ops := OperatorsForKind(KindBoolean)
	if len(ops) != 0 {
		t.Errorf("boolean kind has no operator picker; want [], got %v", ops)
	}
}

func TestOperatorsForKind_Enum(t *testing.T) {
	ops := OperatorsForKind(KindEnum)
	if len(ops) != 2 {
		t.Fatalf("enum kind should expose 2 ops, got %d", len(ops))
	}
	gotOps := map[string]bool{}
	for _, op := range ops {
		gotOps[op.Op] = true
		if op.LabelKey == "" {
			t.Errorf("op %q missing label key", op.Op)
		}
	}
	if !gotOps["equals"] || !gotOps["not_equals"] {
		t.Errorf("enum ops should include equals + not_equals; got %v", gotOps)
	}
}

func TestOperatorsForKind_Number(t *testing.T) {
	ops := OperatorsForKind(KindNumber)
	if len(ops) != 6 {
		t.Fatalf("number kind should expose 6 ops, got %d", len(ops))
	}
	want := map[string]bool{"==": false, "!=": false, ">": false, ">=": false, "<": false, "<=": false}
	for _, op := range ops {
		want[op.Op] = true
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("missing number op %q", k)
		}
	}
}

func TestOperatorsForKind_Date(t *testing.T) {
	if ops := OperatorsForKind(KindDate); len(ops) != 0 {
		t.Errorf("date kind has its own DateOps list, not Operators; got %v", ops)
	}
}

func TestOperatorsForKind_Unknown(t *testing.T) {
	if ops := OperatorsForKind("garbage"); len(ops) != 0 {
		t.Errorf("unknown kind should return empty slice, got %v", ops)
	}
}

func TestDateOps(t *testing.T) {
	ops := DateOps()
	if len(ops) != 9 {
		t.Fatalf("date helper set should be 9 ops, got %d", len(ops))
	}
	hasArg := map[DateOp]bool{
		DateOpIsDueSoon:        true,
		DateOpIsOverdueInDays:  true,
		DateOpIsExpiredAfter:   true,
		DateOpIsUpcomingBefore: true,
		DateOpAgeGt:            true,
		DateOpAgeLt:            true,
	}
	for _, op := range ops {
		if op.LabelKey == "" {
			t.Errorf("date op %q missing label key", op.Op)
		}
		if hasArg[op.Op] != op.HasArg {
			t.Errorf("date op %q HasArg = %v, want %v", op.Op, op.HasArg, hasArg[op.Op])
		}
	}
}
