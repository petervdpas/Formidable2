package stat

import (
	"fmt"
	"strconv"
	"strings"
)

// Compile renders a StatConfig into the canonical statistical-DSL string.
// It is the inverse of Parse; Compile(Parse(Compile(x))) == Compile(x)
// (string equality is the round-trip contract, matching the expression
// builder). Structural validation only: it does not know field types, so
// bin-on-non-date and numeric-source-typing are the engine's concern.
func Compile(cfg StatConfig) (string, error) {
	if len(cfg.Measures) == 0 {
		return "", fmt.Errorf("stat dsl: at least one measure required")
	}
	parts := make([]string, 0, len(cfg.Measures))
	for _, m := range cfg.Measures {
		s, err := compileMeasure(m)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	out := strings.Join(parts, ", ")
	if len(cfg.Dimensions) > 0 {
		dims := make([]string, 0, len(cfg.Dimensions))
		for _, d := range cfg.Dimensions {
			s, err := compileDimension(d)
			if err != nil {
				return "", err
			}
			dims = append(dims, s)
		}
		out += " by " + strings.Join(dims, ", ")
	}
	if len(cfg.Filters) > 0 {
		preds := make([]string, 0, len(cfg.Filters))
		for _, f := range cfg.Filters {
			s, err := compileFilter(f)
			if err != nil {
				return "", err
			}
			preds = append(preds, s)
		}
		out += " where " + strings.Join(preds, " and ")
	}
	// Scale reference before pct (fixed canonical order). A quoted name so
	// hyphenated object names round-trip.
	if cfg.Scale != "" {
		out += " scale " + strconv.Quote(cfg.Scale)
	}
	// The percentage base is canonical only when non-default: "" and
	// "distribution" both omit the clause, so round-trip stays stable.
	if cfg.Percent != "" && cfg.Percent != PctDistribution {
		if !validPercentBases[cfg.Percent] {
			return "", fmt.Errorf("stat dsl: invalid percent base %q", cfg.Percent)
		}
		out += " pct " + string(cfg.Percent)
	}
	return out, nil
}

func compileFilter(f Filter) (string, error) {
	src := compileSource(f.Source)
	switch {
	case equalityOps[f.Op]:
		return src + " " + string(f.Op) + " " + strconv.Quote(f.Value), nil
	case comparisonOps[f.Op]:
		if _, err := strconv.ParseFloat(f.Value, 64); err != nil {
			return "", fmt.Errorf("stat dsl: %s needs a numeric value, got %q", f.Op, f.Value)
		}
		return src + " " + string(f.Op) + " " + f.Value, nil
	default:
		return "", fmt.Errorf("stat dsl: unknown filter operator %q", f.Op)
	}
}

func compileMeasure(m Measure) (string, error) {
	switch {
	case m.Op == OpCount || m.Op == OpRecords:
		if m.Source != nil {
			return "", fmt.Errorf("stat dsl: %s() takes no source", m.Op)
		}
		return string(m.Op) + "()", nil
	case m.Op == OpPercentile:
		if err := requireFieldSource(m.Source, "percentile"); err != nil {
			return "", err
		}
		if m.Arg == nil {
			return "", fmt.Errorf("stat dsl: percentile requires a percentage argument")
		}
		return fmt.Sprintf("percentile(%s, %s)", compileSource(*m.Source), formatNum(*m.Arg)), nil
	case reduceOps[m.Op]:
		if err := requireFieldSource(m.Source, string(m.Op)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s(%s)", m.Op, compileSource(*m.Source)), nil
	default:
		return "", fmt.Errorf("stat dsl: unknown measure %q", m.Op)
	}
}

func requireFieldSource(s *SourceRef, op string) error {
	if s == nil {
		return fmt.Errorf("stat dsl: %s requires a source", op)
	}
	if s.Kind != SourceField {
		return fmt.Errorf("stat dsl: %s source must be a field", op)
	}
	return nil
}

func compileSource(s SourceRef) string {
	if s.Kind == SourceFacet {
		return "Facet[" + strconv.Quote(s.Key) + "]"
	}
	out := "F[" + strconv.Quote(s.Key) + "]"
	if s.Column != "" {
		out += "[" + strconv.Quote(s.Column) + "]"
	}
	return out
}

func compileDimension(d Dimension) (string, error) {
	out := compileSource(d.Source)
	if d.Bin != BinNone {
		if !validBins[d.Bin] {
			return "", fmt.Errorf("stat dsl: invalid bin %q", d.Bin)
		}
		out += "@" + string(d.Bin)
	}
	if d.Top != 0 {
		if d.Top < 1 || d.Top > 20 {
			return "", fmt.Errorf("stat dsl: top must be between 1 and 20, got %d", d.Top)
		}
		out += " top " + strconv.Itoa(d.Top)
	}
	return out, nil
}

// formatNum renders a percentile argument so Parse round-trips it
// identically (shortest form that reparses to the same float).
func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
