package connection

// HTTP methods are a closed vocabulary, and an operation list is unreadable
// without telling them apart at a glance. The colours live here rather than in
// a stylesheet for the same reason the field-type palette does: a different UI
// over the same backend has to render a method badge without reimplementing
// what a method is.

// MethodColor is one badge's values, ready to paint with.
type MethodColor struct {
	Bg     string `json:"bg"`
	Border string `json:"border"`
	Text   string `json:"text"`
}

// MethodDescriptor is one method with the colours it renders in.
type MethodDescriptor struct {
	Method  string      `json:"method"`
	Palette MethodColor `json:"palette"`
}

// methodPalette follows the convention every API tool has settled on, so a
// green POST and a red DELETE read the same way here as everywhere else.
var methodPalette = map[string]MethodColor{
	"GET":     {Bg: "#61affe", Border: "#4a97e5", Text: "#ffffff"},
	"POST":    {Bg: "#49cc90", Border: "#38b47a", Text: "#ffffff"},
	"PUT":     {Bg: "#fca130", Border: "#e08a1c", Text: "#ffffff"},
	"PATCH":   {Bg: "#50e3c2", Border: "#38c9a9", Text: "#10312b"},
	"DELETE":  {Bg: "#f93e3e", Border: "#dc2626", Text: "#ffffff"},
	"HEAD":    {Bg: "#9012fe", Border: "#7a0ddb", Text: "#ffffff"},
	"OPTIONS": {Bg: "#0d5aa7", Border: "#094a8c", Text: "#ffffff"},
	"TRACE":   {Bg: "#6b7280", Border: "#565d69", Text: "#ffffff"},
}

// Methods returns every method a catalog can contain, in the order operations
// are sorted within one path, each with its palette.
func Methods() []MethodDescriptor {
	out := make([]MethodDescriptor, 0, len(methodOrder))
	for _, m := range methodOrder {
		out = append(out, MethodDescriptor{Method: m, Palette: methodPalette[m]})
	}
	return out
}
