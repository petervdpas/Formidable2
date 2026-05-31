// Package query is a constrained, read-only SELECT surface over a template's
// form data: row listings and ad-hoc group/count views. Single-template by
// design: no cross-template joins, no subqueries, no user SQL.
package query

// Source identifies one value: a scalar field, a table column (Col = positional
// column index), or a facet.
type Source struct {
	Kind string `json:"kind"` // "field" | "facet"
	Key  string `json:"key"`
	Col  *int   `json:"col,omitempty"`
}

// Column is one projected output column: a Header plus the Source it reads.
type Column struct {
	Header string `json:"header"`
	Source Source `json:"source"`
}

// Filter scopes the query to rows where Source satisfies the comparison.
// Op is eq/ne (text) or lt/le/gt/ge (numeric).
type Filter struct {
	Source Source `json:"source"`
	Op     string `json:"op"`
	Value  string `json:"value"`
}

// Sort orders by a projected column (index into Spec.Columns). Numeric sorts on
// the parsed number (2 < 10, not lexical). Row-listing mode; group mode orders
// by group key.
type Sort struct {
	Column  int  `json:"column"`
	Desc    bool `json:"desc"`
	Numeric bool `json:"numeric"`
}

// SourceInfo describes one selectable source for the query UI. Fans marks a
// multi-valued source (table column, or list/tags/multioption) that explodes
// into rows; Aggregatable marks a numeric source a measure can target.
type SourceInfo struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Source       Source   `json:"source"`
	Numeric      bool     `json:"numeric"`
	Date         bool     `json:"date"`
	Fans         bool     `json:"fans"`
	Aggregatable bool     `json:"aggregatable"`
	Choices      []Choice `json:"choices,omitempty"`
}

// Choice is one closed-set value a filter can offer as a dropdown.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Measure is one aggregate column in group mode. Func is count / count_distinct
// / sum / avg / min / max; Source is ignored for the counts. Numeric measures
// coerce each cell and skip non-numeric values (tolerate outliers, the doctor
// stance).
type Measure struct {
	Func   string `json:"func"`
	Source Source `json:"source"`
	Header string `json:"header"`
}

// Spec is a full query request. No GroupBy is a row listing (Distinct collapses
// the projected tuple); GroupBy aggregates, with Count adding a per-group row
// count.
type Spec struct {
	Template    string    `json:"template"`
	Columns     []Column  `json:"columns"`
	Filters     []Filter  `json:"filters,omitempty"`
	Distinct    bool      `json:"distinct,omitempty"`
	GroupBy     []int     `json:"groupBy,omitempty"`
	Measures    []Measure `json:"measures,omitempty"`
	Count       bool      `json:"count,omitempty"`
	CountHeader string    `json:"countHeader,omitempty"`
	OrderBy     []Sort    `json:"orderBy,omitempty"`
	Limit       int       `json:"limit,omitempty"`
}

// Cell is one output value: Text plus Num when the value parsed, so a REST
// consumer can emit a real JSON number instead of a quoted string.
type Cell struct {
	Text string   `json:"text"`
	Num  *float64 `json:"num,omitempty"`
}

// Result is the query output. Count is the result row count; Total is the
// template's full form count, a denominator for "N of M".
type Result struct {
	Columns []string `json:"columns"`
	Rows    [][]Cell `json:"rows"`
	Count   int      `json:"count"`
	Total   int      `json:"total"`
	// Anomalies are typed-column cells that do not coerce back to their declared
	// type. The input form enforces field types, so a non-round-tripping value
	// means corrupt data, not a tolerable outlier. Surfaced, never dropped.
	Anomalies []Anomaly `json:"anomalies,omitempty"`
}

// Anomaly names one cell that betrayed its column's declared type.
type Anomaly struct {
	Form     string `json:"form"`
	Column   string `json:"column"`
	Value    string `json:"value"`
	Expected string `json:"expected"`
}
