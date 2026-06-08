package log

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

func msgs(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Msg
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewBroadcaster_NonPositiveCapacityDefaults(t *testing.T) {
	for _, cap := range []int{0, -1, -500} {
		b := NewBroadcaster(cap)
		if b.cap != 500 {
			t.Errorf("NewBroadcaster(%d).cap = %d, want 500", cap, b.cap)
		}
		if len(b.ring) != 500 {
			t.Errorf("NewBroadcaster(%d) ring len = %d, want 500", cap, len(b.ring))
		}
	}
}

func TestRecent_EmptyRing(t *testing.T) {
	b := NewBroadcaster(8)
	for _, n := range []int{0, 1, 100, -1} {
		if got := b.Recent(n); len(got) != 0 {
			t.Errorf("Recent(%d) on empty ring = %v, want empty", n, msgs(got))
		}
	}
}

func TestRecent_PartialFill(t *testing.T) {
	b := NewBroadcaster(8)
	b.push(Entry{Msg: "a"})
	b.push(Entry{Msg: "b"})
	b.push(Entry{Msg: "c"})

	cases := []struct {
		n    int
		want []string
	}{
		{0, []string{"a", "b", "c"}},  // 0 = everything
		{-1, []string{"a", "b", "c"}}, // negative = everything
		{2, []string{"b", "c"}},       // last 2, chronological
		{3, []string{"a", "b", "c"}},  // exactly size
		{10, []string{"a", "b", "c"}}, // n > size clamps to size
		{1, []string{"c"}},            // single most-recent
	}
	for _, c := range cases {
		if got := msgs(b.Recent(c.n)); !eq(got, c.want) {
			t.Errorf("Recent(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestRecent_FullAndWrapped(t *testing.T) {
	b := NewBroadcaster(3)
	for _, m := range []string{"e1", "e2", "e3", "e4", "e5"} {
		b.push(Entry{Msg: m})
	}
	// Ring holds only the last 3, oldest-first.
	if got := msgs(b.Recent(0)); !eq(got, []string{"e3", "e4", "e5"}) {
		t.Errorf("Recent(0) wrapped = %v, want [e3 e4 e5]", got)
	}
	if got := msgs(b.Recent(2)); !eq(got, []string{"e4", "e5"}) {
		t.Errorf("Recent(2) wrapped = %v, want [e4 e5]", got)
	}
	// n larger than capacity clamps to capacity, never reads stale slots.
	if got := msgs(b.Recent(99)); !eq(got, []string{"e3", "e4", "e5"}) {
		t.Errorf("Recent(99) wrapped = %v, want [e3 e4 e5]", got)
	}
}

func TestRecent_ExactlyFullNoWrap(t *testing.T) {
	b := NewBroadcaster(3)
	for _, m := range []string{"a", "b", "c"} {
		b.push(Entry{Msg: m})
	}
	if !b.full {
		t.Fatal("ring should be marked full at head==0")
	}
	if got := msgs(b.Recent(0)); !eq(got, []string{"a", "b", "c"}) {
		t.Errorf("Recent(0) full-no-wrap = %v, want [a b c]", got)
	}
}

func TestSetEmitter_InstallAndClear(t *testing.T) {
	b := NewBroadcaster(8)
	var got []string
	b.SetEmitter(func(e Entry) { got = append(got, e.Msg) })

	b.push(Entry{Msg: "first"})
	b.push(Entry{Msg: "second"})
	if !eq(got, []string{"first", "second"}) {
		t.Fatalf("emitter saw %v, want [first second]", got)
	}

	// Clearing stops emission but the ring still records.
	b.SetEmitter(nil)
	b.push(Entry{Msg: "third"})
	if !eq(got, []string{"first", "second"}) {
		t.Errorf("emitter saw %v after clear, want unchanged", got)
	}
	if n := len(b.Recent(0)); n != 3 {
		t.Errorf("ring has %d entries after clear, want 3", n)
	}
}

func TestSetEmitter_NilEmitterNeverPanics(t *testing.T) {
	b := NewBroadcaster(4)
	// Default emitter is nil; push must not panic.
	b.push(Entry{Msg: "x"})
	if got := b.Recent(0); len(got) != 1 {
		t.Errorf("ring = %v, want one entry", msgs(got))
	}
}

func TestHandler_RecordBecomesEntry(t *testing.T) {
	b := NewBroadcaster(8)
	l := slog.New(b.Handler())
	l.Warn("hello", "k", "v")

	got := b.Recent(0)
	if len(got) != 1 {
		t.Fatalf("ring had %d, want 1", len(got))
	}
	e := got[0]
	if e.Level != "WARN" || e.Msg != "hello" {
		t.Errorf("entry = %+v, want WARN/hello", e)
	}
	if e.Attrs["k"] != "v" {
		t.Errorf("attr k = %v, want v", e.Attrs["k"])
	}
}

func TestHandler_NoAttrsLeavesMapNil(t *testing.T) {
	b := NewBroadcaster(8)
	slog.New(b.Handler()).Info("bare")
	e := b.Recent(0)[0]
	if e.Attrs != nil {
		t.Errorf("Attrs = %v, want nil for attr-less record", e.Attrs)
	}
}

func TestHandler_WithAttrsAndWithGroupDottedKeys(t *testing.T) {
	b := NewBroadcaster(8)
	// Pre-group attr plus a group, then a record attr inside the group.
	l := slog.New(b.Handler()).With("base", "B").WithGroup("g")
	l.Info("grouped", "leaf", "L")

	e := b.Recent(0)[0]
	if e.Attrs["g.base"] != "B" {
		t.Errorf("expected g.base=B, got attrs %v", e.Attrs)
	}
	if e.Attrs["g.leaf"] != "L" {
		t.Errorf("expected g.leaf=L, got attrs %v", e.Attrs)
	}
}

func TestHandler_NestedGroupAttr(t *testing.T) {
	b := NewBroadcaster(8)
	slog.New(b.Handler()).Info("nest",
		slog.Group("outer", slog.String("inner", "v")))

	e := b.Recent(0)[0]
	if e.Attrs["outer.inner"] != "v" {
		t.Errorf("expected outer.inner=v, got %v", e.Attrs)
	}
}

func TestHandler_WithGroupEmptyIsNoOp(t *testing.T) {
	b := NewBroadcaster(8)
	h := b.Handler()
	if h.WithGroup("") != h {
		t.Error("WithGroup(\"\") should return the same handler")
	}
}

func TestHandler_EmptyAttrSkipped(t *testing.T) {
	dst := map[string]any{}
	collectAttr(dst, nil, slog.Attr{})
	if len(dst) != 0 {
		t.Errorf("empty attr should be skipped; dst = %v", dst)
	}
}

func TestHandler_WithAttrsDoesNotMutateParent(t *testing.T) {
	b := NewBroadcaster(8)
	parent := b.Handler()
	child := parent.WithAttrs([]slog.Attr{slog.String("only", "child")})

	slog.New(parent).Info("p")
	slog.New(child).Info("c")

	entries := b.Recent(0)
	var pe, ce Entry
	for _, e := range entries {
		switch e.Msg {
		case "p":
			pe = e
		case "c":
			ce = e
		}
	}
	if _, ok := pe.Attrs["only"]; ok {
		t.Error("parent handler leaked child's attr")
	}
	if ce.Attrs["only"] != "child" {
		t.Errorf("child missing its own attr: %v", ce.Attrs)
	}
}

func TestJoinKey(t *testing.T) {
	cases := []struct {
		groups []string
		key    string
		want   string
	}{
		{nil, "k", "k"},
		{[]string{}, "k", "k"},
		{[]string{"a"}, "k", "a.k"},
		{[]string{"a", "b"}, "k", "a.b.k"},
		{[]string{"a", "b"}, "", "a.b"},
		{nil, "", ""},
		{[]string{"only"}, "", "only"},
	}
	for _, c := range cases {
		if got := joinKey(c.groups, c.key); got != c.want {
			t.Errorf("joinKey(%v, %q) = %q, want %q", c.groups, c.key, got, c.want)
		}
	}
}

func TestEnabled_AlwaysTrue(t *testing.T) {
	b := NewBroadcaster(4)
	h := b.Handler()
	for _, lvl := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if !h.Enabled(context.Background(), lvl) {
			t.Errorf("Enabled(%v) = false, want true", lvl)
		}
	}
}

func TestMultiHandler_EnabledIsOR(t *testing.T) {
	off := levelHandler{min: slog.LevelError}
	on := levelHandler{min: slog.LevelDebug}
	m := newMultiHandler(off, on)
	if !m.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("multi Enabled should be true when any child is enabled")
	}

	allOff := newMultiHandler(levelHandler{min: slog.LevelError})
	if allOff.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("multi Enabled should be false when all children disabled")
	}
}

func TestMultiHandler_HandleSkipsDisabledChildren(t *testing.T) {
	b1 := NewBroadcaster(8) // debug: sees everything
	b2 := NewBroadcaster(8) // error-only
	m := newMultiHandler(
		gatedHandler{min: slog.LevelDebug, inner: b1.Handler()},
		gatedHandler{min: slog.LevelError, inner: b2.Handler()},
	)
	l := slog.New(m)
	l.Info("info-line")
	l.Error("error-line")

	if got := msgs(b1.Recent(0)); !eq(got, []string{"info-line", "error-line"}) {
		t.Errorf("debug child saw %v, want both", got)
	}
	if got := msgs(b2.Recent(0)); !eq(got, []string{"error-line"}) {
		t.Errorf("error child saw %v, want only error-line", got)
	}
}

func TestMultiHandler_WithAttrsAndGroupFanOut(t *testing.T) {
	b1 := NewBroadcaster(8)
	b2 := NewBroadcaster(8)
	m := newMultiHandler(b1.Handler(), b2.Handler())
	l := slog.New(m).With("shared", "S").WithGroup("grp")
	l.Info("fan", "x", "1")

	for i, b := range []*Broadcaster{b1, b2} {
		e := b.Recent(0)[0]
		if e.Attrs["grp.shared"] != "S" || e.Attrs["grp.x"] != "1" {
			t.Errorf("child %d attrs = %v, want grp.shared/grp.x", i, e.Attrs)
		}
	}
}

func TestRecent_ConcurrentWithPush(t *testing.T) {
	b := NewBroadcaster(64)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			b.push(Entry{Msg: "x"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = b.Recent(10)
		}
	}()
	wg.Wait()
	// No assertion beyond "the race detector stayed quiet"; ring is bounded.
	if n := len(b.Recent(0)); n > 64 {
		t.Errorf("ring overflowed its capacity: %d", n)
	}
}

// levelHandler is a minimal slog.Handler whose Enabled gates on min level
// and whose Handle is a no-op; it exists only to drive multiHandler.Enabled.
type levelHandler struct{ min slog.Level }

func (h levelHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }
func (h levelHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h levelHandler) WithAttrs([]slog.Attr) slog.Handler           { return h }
func (h levelHandler) WithGroup(string) slog.Handler                { return h }

// gatedHandler gates on min level and forwards enabled records to inner,
// letting a test confirm multiHandler honors per-child Enabled.
type gatedHandler struct {
	min   slog.Level
	inner slog.Handler
}

func (h gatedHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }
func (h gatedHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}
func (h gatedHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return gatedHandler{min: h.min, inner: h.inner.WithAttrs(a)}
}
func (h gatedHandler) WithGroup(n string) slog.Handler {
	return gatedHandler{min: h.min, inner: h.inner.WithGroup(n)}
}
