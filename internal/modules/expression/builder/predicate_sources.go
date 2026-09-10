package builder

// PredicateSourceOption is one entry the rule editor's "add predicate for..."
// picker offers: the key it compiles to as F["key"], a display label, the
// declared type the caller hands back when asking for a default Predicate, and
// a group so the UI can separate real fields from formulas. Assembled by the
// Service (it has the template) from the predicateable fields plus the
// predicateable formulas.
type PredicateSourceOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Group string `json:"group"` // "field" | "formula"
}
