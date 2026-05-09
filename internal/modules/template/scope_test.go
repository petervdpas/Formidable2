package template

import "testing"

func TestAssignLevelScopes_Flat(t *testing.T) {
	in := []Field{
		{Key: "a", Type: "text"},
		{Key: "b", Type: "boolean"},
	}
	out := assignLevelScopes(in)
	for i, f := range out {
		if f.LevelScope != 0 {
			t.Errorf("field[%d] %q: want LevelScope=0, got %d", i, f.Key, f.LevelScope)
		}
	}
}

func TestAssignLevelScopes_SingleLoop(t *testing.T) {
	in := []Field{
		{Key: "before", Type: "text"},
		{Key: "L", Type: "loopstart"},
		{Key: "inside", Type: "text"},
		{Key: "L", Type: "loopstop"},
		{Key: "after", Type: "text"},
	}
	out := assignLevelScopes(in)
	want := []int{0, 0, 1, 0, 0}
	for i, f := range out {
		if f.LevelScope != want[i] {
			t.Errorf("field[%d] %q (%s): want LevelScope=%d, got %d",
				i, f.Key, f.Type, want[i], f.LevelScope)
		}
	}
}

func TestAssignLevelScopes_DoubleNest(t *testing.T) {
	in := []Field{
		{Key: "X", Type: "loopstart"},
		{Key: "mid", Type: "text"},
		{Key: "Y", Type: "loopstart"},
		{Key: "deep", Type: "text"},
		{Key: "Y", Type: "loopstop"},
		{Key: "X", Type: "loopstop"},
	}
	out := assignLevelScopes(in)
	want := []int{0, 1, 1, 2, 1, 0}
	for i, f := range out {
		if f.LevelScope != want[i] {
			t.Errorf("field[%d] %q (%s): want LevelScope=%d, got %d",
				i, f.Key, f.Type, want[i], f.LevelScope)
		}
	}
}

func TestAssignLevelScopes_UnbalancedClampsAtZero(t *testing.T) {
	in := []Field{
		{Key: "X", Type: "loopstop"},
		{Key: "after", Type: "text"},
	}
	out := assignLevelScopes(in)
	want := []int{0, 0}
	for i, f := range out {
		if f.LevelScope != want[i] {
			t.Errorf("field[%d] %q (%s): want LevelScope=%d, got %d",
				i, f.Key, f.Type, want[i], f.LevelScope)
		}
	}
}

func TestAssignLevelScopes_RewritesIncomingValues(t *testing.T) {
	in := []Field{
		{Key: "a", Type: "text", LevelScope: 5},
		{Key: "L", Type: "loopstart", LevelScope: 9},
		{Key: "b", Type: "text", LevelScope: 0},
		{Key: "L", Type: "loopstop", LevelScope: 9},
	}
	out := assignLevelScopes(in)
	want := []int{0, 0, 1, 0}
	for i, f := range out {
		if f.LevelScope != want[i] {
			t.Errorf("field[%d] %q (%s): want LevelScope=%d, got %d",
				i, f.Key, f.Type, want[i], f.LevelScope)
		}
	}
}

func TestYAMLMissingLevelScope_AllPresent(t *testing.T) {
	raw := []byte("name: x\nfields:\n  - key: a\n    type: text\n    level_scope: 0\n  - key: L\n    type: loopstart\n    level_scope: 0\n  - key: b\n    type: text\n    level_scope: 1\n  - key: L\n    type: loopstop\n    level_scope: 0\n")
	if yamlMissingLevelScope(raw) {
		t.Errorf("expected false when every field carries level_scope")
	}
}

func TestYAMLMissingLevelScope_OneMissing(t *testing.T) {
	raw := []byte("name: x\nfields:\n  - key: a\n    type: text\n    level_scope: 0\n  - key: b\n    type: text\n")
	if !yamlMissingLevelScope(raw) {
		t.Errorf("expected true when at least one field lacks level_scope")
	}
}

func TestYAMLMissingLevelScope_NoFields(t *testing.T) {
	raw := []byte("name: x\nfields: []\n")
	if yamlMissingLevelScope(raw) {
		t.Errorf("expected false for empty fields list")
	}
}

func TestNormalize_AssignsLevelScopes(t *testing.T) {
	tpl := &Template{
		Fields: []Field{
			{Key: "L", Type: "loopstart"},
			{Key: "x", Type: "text"},
			{Key: "L", Type: "loopstop"},
		},
	}
	Normalize(tpl)
	want := []int{0, 1, 0}
	for i, f := range tpl.Fields {
		if f.LevelScope != want[i] {
			t.Errorf("field[%d] %q (%s): want LevelScope=%d, got %d",
				i, f.Key, f.Type, want[i], f.LevelScope)
		}
	}
}

func TestLevelScopeMismatchErrors_AllZeroSkipped(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "L", Type: "loopstart"},
			{Key: "inner", Type: "text"},
			{Key: "L", Type: "loopstop"},
		},
	})
	for _, e := range errs {
		if e.Type == "level-scope-mismatch" {
			t.Errorf("all-zero input should skip mismatch, got %+v", e)
		}
	}
}

func TestLevelScopeMismatchErrors_StampedAndCorrect(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "L", Type: "loopstart", LevelScope: 0},
			{Key: "inner", Type: "text", LevelScope: 1},
			{Key: "L", Type: "loopstop", LevelScope: 0},
		},
	})
	for _, e := range errs {
		if e.Type == "level-scope-mismatch" {
			t.Errorf("did not expect level-scope-mismatch, got %+v", e)
		}
	}
}

func TestLevelScopeMismatchErrors_StampedButWrong(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "L", Type: "loopstart", LevelScope: 0},
			{Key: "inner", Type: "text", LevelScope: 2},
			{Key: "L", Type: "loopstop", LevelScope: 0},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "level-scope-mismatch" && errs[i].Key == "inner" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected level-scope-mismatch on 'inner', got %+v", errs)
	}
	if d, _ := got.Detail["got"].(int); d != 2 {
		t.Errorf("got detail.got=%v, want 2", got.Detail["got"])
	}
	if d, _ := got.Detail["want"].(int); d != 1 {
		t.Errorf("got detail.want=%v, want 1", got.Detail["want"])
	}
}

func TestExpressionItemLevelScopeErrors_RootIsOK(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "rootflag", Type: "boolean", ExpressionItem: true},
		},
	})
	for _, e := range errs {
		if e.Type == "expression-item-non-root" {
			t.Errorf("did not expect expression-item-non-root, got %+v", e)
		}
	}
}

func TestExpressionItemLevelScopeErrors_InsideLoopRejected(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "L", Type: "loopstart"},
			{Key: "looped", Type: "boolean", ExpressionItem: true},
			{Key: "L", Type: "loopstop"},
		},
	})
	var got *ValidationError
	for i := range errs {
		if errs[i].Type == "expression-item-non-root" {
			got = &errs[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected expression-item-non-root error; got %+v", errs)
	}
	if got.Key != "looped" {
		t.Errorf("error should target the offending field, got Key=%q", got.Key)
	}
}

func TestExpressionItemLevelScopeErrors_DoubleNestRejected(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "X", Type: "loopstart"},
			{Key: "Y", Type: "loopstart"},
			{Key: "deep", Type: "boolean", ExpressionItem: true},
			{Key: "Y", Type: "loopstop"},
			{Key: "X", Type: "loopstop"},
		},
	})
	found := false
	for _, e := range errs {
		if e.Type == "expression-item-non-root" && e.Key == "deep" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected expression-item-non-root for deep, got %+v", errs)
	}
}
