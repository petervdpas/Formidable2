package stat

// Builder metadata for the Statistical Engine's visual builder. The
// frontend renders its op/bin pickers from these catalogs so it never
// hardcodes the engine's vocabulary or input rules; only the wording
// (i18n) lives on the UI side.

// MeasureOpDescriptor describes one measure operation: the op id plus
// whether it needs a numeric field source and a numeric argument.
type MeasureOpDescriptor struct {
	Op          MeasureOp `json:"op"`
	NeedsSource bool      `json:"needs_source"`
	NeedsArg    bool      `json:"needs_arg"`
}

// measureOpOrder is the catalog order the builder presents.
var measureOpOrder = []MeasureOp{
	OpCount, OpSum, OpAvg, OpMin, OpMax, OpMedian, OpStddev, OpPercentile,
}

// MeasureOps returns the ordered measure catalog with input rules. The
// rules are derived from the same facts Compile/Parse enforce (count
// takes no source; percentile takes an argument; every other op reduces
// a numeric source), so there is one expression of the rule set.
func MeasureOps() []MeasureOpDescriptor {
	out := make([]MeasureOpDescriptor, 0, len(measureOpOrder))
	for _, op := range measureOpOrder {
		out = append(out, MeasureOpDescriptor{
			Op:          op,
			NeedsSource: op != OpCount,
			NeedsArg:    op == OpPercentile,
		})
	}
	return out
}

// Bins is the ordered catalog of date-bin options (including "none").
func Bins() []Bin { return []Bin{BinNone, BinYear, BinMonth, BinDay} }
