package stat

// Statistical DSL: the serialized form of a template's "Statistical
// Insight" objects. This file owns the DSL value types; compile.go and
// parse.go own the Compile/Parse round-trip; the engine (later) evaluates
// a Config over the index into a rank-N values grid. See
// design/statistics-dsl.md.
//
// Grammar:
//
//	object    := measure ("," measure)*  ( "by" dimension ("," dimension)* )?
//	             ( "where" filter ( "and" filter )* )?
//	filter    := source op value                     // scope, AND-chained
//	op        := "eq" | "ne" | "lt" | "le" | "gt" | "ge"
//	             // eq/ne take a quoted string; lt/le/gt/ge take a number
//	measure   := "count" "(" ")"
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

// SourceRef references a statistical source by key: a field (optionally a
// table column by its option value-key) or a facet. Column is "" for a
// scalar field or a facet.
type SourceRef struct {
	Kind   SourceKind
	Key    string
	Column string
}

// MeasureOp is the aggregation a measure applies to each cell.
type MeasureOp string

const (
	OpCount      MeasureOp = "count"
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

// Measure is one cell value layer: count() (no source), a reduce over a
// numeric field source, or percentile(source, p).
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

// Dimension is one group-by axis: a source, optionally date-binned, and
// optionally capped to its Top-N categories (ranked by the first measure,
// the tail dropped). Top 0 means all categories; valid Top is 1..20.
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

// StatConfig is the parsed statistical DSL: one or more measures (cell
// value layers) over zero or more dimensions (axes), optionally scoped by
// AND-ed equality filters. No dimensions => a rank-0 scalar; one => a 1D
// array; two => a 2D matrix; and so on.
//
// Named StatConfig (not Config) to stay unambiguous inside the stat
// package, which already carries Result/Series.
type StatConfig struct {
	Measures   []Measure
	Dimensions []Dimension
	Filters    []Filter
}
