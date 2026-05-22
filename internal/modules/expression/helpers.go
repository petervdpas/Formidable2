// Package expression provides a sandboxed expression engine used by
// the sidebar (and other surfaces) to render dynamic labels from a
// record's harvested ExpressionItems.
//
// Helpers ported 1:1 from the original Electron build's
// controls/expressionHelpers.js. Each helper is pure and deterministic
// given (input, nowFn). nowFn is package-level so tests can pin a
// fixed today; production code uses time.Now.
package expression

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"time"
)

// stderr is overridable in tests so debug() output stays out of the
// `go test -v` log unless the test explicitly captures it.
var stderr io.Writer = os.Stderr

// nowFn is overridable from tests via withFakeNow. Production stays on
// time.Now. Module-level rather than per-Manager because all helpers
// pipe through these date utilities and a per-call clock would clutter
// every signature for no real win.
var nowFn = time.Now

// dmyPattern matches `DD-MM-YYYY` - the only legacy date shape the JS
// original normalised. Anything else passes through unchanged so users
// can pass ISO strings, free text, or pre-formatted display values
// without the helper layer mangling them.
var dmyPattern = regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)

// normalizeDate converts a `DD-MM-YYYY` input to ISO `YYYY-MM-DD`.
// Anything else (including empty, ISO already, or garbage) is returned
// unchanged - callers test for emptiness or parse failure downstream.
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

// notEmpty mirrors JS's loose-truthy on strings/arrays. Numbers and
// booleans pass through as "non-empty" because the original treated
// only nil and "" as empty (so 0 is "not empty", matching JS).
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

// typeOf maps Go's reflect kinds onto the JS `typeof`-ish names the
// original engine emitted, so existing templates keep working without
// learning a new vocabulary. Maps render as "object", numerics as
// "number", everything else falls through to reflect's lowercase
// kind name.
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

// parseDate normalises any date-shaped input into a time.Time. Returns
// (zero, false) on parse failure so callers can surface a falsy
// answer rather than panic. Two accepted shapes: ISO `YYYY-MM-DD` and
// legacy `DD-MM-YYYY` (already normalised by normalizeDate).
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

// isOverdue: empty value is overdue (matches JS `if (!val) return true`).
// A real date is overdue when strictly less than today - a date that
// equals today is "due", not "overdue".
func isOverdue(v any) bool {
	if !notEmpty(v) {
		return true
	}
	t, ok := parseDate(v)
	if !ok {
		return true // un-parseable values are treated as missing → overdue
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

// daysBetween returns calendar days from a → b (b minus a). Garbage
// inputs return 0 - same falsy contract as the JS original which
// returned null but in Go the zero value is the closest analogue.
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

// isExpiredAfter: (val + days) < today. Matches JS by treating empty
// as expired so blank deadlines surface in red rather than disappear.
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

// isUpcomingBefore: (val - days) > today. JS-equivalent string
// comparison so `2026-04-30` < `2026-05-01` lexicographically (ISO
// dates sort the same as time-ordered).
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

// ageInDays: shorthand for daysBetween(val, today). Always >= 0 for
// past dates; negative for future ones (matches JS behaviour).
func ageInDays(v any) int {
	return daysBetween(v, today())
}

// isSimilar: Levenshtein-based similarity ratio (0–1) ≥ threshold.
// Threshold defaults to 0.8 when the caller passes 0 (matches JS).
// Both inputs are case-folded so "Hello" and "hello" score 1.0.
func isSimilar(a, b string, threshold float64) bool {
	if threshold == 0 {
		threshold = 0.8
	}
	if a == "" || b == "" {
		return false
	}
	return stringSimilarity(a, b) >= threshold
}

// stringSimilarity returns 1 - (edit distance / max length). 0 means
// completely different; 1 means identical. Empty inputs short-circuit
// to 0 - the JS original returned 1 for both-empty but the only
// caller (isSimilar) now special-cases empty to false up front, so
// this branch is dead code outside direct unit tests.
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

// levenshtein: standard DP. O(la*lb) time, O(lb) space - only the
// previous row is needed. Plenty fast for sidebar-sized strings.
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

// lower is a tiny helper used by isSimilar - pulled out of unicode/strings
// to avoid an extra import in this file. ASCII-only is fine because
// the similarity check is a fuzzy heuristic and Unicode case folding
// would not meaningfully change a tag like "audit_control".
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

// debug is the JS-side `debug(...args)` - logs to stderr and returns
// the first argument. Useful when authoring an expression: wrap a
// subexpression to inspect its value at runtime without disturbing
// the result. Marked `safe=false` in the registry so it ships only
// when the manager is built with verbose helpers enabled.
func debug(args ...any) any {
	if len(args) == 0 {
		return nil
	}
	fmt.Fprintln(stderr, append([]any{"[expr-debug]"}, args...)...)
	return args[0]
}
