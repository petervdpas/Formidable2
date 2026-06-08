package render

import (
	"math"
	"testing"
)

func TestEvaluateMath_Add(t *testing.T) {
	if got := EvaluateMath(2, "+", 3); got != float64(5) {
		t.Errorf("2+3 = %v, want 5", got)
	}
	if got := EvaluateMath(2, "add", 3); got != float64(5) {
		t.Errorf("2 add 3 = %v, want 5", got)
	}
}

func TestEvaluateMath_Subtract(t *testing.T) {
	if got := EvaluateMath(10, "-", 3); got != float64(7) {
		t.Errorf("10-3 = %v, want 7", got)
	}
}

func TestEvaluateMath_DivideByZero(t *testing.T) {
	if got := EvaluateMath(10, "/", 0); got != "" {
		t.Errorf("10/0 = %v, want empty string", got)
	}
	if got := EvaluateMath(10, "%", 0); got != "" {
		t.Errorf("10%%0 = %v, want empty string", got)
	}
}

func TestEvaluateMath_NaN(t *testing.T) {
	if got := EvaluateMath("abc", "+", 1); got != "" {
		t.Errorf(`"abc"+1 = %v, want empty string`, got)
	}
}

func TestEvaluateMath_Pad(t *testing.T) {
	if got := EvaluateMath(5, "pad", 3); got != "005" {
		t.Errorf("pad 5 to width 3 = %v, want 005", got)
	}
}

func TestEvaluateMath_Unary(t *testing.T) {
	if got := EvaluateMath(-3.7, "abs", 0); got != float64(3.7) {
		t.Errorf("abs(-3.7) = %v, want 3.7", got)
	}
	if got := EvaluateMath(3.4, "round", 0); got != float64(3) {
		t.Errorf("round(3.4) = %v, want 3", got)
	}
	if got := EvaluateMath(3.1, "ceil", 0); got != float64(4) {
		t.Errorf("ceil(3.1) = %v, want 4", got)
	}
	if got := EvaluateMath(3.9, "floor", 0); got != float64(3) {
		t.Errorf("floor(3.9) = %v, want 3", got)
	}
}

func TestEvaluateMath_StringInputs(t *testing.T) {
	// Numeric strings (from form values) should coerce.
	if got := EvaluateMath("4", "*", "5"); got != float64(20) {
		t.Errorf("4*5 = %v, want 20", got)
	}
}

func TestEvaluateMath_Unknown(t *testing.T) {
	if got := EvaluateMath(1, "wat", 2); got != "" {
		t.Errorf("unknown op = %v, want empty string", got)
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a    any
		op   string
		b    any
		want bool
	}{
		{1, "===", 1, true},
		{1, "===", "1", false},
		{1, "==", "1", true},
		{1, "!==", "1", true},
		{1, "<", 2, true},
		{2, "<=", 2, true},
		{3, ">", 2, true},
		{3, ">=", 3, true},
		{1, "??", 1, false}, // unknown op
	}
	for _, c := range cases {
		got := Compare(c.a, c.op, c.b)
		if got != c.want {
			t.Errorf("Compare(%v, %q, %v) = %v, want %v", c.a, c.op, c.b, got, c.want)
		}
	}
}

func TestComputeStats_Empty(t *testing.T) {
	if got := ComputeStats(nil, nil); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
	if got := ComputeStats([]any{"x", "y"}, nil); got != nil {
		t.Errorf("all-NaN input should return nil, got %v", got)
	}
}

func TestComputeStats_Basic(t *testing.T) {
	s := ComputeStats([]any{1, 2, 3, 4, 5}, nil)
	if s == nil {
		t.Fatal("nil stats")
	}
	if s.Count != 5 {
		t.Errorf("count = %d, want 5", s.Count)
	}
	if s.Min != 1 || s.Max != 5 {
		t.Errorf("min/max = %v/%v, want 1/5", s.Min, s.Max)
	}
	if s.Avg != 3 {
		t.Errorf("avg = %v, want 3", s.Avg)
	}
	if s.Median != 3 {
		t.Errorf("median = %v, want 3", s.Median)
	}
	if math.Abs(s.Stddev-1.5811388300841898) > 1e-9 {
		t.Errorf("stddev = %v, want sample stddev of {1..5}", s.Stddev)
	}
}

func TestComputeStats_EvenMedian(t *testing.T) {
	s := ComputeStats([]any{1, 2, 3, 4}, nil)
	if s.Median != 2.5 {
		t.Errorf("median = %v, want 2.5", s.Median)
	}
}

func TestComputeStats_Percentile(t *testing.T) {
	p := 50.0
	s := ComputeStats([]any{1, 2, 3, 4, 5}, &p)
	if s.Percentile == nil || *s.Percentile != 3 {
		t.Errorf("p50 = %v, want 3", s.Percentile)
	}
	if s.PercentileInput == nil || *s.PercentileInput != 50 {
		t.Errorf("percentile input = %v, want 50", s.PercentileInput)
	}
}

func TestComputeStats_PercentileInterpolated(t *testing.T) {
	p := 25.0
	s := ComputeStats([]any{1, 2, 3, 4, 5}, &p)
	// (25/100) * (5-1) = 1.0 → exact index 1 → value 2
	if s.Percentile == nil || *s.Percentile != 2 {
		t.Errorf("p25 = %v, want 2", s.Percentile)
	}
}

func TestComputeStats_PercentileClamped(t *testing.T) {
	p := 150.0
	s := ComputeStats([]any{1, 2, 3}, &p)
	if s.Percentile == nil || *s.Percentile != 3 {
		t.Errorf("p>100 should clamp to max: got %v", s.Percentile)
	}
}

func TestComputeStats_MixedValid(t *testing.T) {
	// Non-numeric strings are dropped; nil coerces to 0 (mirrors JS Number(null)).
	s := ComputeStats([]any{1, "x", 3, 5}, nil)
	if s == nil || s.Count != 3 {
		t.Errorf("count = %v, want 3 (drop NaN strings)", s)
	}
	if s.Min != 1 || s.Max != 5 {
		t.Errorf("min/max = %v/%v, want 1/5", s.Min, s.Max)
	}
}

func TestToFloat_Coercions(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
	}{
		{"nil-is-zero", nil, 0},
		{"bool-true", true, 1},
		{"bool-false", false, 0},
		{"int", int(7), 7},
		{"int64", int64(7), 7},
		{"uint8", uint8(7), 7},
		{"float32", float32(1.5), 1.5},
		{"empty-string-is-zero", "", 0},
		{"whitespace-string-is-zero", "   ", 0},
		{"numeric-string", "3.5", 3.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := toFloat(c.in); got != c.want {
				t.Errorf("toFloat(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestToFloat_UnsupportedTypesAreNaN(t *testing.T) {
	// Form values that aren't numbers (a table cell slice, an object,
	// a non-numeric string) must coerce to NaN so EvaluateMath blanks out
	// rather than silently treating them as zero.
	for _, in := range []any{
		"not-a-number",
		[]any{1, 2},
		map[string]any{"a": 1},
		struct{ X int }{1},
	} {
		if got := toFloat(in); !math.IsNaN(got) {
			t.Errorf("toFloat(%v) = %v, want NaN", in, got)
		}
	}
}

func TestEvaluateMath_NaNFirstOperandBlanks(t *testing.T) {
	if got := EvaluateMath("oops", "+", 1); got != "" {
		t.Errorf("NaN first operand = %v, want empty string", got)
	}
}

func TestEvaluateMath_ModByZeroBlanks(t *testing.T) {
	for _, op := range []string{"%", "mod"} {
		if got := EvaluateMath(10, op, 0); got != "" {
			t.Errorf("10 %s 0 = %v, want empty string", op, got)
		}
	}
}

func TestEvaluateMath_PadShorterWidthReturnsAsIs(t *testing.T) {
	// width <= len(s): the no-pad branch.
	if got := EvaluateMath(12345, "pad", 2); got != "12345" {
		t.Errorf("pad width<len = %v, want 12345", got)
	}
}

func TestCompare_RelationalWithNonNumericIsFalse(t *testing.T) {
	// toFloat → NaN on both sides; every NaN comparison is false.
	for _, op := range []string{"<", "<=", ">", ">="} {
		if Compare("abc", op, "def") {
			t.Errorf("Compare(abc %s def) = true, want false (NaN)", op)
		}
	}
}

func TestCompare_StrictNilHandling(t *testing.T) {
	if !Compare(nil, "===", nil) {
		t.Error("nil === nil should be true")
	}
	if Compare(nil, "===", 1) {
		t.Error("nil === 1 should be false")
	}
	if Compare(1, "===", nil) {
		t.Error("1 === nil should be false")
	}
}

func TestCompare_StrictTypeMismatch(t *testing.T) {
	// Same printed value, different concrete type: strict rejects, loose accepts.
	if Compare(1, "===", "1") {
		t.Error("1 === \"1\" should be false (type mismatch)")
	}
	if !Compare(1, "==", "1") {
		t.Error("1 == \"1\" should be true (loose)")
	}
}

func TestCompare_LooseNilEquality(t *testing.T) {
	if !Compare(nil, "==", nil) {
		t.Error("nil == nil should be true")
	}
	// int vs float with equal value: loose equal is numeric.
	if !Compare(1, "==", 1.0) {
		t.Error("1 == 1.0 should be true (numeric loose equal)")
	}
}
