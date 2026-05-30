package stat

import "fmt"

// Scaling objects: a reusable weighting applied to a plain object's
// count()/records() contributions. Each contributing unit (a row for count, a
// distinct form for records) adds a factor looked up from a per-form
// categorical value instead of adding 1. The headline use is a risk/urgency
// score: weight applications by their fcdm-coverage facet so low coverage
// counts heavier than high coverage.
//
// A scaling is its own named statistical object (like a composite). A plain
// object references one by name through the DSL `scale "<name>"` clause; the
// engine resolves the name to this Scaling at evaluate time. Storing it once
// keeps the weight map a single source of truth across every chart that
// references it.

// WeightEntry maps one option value to its multiplier. Label is the stored
// value the dimension carries: a facet's selected option label, or a
// dropdown/radio field's option value. Kept an ordered slice (not a map) so
// the spec serialises deterministically.
type WeightEntry struct {
	Label  string  `json:"label"`
	Factor float64 `json:"factor"`
}

// Scaling is the resolved weighting: the per-form source, the option->factor
// map (as an ordered slice), and the factor for any option not listed (and for
// forms with no value). Source must be a per-form facet or scalar field, never
// a table column (whose value is per-row, not per-form).
type Scaling struct {
	Source  SourceRef     `json:"source"`
	Weights []WeightEntry `json:"weights"`
	Default float64       `json:"default"`
}

// validate enforces the per-form source rule. A table-column source (Column
// set) is rejected: its value fans out per row, so it has no single per-form
// weight. Facets and scalar fields are per-form and allowed.
func (s Scaling) validate() error {
	if s.Source.Kind != SourceFacet && s.Source.Kind != SourceField {
		return fmt.Errorf("stat: scaling source kind %q invalid (want field or facet)", s.Source.Kind)
	}
	if s.Source.Key == "" {
		return fmt.Errorf("stat: scaling source has no key")
	}
	if s.Source.Column != "" {
		return fmt.Errorf("stat: scaling source must be a per-form facet or scalar field, not a table column")
	}
	return nil
}

// weightMap indexes the ordered weights by label for O(1) lookup.
func (s Scaling) weightMap() map[string]float64 {
	m := make(map[string]float64, len(s.Weights))
	for _, w := range s.Weights {
		m[w.Label] = w.Factor
	}
	return m
}

// ScalingSource resolves a scaling object by name to its weighting.
type ScalingSource interface {
	Scaling(name string) (*Scaling, error)
}
