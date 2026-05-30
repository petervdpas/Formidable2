package index

import "database/sql"

// Statistics-contract types. The index no longer computes statistics (datacore
// does), but these data shapes remain the currency the stat.Index interface and
// the datacore adapter speak in, so they live on as plain structs here next to
// the other index data shapes (FormRow, FormValueRow). Nothing in this package
// produces them anymore.

// Bucket is one category and its count in a rank-1 distribution.
type Bucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// CrossCell is one (a, b, count) triple from a cross-tab between two keys.
type CrossCell struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Count int    `json:"count"`
}

// AggDim is one grouping axis for a raw aggregation: a scalar field's value, a
// table column's value (Col set), or a facet's selected option, optionally
// date-binned. Col is the positional column index (nil = scalar; ignored for
// facets). DateWidth is the ISO-date prefix length (4 year, 7 month, 10 day).
type AggDim struct {
	Kind      string // "field" | "facet"
	Key       string
	Col       *int
	DateWidth int
}

// AggNum is a numeric source for a reducing measure: a scalar field (Col nil)
// or a table column (Col set).
type AggNum struct {
	Key string
	Col *int
}

// AggFilter scopes an aggregation to rows where the source satisfies the
// comparison. Op is "eq"/"ne" (text) or "lt"/"le"/"gt"/"ge" (numeric).
type AggFilter struct {
	Kind  string // "field" | "facet"
	Key   string
	Col   *int
	Op    string
	Value string
}

// StatRawRow is one form's contribution to an aggregation: the source form's
// filename, the category label per dimension, and the numeric value of each
// requested source (invalid when the form has no value for it).
type StatRawRow struct {
	Form string
	Dims []string
	Nums []sql.NullFloat64
}
