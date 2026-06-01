package stat

// Statistical DSL value types (the serialized form of a template's
// "Statistical Insight" objects). compile.go/parse.go own the round-trip;
// the engine evaluates a Config into a rank-N grid. See
// design/statistics-dsl.md.
//
// Grammar:
//
//	object    := measure ("," measure)*  ( "by" dimension ("," dimension)* )?
//	             ( "where" filter ( "and" filter )* )?
//	             ( "scale" str )?                     // weighting object, by name
//	             ( "pct" base )?                      // percentage denominator
//	base      := "distribution" | "forms" | "none"   // default: distribution
//	filter    := source op value                     // scope, AND-chained
//	op        := "eq" | "ne" | "lt" | "le" | "gt" | "ge"
//	             // eq/ne take a quoted string; lt/le/gt/ge take a number
//	measure   := "count" "(" ")"
//	           | "records" "(" ")"              // distinct forms, not rows
//	           | reduce "(" numSource ")"
//	           | "percentile" "(" numSource "," number ")"
//	reduce    := "sum" | "avg" | "min" | "max" | "median" | "stddev"
//	dimension := source bin? ( "top" number )?       // top N: 1..20
//	source    := "F[" str "]" ( "[" str "]" )?      // field, or table column by value-key
//	           | "Facet[" str "]"                    // facet
//	numSource := "F[" str "]" ( "[" str "]" )?       // must be a field source
//	bin       := "@year" | "@month" | "@day"         // date sources only

// SourceKind distinguishes a field source from a facet source.
type SourceKind string

const (
	SourceField SourceKind = "field"
	SourceFacet SourceKind = "facet"
)

// SourceRef references a source by key: a field (optionally a table column
// by option value-key) or a facet. Column is "" for a scalar field or facet.
type SourceRef struct {
	Kind   SourceKind
	Key    string
	Column string
}

// MeasureOp is the aggregation a measure applies to each cell.
type MeasureOp string

const (
	OpCount      MeasureOp = "count"
	OpRecords    MeasureOp = "records"
	OpSum        MeasureOp = "sum"
	OpAvg        MeasureOp = "avg"
	OpMin        MeasureOp = "min"
	OpMax        MeasureOp = "max"
	OpMedian     MeasureOp = "median"
	OpStddev     MeasureOp = "stddev"
	OpPercentile MeasureOp = "percentile"
)

// reduceOps are the measures that reduce a single numeric field source.
var reduceOps = map[MeasureOp]bool{
	OpSum: true, OpAvg: true, OpMin: true, OpMax: true, OpMedian: true, OpStddev: true,
}

// Measure is one cell value layer: count(), records(), a reduce over a
// numeric field source, or percentile(source, p). count() and records()
// diverge only on a fanned table-column source: count() tallies rows,
// records() tallies distinct forms.
type Measure struct {
	Op     MeasureOp
	Source *SourceRef // nil only for count
	Arg    *float64   // percentile p; nil otherwise
}

// Bin buckets a date dimension; "" means no binning (group by raw value).
type Bin string

const (
	BinNone  Bin = ""
	BinYear  Bin = "year"
	BinMonth Bin = "month"
	BinDay   Bin = "day"
)

var validBins = map[Bin]bool{BinYear: true, BinMonth: true, BinDay: true}

// Dimension is one group-by axis: a source, optionally date-binned and
// capped to Top-N categories (ranked by the first measure). Top 0 = all,
// otherwise 1..20.
type Dimension struct {
	Source SourceRef
	Bin    Bin
	Top    int
}

// FilterOp is a where-clause comparison operator.
type FilterOp string

const (
	FilterEq FilterOp = "eq"
	FilterNe FilterOp = "ne"
	FilterLt FilterOp = "lt"
	FilterLe FilterOp = "le"
	FilterGt FilterOp = "gt"
	FilterGe FilterOp = "ge"
)

// equalityOps compare the stored text value (categorical, also dates/ISO);
// Value is a string literal. comparisonOps compare the numeric value;
// Value is a number literal.
var equalityOps = map[FilterOp]bool{FilterEq: true, FilterNe: true}
var comparisonOps = map[FilterOp]bool{FilterLt: true, FilterLe: true, FilterGt: true, FilterGe: true}

// Filter scopes which rows count, before grouping: keep only those where
// Source <op> Value holds. AND-chained. Equality ops match the text
// value; comparison ops match the numeric value.
type Filter struct {
	Source SourceRef
	Op     FilterOp
	Value  string
}

// PercentBase selects the denominator the engine uses for each cell's
// computed percentage. It is an authored setting on the object (the builder
// sets it, the DSL carries it), not a render-time choice.
type PercentBase string

const (
	// PctDistribution: share of the measure's total across this grid's cells
	// (categories sum to 100%). The default; an empty PercentBase means this.
	PctDistribution PercentBase = "distribution"
	// PctForms: share of all forms (grid Total), so e.g. partially-filled or
	// multi-value distributions read against the record count.
	PctForms PercentBase = "forms"
	// PctNone: no percentage computed.
	PctNone PercentBase = "none"
)

var validPercentBases = map[PercentBase]bool{PctDistribution: true, PctForms: true, PctNone: true}

// StatConfig is the parsed DSL: measures over zero or more dimensions,
// optionally AND-filtered, with a percent base. Dimension count sets the
// rank (0 = scalar, 1 = array, 2 = matrix, …). Named StatConfig, not
// Config, to disambiguate within the stat package.
type StatConfig struct {
	Measures   []Measure
	Dimensions []Dimension
	Filters    []Filter
	Percent    PercentBase // "" means PctDistribution
	// Scales names zero or more scaling objects that weight count()/records()
	// per form; their per-record factors multiply. Empty means unweighted.
	// Only the references are carried, resolved at evaluate time (each
	// referenced object owns its source + factor map).
	Scales []string
}
