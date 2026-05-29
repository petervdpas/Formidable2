// Package query is a constrained, read-only SELECT surface over the
// index module's form_values store. It is a sibling consumer of index
// alongside stat: stat turns the same datacore into chart grids, query
// turns it into row listings and ad-hoc group/count views. Query owns no
// SQL - it translates a Spec into index.ProjectRows / index.AggregateRaw
// calls and shapes the result. Scope is deliberately single-template: no
// cross-template joins, no subqueries, no user SQL. See the datacore
// boundary discussion in design notes.
package query

// Source identifies one indexed value: a scalar field, a table column
// (Col set to its positional form_values.col index), or a facet. Mirrors
// the (Kind, Key, Col) shape index.ProjectCol / index.AggDim use, so the
// translation is one-to-one.
type Source struct {
	Kind string `json:"kind"` // "field" | "facet"
	Key  string `json:"key"`
	Col  *int   `json:"col,omitempty"`
}

// Column is one projected output column: a display Header plus the
// indexed Source it reads.
type Column struct {
	Header string `json:"header"`
	Source Source `json:"source"`
}

// Filter scopes the query to rows where Source satisfies the comparison.
// Op is eq/ne (text) or lt/le/gt/ge (numeric); validation lives in the
// index layer so there is one definition of the comparison semantics.
type Filter struct {
	Source Source `json:"source"`
	Op     string `json:"op"`
	Value  string `json:"value"`
}

// Sort orders the result by a projected column (Column index into Spec
// Columns). Numeric sorts on the parsed number, so a number column
// orders 2 < 10 rather than lexically. Honored in row-listing mode;
// group mode orders by group key (explicit group-mode sort is a
// follow-up).
type Sort struct {
	Column  int  `json:"column"`
	Desc    bool `json:"desc"`
	Numeric bool `json:"numeric"`
}

// SourceInfo describes one selectable source for the query UI: its stable
// id, a display label, the Source it maps to, and the capabilities the
// dialog needs. The backend owns this list (derived from the template) so
// the frontend never reimplements which fields/columns/facets exist or how
// they behave. Fans marks a multi-valued source (a table column, or a
// list/tags/multioption field) that explodes into rows; Aggregatable marks
// a numeric source a sum/avg/min/max measure can target.
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

// Choice is one closed-set value a filter can offer as a dropdown instead
// of free text (dropdown/radio option values, or facet option labels).
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Measure is one aggregate column in group mode: a function applied to a
// source over the rows of each group. Func is count / count_distinct /
// sum / avg / min / max; Source is ignored for count and count_distinct
// (they count rows / distinct source forms). Header is the output label.
// Numeric measures coerce each cell to a number at execute time and skip
// the ones that don't parse, so a stray non-numeric value can't fail the
// whole aggregate (the "doctor" stance: apply type, tolerate outliers).
type Measure struct {
	Func   string `json:"func"`
	Source Source `json:"source"`
	Header string `json:"header"`
}

// Spec is a full query request. With no GroupBy it is a row listing
// (each form/cell a row), reusing index.ProjectRows; Distinct collapses
// the projected tuple (the flatten-list/table-and-distinct case). With
// GroupBy it is an aggregation over index.AggregateRaw: the result
// carries the group columns plus, when Count is set, a count of the rows
// contributing to each group.
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

// Cell is one output value: Text is the display string; Num carries the
// parsed number when the value had one, so the REST consumer can emit a
// real JSON number instead of a quoted string.
type Cell struct {
	Text string   `json:"text"`
	Num  *float64 `json:"num,omitempty"`
}

// Result is the query output: Columns are the header strings (group
// columns plus the count header in group mode), Rows the typed cells.
// Count is the number of result rows; Total is the template's full form
// count, a denominator for "N of M" context.
type Result struct {
	Columns []string `json:"columns"`
	Rows    [][]Cell `json:"rows"`
	Count   int      `json:"count"`
	Total   int      `json:"total"`
	// Anomalies are integrity violations found while running: a cell in a
	// typed (number/date) column that does not coerce back to its declared
	// type. The input form enforces field types, so a value that won't
	// round-trip means the stored data is corrupt, not a tolerable outlier.
	// They are surfaced, never silently dropped.
	Anomalies []Anomaly `json:"anomalies,omitempty"`
}

// Anomaly names one cell that betrayed its column's declared type.
type Anomaly struct {
	Form     string `json:"form"`
	Column   string `json:"column"`
	Value    string `json:"value"`
	Expected string `json:"expected"`
}
