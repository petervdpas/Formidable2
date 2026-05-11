package history

import (
	"reflect"
	"testing"
)

func TestNewManager_StartsEmpty(t *testing.T) {
	m := NewManager(20)
	st := m.State()
	if st.CanBack || st.CanForward {
		t.Fatalf("fresh manager: canBack=%v canForward=%v, want false,false", st.CanBack, st.CanForward)
	}
	if _, ok := m.Back(); ok {
		t.Fatalf("Back on empty: ok=true, want false")
	}
	if _, ok := m.Forward(); ok {
		t.Fatalf("Forward on empty: ok=true, want false")
	}
}

func TestPush_Appends(t *testing.T) {
	m := NewManager(20)
	m.Push("formidable://a.yaml:1.meta.json")
	m.Push("formidable://a.yaml:2.meta.json")

	snap := m.Snapshot()
	want := []string{"formidable://a.yaml:1.meta.json", "formidable://a.yaml:2.meta.json"}
	if !reflect.DeepEqual(snap.Stack, want) || snap.Index != 1 {
		t.Fatalf("snapshot=%+v, want stack=%v index=1", snap, want)
	}
	if st := m.State(); !st.CanBack || st.CanForward {
		t.Fatalf("after two pushes: state=%+v, want canBack=true canForward=false", st)
	}
}

func TestPush_EmptyHrefIsNoOp(t *testing.T) {
	m := NewManager(20)
	m.Push("")
	if snap := m.Snapshot(); len(snap.Stack) != 0 || snap.Index != -1 {
		t.Fatalf("empty push mutated state: %+v", snap)
	}
}

func TestPush_DuplicateAtCursorIsNoOp(t *testing.T) {
	m := NewManager(20)
	m.Push("formidable://a.yaml:1.meta.json")
	m.Push("formidable://a.yaml:1.meta.json")

	snap := m.Snapshot()
	if len(snap.Stack) != 1 || snap.Index != 0 {
		t.Fatalf("duplicate push grew stack: %+v", snap)
	}
}

func TestPush_TruncatesForwardBranch(t *testing.T) {
	m := NewManager(20)
	m.Push("a")
	m.Push("b")
	m.Push("c")
	if _, ok := m.Back(); !ok {
		t.Fatal("Back: ok=false")
	}
	if _, ok := m.Back(); !ok {
		t.Fatal("Back: ok=false")
	}

	m.Push("d")

	snap := m.Snapshot()
	want := []string{"a", "d"}
	if !reflect.DeepEqual(snap.Stack, want) || snap.Index != 1 {
		t.Fatalf("snapshot=%+v, want stack=%v index=1", snap, want)
	}
	if st := m.State(); !st.CanBack || st.CanForward {
		t.Fatalf("state=%+v, want canBack=true canForward=false", st)
	}
}

func TestPush_DropsOldestAtMaxSize(t *testing.T) {
	m := NewManager(3)
	m.Push("a")
	m.Push("b")
	m.Push("c")
	m.Push("d")

	snap := m.Snapshot()
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(snap.Stack, want) || snap.Index != 2 {
		t.Fatalf("snapshot=%+v, want stack=%v index=2", snap, want)
	}
}

func TestBackForward_HappyPath(t *testing.T) {
	m := NewManager(20)
	m.Push("a")
	m.Push("b")
	m.Push("c")

	if href, ok := m.Back(); !ok || href != "b" {
		t.Fatalf("Back: href=%q ok=%v, want b,true", href, ok)
	}
	if href, ok := m.Back(); !ok || href != "a" {
		t.Fatalf("Back: href=%q ok=%v, want a,true", href, ok)
	}
	if _, ok := m.Back(); ok {
		t.Fatal("Back at start: ok=true, want false")
	}
	if href, ok := m.Forward(); !ok || href != "b" {
		t.Fatalf("Forward: href=%q ok=%v, want b,true", href, ok)
	}
	if href, ok := m.Forward(); !ok || href != "c" {
		t.Fatalf("Forward: href=%q ok=%v, want c,true", href, ok)
	}
	if _, ok := m.Forward(); ok {
		t.Fatal("Forward at end: ok=true, want false")
	}
}

func TestSuppressNextPush_SkipsExactlyOne(t *testing.T) {
	m := NewManager(20)
	m.Push("a")

	m.SetSuppressNextPush()
	m.Push("b")
	if snap := m.Snapshot(); len(snap.Stack) != 1 || snap.Stack[0] != "a" {
		t.Fatalf("suppressed push still ran: %+v", snap)
	}

	m.Push("b")
	snap := m.Snapshot()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(snap.Stack, want) || snap.Index != 1 {
		t.Fatalf("second push didn't run: %+v", snap)
	}
}

func TestSnapshot_ReturnsCopy(t *testing.T) {
	m := NewManager(20)
	m.Push("a")
	m.Push("b")

	snap := m.Snapshot()
	snap.Stack[0] = "MUTATED"

	if got := m.Snapshot().Stack[0]; got != "a" {
		t.Fatalf("internal stack mutated via snapshot: %q", got)
	}
}

func TestRestore_HappyPath(t *testing.T) {
	m := NewManager(20)
	m.Restore([]string{"a", "b", "c"}, 1)

	if snap := m.Snapshot(); !reflect.DeepEqual(snap.Stack, []string{"a", "b", "c"}) || snap.Index != 1 {
		t.Fatalf("restored snapshot=%+v", snap)
	}
	st := m.State()
	if !st.CanBack || !st.CanForward {
		t.Fatalf("state=%+v, want both true", st)
	}
}

func TestRestore_ClampsIndex(t *testing.T) {
	cases := []struct {
		name  string
		stack []string
		in    int
		want  int
	}{
		{"negative", []string{"a", "b"}, -5, -1},
		{"too-high", []string{"a", "b"}, 99, 1},
		{"empty-stack-positive-index", []string{}, 3, -1},
		{"empty-stack-negative-index", []string{}, -3, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(20)
			m.Restore(tc.stack, tc.in)
			if got := m.Snapshot().Index; got != tc.want {
				t.Fatalf("Index=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestRestore_TrimsOversizedStack(t *testing.T) {
	m := NewManager(3)
	m.Restore([]string{"a", "b", "c", "d", "e"}, 4)

	snap := m.Snapshot()
	want := []string{"c", "d", "e"}
	if !reflect.DeepEqual(snap.Stack, want) {
		t.Fatalf("trimmed stack=%v, want %v", snap.Stack, want)
	}
	if snap.Index != 2 {
		t.Fatalf("Index=%d after trim, want 2", snap.Index)
	}
}

func TestRestore_DropsEmptyEntries(t *testing.T) {
	m := NewManager(20)
	m.Restore([]string{"a", "", "b", ""}, 3)

	snap := m.Snapshot()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(snap.Stack, want) {
		t.Fatalf("filtered stack=%v, want %v", snap.Stack, want)
	}
	if snap.Index != 1 {
		t.Fatalf("Index=%d after filter, want 1 (clamped to last)", snap.Index)
	}
}

func TestNewManager_RejectsNonPositiveMaxSize(t *testing.T) {
	for _, n := range []int{0, -1, -99} {
		m := NewManager(n)
		for range 100 {
			m.Push("x")
		}
		if got := len(m.Snapshot().Stack); got == 0 || got > 1000 {
			t.Fatalf("NewManager(%d): stack length=%d (want a sane positive default)", n, got)
		}
	}
}
