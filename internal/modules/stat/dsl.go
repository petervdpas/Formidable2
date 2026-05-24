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

// StatConfig is the parsed statistical DSL: one or more measures (cell
// value layers) over zero or more dimensions (axes). No dimensions => a
// rank-0 scalar; one => a 1D array; two => a 2D matrix; and so on.
//
// Named StatConfig (not Config) to stay unambiguous inside the stat
// package, which already carries Result/Series.
type StatConfig struct {
	Measures   []Measure
	Dimensions []Dimension
}
