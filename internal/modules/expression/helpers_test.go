package expression

import (
	"testing"
	"time"
)

// All date helpers compare against time.Now() at evaluation time.
// Tests pin a fixed today via withFakeNow so they are stable across
// runs (no flakes around midnight, no leap-second drama).
func withFakeNow(t *testing.T, iso string) {
	t.Helper()
	tt, err := time.Parse("2006-01-02", iso)
	if err != nil {
		t.Fatalf("withFakeNow: %v", err)
	}
	prev := nowFn
	nowFn = func() time.Time { return tt }
	t.Cleanup(func() { nowFn = prev })
}

func TestNotEmpty(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{nil, false},
		{"", false},
		{"x", true},
		{0, true},        // zero is a value, only nil/empty-string are "empty"
		{[]any{}, false}, // empty slice is empty
		{[]any{"a"}, true},
		{map[string]any{}, true}, // maps don't get the empty-treatment in the JS original either
	}
	for _, c := range cases {
		got := notEmpty(c.in)
		if got != c.want {
			t.Errorf("notEmpty(%v): want %v, got %v", c.in, c.want, got)
		}
	}
}

func TestDefaultText(t *testing.T) {
	if got := defaultText(nil, "fallback"); got != "fallback" {
		t.Errorf("nil → fallback; got %q", got)
	}
	if got := defaultText("", "fb"); got != "fb" {
		t.Errorf("\"\" → fallback; got %q", got)
	}
	if got := defaultText("real", "fb"); got != "real" {
		t.Errorf("real value passed through; got %q", got)
	}
	if got := defaultText(0, "fb"); got != 0 {
		t.Errorf("zero is not empty; got %v", got)
	}
}

func TestTypeOf(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "null"},
		{[]any{1, 2}, "array"},
		{"x", "string"},
		{42, "number"},
		{3.14, "number"},
		{true, "boolean"},
		{map[string]any{}, "object"},
	}
	for _, c := range cases {
		got := typeOf(c.in)
		if got != c.want {
			t.Errorf("typeOf(%v): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestNormalizeDate(t *testing.T) {
	if got := normalizeDate("01-02-2026"); got != "2026-02-01" {
		t.Errorf("dd-mm-yyyy → ISO; got %q", got)
	}
	if got := normalizeDate("2026-02-01"); got != "2026-02-01" {
		t.Errorf("ISO passes through; got %q", got)
	}
	if got := normalizeDate(""); got != "" {
		t.Errorf("empty passes through; got %q", got)
	}
	if got := normalizeDate("not-a-date"); got != "not-a-date" {
		t.Errorf("non-date passes through unchanged; got %q", got)
	}
}

func TestToday(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if got := today(); got != "2026-05-09" {
		t.Errorf("today: want 2026-05-09, got %q", got)
	}
}

func TestIsOverdue(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	cases := []struct {
		in   any
		want bool
	}{
		{nil, true},                   // empty is overdue per JS original
		{"", true},                    // same
		{"2026-05-08", true},          // yesterday
		{"2026-05-09", false},         // today is not overdue
		{"2026-05-10", false},         // tomorrow
		{"08-05-2026", true},          // dd-mm-yyyy yesterday
	}
	for _, c := range cases {
		if got := isOverdue(c.in); got != c.want {
			t.Errorf("isOverdue(%v): want %v, got %v", c.in, c.want, got)
		}
	}
}

func TestIsFuture(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if !isFuture("2026-05-10") {
		t.Errorf("tomorrow should be future")
	}
	if isFuture("2026-05-09") {
		t.Errorf("today is not future")
	}
	if isFuture("2026-05-08") {
		t.Errorf("yesterday is not future")
	}
	if isFuture(nil) {
		t.Errorf("nil is not future")
	}
}

func TestIsToday(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if !isToday("2026-05-09") {
		t.Errorf("isToday positive")
	}
	if isToday("2026-05-08") {
		t.Errorf("yesterday is not today")
	}
	if isToday(nil) {
		t.Errorf("nil is not today")
	}
}

func TestDaysBetween(t *testing.T) {
	if got := daysBetween("2026-07-01", "2026-07-30"); got != 29 {
		t.Errorf("july 1 → july 30: want 29, got %d", got)
	}
	if got := daysBetween("2026-02-28", "2026-03-01"); got != 1 {
		t.Errorf("non-leap year boundary: want 1, got %d", got)
	}
	if got := daysBetween("2024-02-28", "2024-03-01"); got != 2 {
		t.Errorf("leap year boundary: want 2, got %d", got)
	}
	if got := daysBetween("not-a-date", "2026-01-01"); got != 0 {
		t.Errorf("garbage in: want 0, got %d", got)
	}
}

func TestIsDueSoon(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if !isDueSoon("2026-05-12", 7) { // 3 days away ≤ 7
		t.Errorf("3 days out, 7-day window: should be due soon")
	}
	if isDueSoon("2026-05-20", 7) { // 11 days away > 7
		t.Errorf("11 days out, 7-day window: not due soon")
	}
	if isDueSoon("2026-05-08", 7) { // already past - not "due soon"
		t.Errorf("past date is not due soon (it's overdue)")
	}
	if isDueSoon(nil, 7) {
		t.Errorf("nil is not due soon")
	}
}

func TestIsOverdueInDays(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if !isOverdueInDays("2026-05-07", 3) { // 2 days overdue ≤ 3
		t.Errorf("2 days overdue, 3-day window: should match")
	}
	if isOverdueInDays("2026-05-01", 3) { // 8 days overdue > 3
		t.Errorf("8 days overdue, 3-day window: should not match")
	}
	if isOverdueInDays("2026-05-15", 3) { // future → not overdue
		t.Errorf("future date is not overdueInDays")
	}
}

func TestAgeInDays(t *testing.T) {
	withFakeNow(t, "2026-05-09")
	if got := ageInDays("2026-05-01"); got != 8 {
		t.Errorf("8 days old: got %d", got)
	}
	if got := ageInDays("not-a-date"); got != 0 {
		t.Errorf("garbage in: got %d", got)
	}
}

func TestIsSimilar(t *testing.T) {
	if !isSimilar("hello", "hello", 0.99) {
		t.Errorf("identical strings should clear 0.99 threshold")
	}
	if !isSimilar("hello", "Hello", 0.99) {
		t.Errorf("case-insensitive match should clear 0.99")
	}
	if !isSimilar("audit control", "audit_control", 0.85) {
		t.Errorf("audit control vs audit_control should clear 0.85")
	}
	if isSimilar("apple", "banana", 0.5) {
		t.Errorf("apple vs banana should fall below 0.5")
	}
	if isSimilar("", "x", 0.5) {
		t.Errorf("empty input returns 0 similarity")
	}
}

func TestIsExpiredAfter(t *testing.T) {
	withFakeNow(t, "2026-08-01")
	// val + days = jul 31 < today (aug 1) → expired.
	if !isExpiredAfter("2026-07-01", 30) {
		t.Errorf("aug 1: jul 1 + 30 = jul 31 < aug 1 → expired")
	}
	withFakeNow(t, "2026-07-30")
	// val + days = jul 31, today = jul 30 → not yet expired.
	if isExpiredAfter("2026-07-01", 30) {
		t.Errorf("jul 30: jul 1 + 30 = jul 31, jul 31 < jul 30 is false → not expired")
	}
	// nil treated as expired (matches JS isExpiredAfter).
	if !isExpiredAfter(nil, 30) {
		t.Errorf("nil should be expired (matches JS truthiness fall-through)")
	}
}

func TestIsUpcomingBefore(t *testing.T) {
	withFakeNow(t, "2026-07-25")
	if !isUpcomingBefore("2026-08-01", 5) {
		t.Errorf("aug 1 - 5 = jul 27, today is jul 25 < jul 27 → upcoming")
	}
	if isUpcomingBefore("2026-07-26", 5) {
		t.Errorf("jul 26 - 5 = jul 21 < today → not upcoming")
	}
}
