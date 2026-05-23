package stat

import (
	"math"
	"testing"
)

func TestSummarize_Empty(t *testing.T) {
	if _, ok := Summarize(nil, nil); ok {
		t.Error("empty input should return ok=false")
	}
}

func TestSummarize_Basic(t *testing.T) {
	s, ok := Summarize([]float64{10, 20, 30}, nil)
	if !ok {
		t.Fatal("ok=false for non-empty input")
	}
	if s.Count != 3 || s.Min != 10 || s.Max != 30 || s.Sum != 60 || s.Avg != 20 || s.Median != 20 {
		t.Errorf("summary = %+v", s)
	}
	// sample stddev of {10,20,30} = 10
	if math.Abs(s.Stddev-10) > 1e-9 {
		t.Errorf("stddev = %v, want 10", s.Stddev)
	}
}

func TestSummarize_MedianEven(t *testing.T) {
	s, _ := Summarize([]float64{1, 2, 3, 4}, nil)
	if s.Median != 2.5 {
		t.Errorf("median = %v, want 2.5", s.Median)
	}
}

func TestSummarize_Percentile(t *testing.T) {
	p := 50.0
	s, _ := Summarize([]float64{1, 2, 3, 4, 5}, &p)
	if s.Percentile == nil || *s.Percentile != 3 {
		t.Errorf("p50 = %v, want 3", s.Percentile)
	}
}

func TestSummarize_PercentileClamped(t *testing.T) {
	p := 150.0
	s, _ := Summarize([]float64{1, 2, 3}, &p)
	if s.Percentile == nil || *s.Percentile != 3 {
		t.Errorf("p150 clamped = %v, want 3 (max)", s.Percentile)
	}
}
