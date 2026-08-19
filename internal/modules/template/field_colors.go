package template

// FieldColor is the palette one field-type slot renders in: the row background,
// its left rail, the (TYPE) badge, and the text that sits on them.
//
// The values live here rather than in a stylesheet because the frontend does not
// own this data: a second UI (a different framework, a web client, an export)
// asks the backend for the field-type registry and gets everything it needs to
// render a field, colour included. A CSS token alone would only be meaningful to
// a UI that already shipped a matching stylesheet, which is the frontend owning
// data again. Facets can hand out bare tokens ("red", "teal") because those mean
// something anywhere; "api-client" does not.
//
// One palette, no light/dark variants: these read on either theme.
type FieldColor struct {
	Bg     string `json:"bg"`
	Border string `json:"border"`
	Badge  string `json:"badge"`
	Text   string `json:"text"`
}

// fieldTypePalette maps a colour slot to its values. Slots are named after the
// type that uses them, except looper, which the whole loop construct shares.
// Plugin widget kinds (progressbar, statusmessage, chart) are not field types;
// their palette belongs to the formwidget module.
var fieldTypePalette = map[string]FieldColor{
	"api":         {Bg: "#29434e", Border: "#80deea", Badge: "#4dd0e1", Text: "#f7fbfc"},
	"api-client":  {Bg: "#103642", Border: "#22b8cf", Badge: "#0f7d8c", Text: "#eaf7fa"},
	"boolean":     {Bg: "#2e7d32", Border: "#4caf50", Badge: "#43a047", Text: "#ffffff"},
	"date":        {Bg: "#229388", Border: "#4dd0e1", Badge: "#44c5d1", Text: "#ffffff"},
	"dropdown":    {Bg: "#5e178b", Border: "#ba68c8", Badge: "#a05db5", Text: "#ffffff"},
	"event":       {Bg: "#303f9c", Border: "#7986cb", Badge: "#5361c9", Text: "#ffffff"},
	"facet":       {Bg: "#d4b73a", Border: "#b8992e", Badge: "#c9a82a", Text: "#222222"},
	"file-path":   {Bg: "#c2620a", Border: "#f59e0b", Badge: "#d97706", Text: "#ffffff"},
	"folder-path": {Bg: "#8a4509", Border: "#c2620a", Badge: "#a3580d", Text: "#ffffff"},
	"formula":     {Bg: "#8f5468", Border: "#c391a3", Badge: "#a56a7e", Text: "#ffffff"},
	"guid":        {Bg: "#444c54", Border: "#888c95", Badge: "#888c95", Text: "#ffffff"},
	"image":       {Bg: "#bdbdbd", Border: "#cfd8dc", Badge: "#cfd8dc", Text: "#222222"},
	"link":        {Bg: "#7b1e3a", Border: "#d46a85", Badge: "#d46a85", Text: "#ffffff"},
	"list":        {Bg: "#6fbf73", Border: "#aed581", Badge: "#9dcf6e", Text: "#ffffff"},
	"looper":      {Bg: "#b9e2c0", Border: "#999999", Badge: "#999999", Text: "#222222"},
	"mermaid":     {Bg: "#c2185b", Border: "#f06292", Badge: "#e91e63", Text: "#ffffff"},
	"multioption": {Bg: "#4e729a", Border: "#5bc0de", Badge: "#5bc0de", Text: "#ffffff"},
	"number":      {Bg: "#3a94e4", Border: "#64b5f6", Badge: "#5aaae9", Text: "#ffffff"},
	"project":     {Bg: "#2a4d69", Border: "#6b9bc3", Badge: "#3d6d94", Text: "#ffffff"},
	"radio":       {Bg: "#cc4417", Border: "#ff7043", Badge: "#f46333", Text: "#ffffff"},
	"range":       {Bg: "#3c3f77", Border: "#7986cb", Badge: "#7986cb", Text: "#ffffff"},
	"sequence":    {Bg: "#515da7", Border: "#97a0e0", Badge: "#7c88d4", Text: "#ffffff"},
	"slide":       {Bg: "#6d4aa7", Border: "#b39ddb", Badge: "#9575cd", Text: "#ffffff"},
	"slideset":    {Bg: "#1f7a6d", Border: "#6fc9bb", Badge: "#3fa595", Text: "#ffffff"},
	"table":       {Bg: "#ee9b84", Border: "#ffccbc", Badge: "#f5bfa9", Text: "#222222"},
	"tags":        {Bg: "#4caf50", Border: "#81c784", Badge: "#66bb6a", Text: "#ffffff"},
	"text":        {Bg: "#1565c0", Border: "#2196f3", Badge: "#1976d2", Text: "#ffffff"},
	"textarea":    {Bg: "#6c4ab0", Border: "#9575cd", Badge: "#8567b9", Text: "#ffffff"},
}

// Palette returns the values for a colour slot, and whether the slot exists.
func Palette(token string) (FieldColor, bool) {
	c, ok := fieldTypePalette[token]
	return c, ok
}
