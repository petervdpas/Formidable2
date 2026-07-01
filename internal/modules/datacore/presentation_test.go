package datacore

import (
	"errors"
	"testing"
)

// With the exclusion predicate refusing a template, every view and graph entry
// point returns ErrPresentationExcluded before building a tensor.
func TestDatacore_ExclusionRefusesViewsAndGraphs(t *testing.T) {
	svc := NewService(func(string) Loader { return staticLoader{} },
		WithExclusion(func(string) bool { return true }))

	if _, err := svc.Count("talk", ""); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Count err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := svc.Graph("talk", 0); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Graph err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := svc.GraphFrom("talk", "x", 1); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("GraphFrom err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := svc.GraphFromDepth("talk", "x", 1, 0); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("GraphFromDepth err = %v, want ErrPresentationExcluded", err)
	}
	// AggregateRaw + Summarize are the data-access entries the stat engine uses,
	// so guarding them also keeps presentations out of statistics.
	if _, err := svc.AggregateRaw("talk", nil, nil, nil); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("AggregateRaw err = %v, want ErrPresentationExcluded", err)
	}
	if _, err := svc.Summarize("talk", "", ""); !errors.Is(err, ErrPresentationExcluded) {
		t.Errorf("Summarize err = %v, want ErrPresentationExcluded", err)
	}
}

// A predicate that allows the template (or no predicate at all) does not raise
// the exclusion error.
func TestDatacore_ExclusionAllowsOtherTemplates(t *testing.T) {
	svc := NewService(func(string) Loader { return staticLoader{} },
		WithExclusion(func(string) bool { return false }))
	if _, err := svc.Graph("other", 0); errors.Is(err, ErrPresentationExcluded) {
		t.Error("allowed template was refused")
	}
	// No predicate wired: also allowed.
	svc2 := NewService(func(string) Loader { return staticLoader{} })
	if _, err := svc2.Count("other", ""); errors.Is(err, ErrPresentationExcluded) {
		t.Error("no-predicate service refused a template")
	}
}
