package expression

// FunctionDoc is one insertable item for the formula editor: a display name, the
// snippet inserted into the expression, a category for grouping in the UI, and a
// short description. The backend owns this catalog so the editor's palettes
// reflect the engine's real capabilities rather than a hardcoded frontend list.
type FunctionDoc struct {
	Name        string `json:"name"`
	Snippet     string `json:"snippet"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Functions returns the curated catalog of functions and control structures the
// formula editor offers. Names and snippets are expression-engine syntax (not
// translated); every math/date/text entry is exercised by TestFunctions_BuiltinsEvaluate
// so the editor never offers something the engine would reject.
func Functions() []FunctionDoc {
	return []FunctionDoc{
		// Control structures (expr-lang syntax).
		{"if / then / else", "cond ? a : b", "control", "Choose a value by condition: a when cond is true, otherwise b"},
		{"equals", " == ", "control", "True when two values are equal"},
		{"not equals", " != ", "control", "True when two values differ"},
		{"greater than", " > ", "control", "Numeric: left is larger"},
		{"less than", " < ", "control", "Numeric: left is smaller"},
		{"and", " && ", "control", "True when both sides are true"},
		{"or", " || ", "control", "True when either side is true"},

		// Math.
		{"max", "max(a, b)", "math", "The larger of two numbers"},
		{"min", "min(a, b)", "math", "The smaller of two numbers"},
		{"abs", "abs(x)", "math", "Absolute value"},
		{"round", "round(x)", "math", "Round to the nearest integer"},
		{"sum", "sum(F[\"list\"])", "math", "Total of a list of numbers"},
		{"mean", "mean(F[\"list\"])", "math", "Average of a list of numbers"},

		// Text.
		{"str", "str(x)", "text", "Value as text (numbers/dates become strings)"},
		{"default", "defaultText(x, fallback)", "text", "Fallback value when empty"},
		{"notEmpty", "notEmpty(x)", "text", "True when the value is set"},

		// Date.
		{"today", "today()", "date", "Today's date (YYYY-MM-DD)"},
		{"ageInDays", "ageInDays(date)", "date", "Days elapsed since a date"},
		{"daysBetween", "daysBetween(a, b)", "date", "Days from date a to date b"},
		{"isOverdue", "isOverdue(date)", "date", "True when a date is in the past"},
	}
}
