package render

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// toFloat coerces a value to float64. Strings are parsed as numbers;
// nil → 0; bool → 1/0; unsupported types → NaN.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	default:
		return math.NaN()
	}
}

// EvaluateMath mirrors `controls/calculator.js` - JS-style numeric
// coercion, returns "" on NaN inputs or division by zero, otherwise
// returns float64 (or string for the "pad" op).
func EvaluateMath(a any, op string, b any) any {
	x := toFloat(a)
	y := toFloat(b)

	if math.IsNaN(x) {
		return ""
	}
	switch op {
	case "/", "divide", "%", "mod":
		if y == 0 {
			return ""
		}
	}

	switch op {
	case "+", "add":
		return x + y
	case "-", "subtract":
		return x - y
	case "*", "multiply":
		return x * y
	case "/", "divide":
		return x / y
	case "%", "mod":
		return math.Mod(x, y)
	case "pad":
		s := strconv.FormatFloat(x, 'f', -1, 64)
		width := int(y)
		if width <= len(s) {
			return s
		}
		return strings.Repeat("0", width-len(s)) + s
	case "abs":
		return math.Abs(x)
	case "round":
		return math.Round(x)
	case "ceil":
		return math.Ceil(x)
	case "floor":
		return math.Floor(x)
	default:
		return ""
	}
}

// Compare mirrors `controls/calculator.js`. The "===" / "!==" ops are
// strict (type + value), "==" / "!=" coerce to string for comparison,
// and the relational ops compare numerically.
func Compare(a any, op string, b any) bool {
	switch op {
	case "===":
		return strictEqual(a, b)
	case "!==":
		return !strictEqual(a, b)
	case "==":
		return looseEqual(a, b)
	case "!=":
		return !looseEqual(a, b)
	case "<":
		return toFloat(a) < toFloat(b)
	case "<=":
		return toFloat(a) <= toFloat(b)
	case ">":
		return toFloat(a) > toFloat(b)
	case ">=":
		return toFloat(a) >= toFloat(b)
	}
	return false
}

func strictEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if fmt.Sprintf("%T", a) != fmt.Sprintf("%T", b) {
		return false
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func looseEqual(a, b any) bool {
	if isNumber(a) && isNumber(b) {
		return toFloat(a) == toFloat(b)
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func isNumber(v any) bool {
	switch x := v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	case string:
		_, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return err == nil
	}
	return false
}

// Stats is the result shape of ComputeStats - mirrors the original
// `computeStats` return object. Percentile/PercentileInput are pointers
// so "absent" is distinguishable from zero.
type Stats struct {
	Min             float64  `json:"min"`
	Max             float64  `json:"max"`
	Sum             float64  `json:"sum"`
	Avg             float64  `json:"avg"`
	Median          float64  `json:"median"`
	Stddev          float64  `json:"stddev"`
	Percentile      *float64 `json:"percentile,omitempty"`
	PercentileInput *float64 `json:"percentile_input,omitempty"`
	Count           int      `json:"count"`
}

// ComputeStats mirrors `computeStats` in calculator.js. Non-numeric
// values are silently dropped. Returns nil when no usable values
// remain. percentile may be nil to skip percentile computation; values
// outside [0, 100] are clamped.
func ComputeStats(values []any, percentile *float64) *Stats {
	clean := make([]float64, 0, len(values))
	for _, v := range values {
		f := toFloat(v)
		if !math.IsNaN(f) {
			clean = append(clean, f)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	sort.Float64s(clean)

	sum := 0.0
	for _, x := range clean {
		sum += x
	}
	n := len(clean)
	avg := sum / float64(n)

	mid := n / 2
	var median float64
	if n%2 == 0 {
		median = (clean[mid-1] + clean[mid]) / 2
	} else {
		median = clean[mid]
	}

	var stddev float64
	if n > 1 {
		variance := 0.0
		for _, x := range clean {
			variance += (x - avg) * (x - avg)
		}
		variance /= float64(n - 1)
		stddev = math.Sqrt(variance)
	}

	out := &Stats{
		Min:    clean[0],
		Max:    clean[n-1],
		Sum:    sum,
		Avg:    avg,
		Median: median,
		Stddev: stddev,
		Count:  n,
	}

	if percentile != nil {
		p := *percentile
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		idx := (p / 100) * float64(n-1)
		lower := int(math.Floor(idx))
		upper := int(math.Ceil(idx))
		var pv float64
		if lower == upper {
			pv = clean[lower]
		} else {
			weight := idx - float64(lower)
			pv = clean[lower]*(1-weight) + clean[upper]*weight
		}
		out.Percentile = &pv
		pi := *percentile
		out.PercentileInput = &pi
	}

	return out
}
