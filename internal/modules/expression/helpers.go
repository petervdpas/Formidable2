// Package expression is a sandboxed expression engine that renders dynamic labels from a record's
// harvested ExpressionItems. Helpers are pure given (input, nowFn); nowFn is package-level so tests can pin today.
package expression

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"time"
)

// stderr is overridable in tests so debug() output stays out of the test log.
var stderr io.Writer = os.Stderr

// nowFn is overridable from tests via withFakeNow.
var nowFn = time.Now

// dmyPattern matches the only legacy date shape normalised here (DD-MM-YYYY); anything else passes through.
var dmyPattern = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)

// normalizeDate converts DD-MM-YYYY to ISO; anything else is returned unchanged.
func normalizeDate(in any) string {
	s, ok := in.(string)
	if !ok || s == "" {
		return ""
	}
	if !dmyPattern.MatchString(s) {
		return s
	}
	return s[6:10] + "-" + s[3:5] + "-" + s[0:2]
}

// today returns nowFn() truncated to a YYYY-MM-DD ISO date.
func today() string {
	return nowFn().Format("2006-01-02")
}

// notEmpty treats only nil and "" as empty (so 0 and false count as non-empty).
func notEmpty(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return s != ""
	}
	if reflect.TypeOf(v).Kind() == reflect.Slice {
		return reflect.ValueOf(v).Len() > 0
	}
	return true
}

// defaultText returns fallback when v is nil or "". Anything else
// (including 0, false, or a populated map) passes through unchanged.
func defaultText(v any, fallback any) any {
	if v == nil {
		return fallback
	}
	if s, ok := v.(string); ok && s == "" {
		return fallback
	}
	return v
}

// typeOf maps Go reflect kinds onto JS typeof-ish names (maps -> "object", numerics -> "number").
func typeOf(v any) string {
	if v == nil {
		return "null"
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	}
	return rv.Kind().String()
}

// parseDate normalises ISO or legacy DD-MM-YYYY input into a time.Time; (zero, false) on parse failure.
func parseDate(v any) (time.Time, bool) {
	s := normalizeDate(v)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// isOverdue: empty/unparseable counts as overdue; a real date is overdue only when strictly before today.
func isOverdue(v any) bool {
	if !notEmpty(v) {
		return true
	}
	t, ok := parseDate(v)
	if !ok {
		return true
	}
	now, _ := time.Parse("2006-01-02", today())
	return t.Before(now)
}

// isFuture: nil/empty/garbage all return false; only a valid date
// strictly after today is "future".
func isFuture(v any) bool {
	if !notEmpty(v) {
		return false
	}
	t, ok := parseDate(v)
	if !ok {
		return false
	}
	now, _ := time.Parse("2006-01-02", today())
	return t.After(now)
}

// isToday: strict equality with today's ISO date.
func isToday(v any) bool {
	if !notEmpty(v) {
		return false
	}
	t, ok := parseDate(v)
	if !ok {
		return false
	}
	now, _ := time.Parse("2006-01-02", today())
	return t.Equal(now)
}

// daysBetween returns calendar days from a to b (b minus a); garbage inputs return 0.
func daysBetween(a, b any) int {
	ta, ok := parseDate(a)
	if !ok {
		return 0
	}
	tb, ok := parseDate(b)
	if !ok {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// isDueSoon: val is in the future and lands within `days` of today.
// Already-overdue dates do NOT match - they would be reported by
// isOverdue/isOverdueInDays.
func isDueSoon(v any, days int) bool {
	if !notEmpty(v) {
		return false
	}
	diff := daysBetween(today(), normalizeDate(v))
	return diff >= 0 && diff <= days
}

// isOverdueInDays: val is in the past and within `days` before today.
func isOverdueInDays(v any, days int) bool {
	if !notEmpty(v) {
		return false
	}
	diff := daysBetween(normalizeDate(v), today())
	return diff >= 0 && diff <= days
}

// isExpiredAfter: (val + days) < today; empty counts as expired so blank deadlines surface rather than disappear.
func isExpiredAfter(v any, days int) bool {
	if !notEmpty(v) {
		return true
	}
	t, ok := parseDate(v)
	if !ok {
		return true
	}
	expires := t.AddDate(0, 0, days).Format("2006-01-02")
	return expires < today()
}

// isUpcomingBefore: (val - days) > today, compared as ISO strings (which sort the same as time-ordered).
func isUpcomingBefore(v any, days int) bool {
	if !notEmpty(v) {
		return false
	}
	t, ok := parseDate(v)
	if !ok {
		return false
	}
	cutoff := t.AddDate(0, 0, -days).Format("2006-01-02")
	return cutoff > today()
}

// ageInDays is daysBetween(val, today): >= 0 for past dates, negative for future.
func ageInDays(v any) int {
	return daysBetween(v, today())
}

// isSimilar: case-folded Levenshtein similarity ratio >= threshold (default 0.8 when threshold is 0).
func isSimilar(a, b string, threshold float64) bool {
	if threshold == 0 {
		threshold = 0.8
	}
	if a == "" || b == "" {
		return false
	}
	return stringSimilarity(a, b) >= threshold
}

// stringSimilarity returns 1 - editDistance/maxLen (1 identical, 0 different); empty inputs short-circuit to 0.
func stringSimilarity(a, b string) float64 {
	a = lower(a)
	b = lower(b)
	if a == b {
		return 1
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 0
	}
	d := levenshtein(a, b)
	max := la
	if lb > max {
		max = lb
	}
	return 1 - float64(d)/float64(max)
}

// levenshtein is standard DP edit distance, O(la*lb) time and O(lb) space.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// lower is an ASCII-only lowercaser; fine here since the similarity check is a fuzzy heuristic.
func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// debug logs to stderr and returns its first argument, for inspecting a subexpression at runtime.
func debug(args ...any) any {
	if len(args) == 0 {
		return nil
	}
	fmt.Fprintln(stderr, append([]any{"[expr-debug]"}, args...)...)
	return args[0]
}
