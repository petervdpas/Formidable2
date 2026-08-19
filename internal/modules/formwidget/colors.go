package formwidget

// Color is the palette one widget kind renders in: the row background in the
// plugin form editor, its left rail, the (KIND) badge, and the text on them.
//
// The values live here rather than in a stylesheet for the same reason the
// field-type palette does: a UI asks the backend what a widget is and gets
// everything needed to draw it, colour included. A bare token would only mean
// something to a UI that already shipped a matching stylesheet.
//
// Widgets are display chrome, not data fields, so they share the brand accent
// (#2563eb) in three shades kept clear of the field blues (text #1565c0,
// number #3a94e4).
type Color struct {
	Bg     string `json:"bg"`
	Border string `json:"border"`
	Badge  string `json:"badge"`
	Text   string `json:"text"`
}

// Descriptor is the per-kind record a UI renders from. Color is the palette slot
// (named after the kind, since widget slots are never shared) and Palette its
// values; both live in one CSS variable namespace with the field-type slots, so
// a kind must not take a field type's name.
type Descriptor struct {
	Kind    Kind   `json:"kind"`
	Color   string `json:"color"`
	Palette Color  `json:"palette"`
}

// kindOrder is the stable render order for pickers and lists.
var kindOrder = []Kind{KindProgressBar, KindStatusMessage, KindChart}

var palette = map[Kind]Color{
	KindProgressBar:   {Bg: "#2563eb", Border: "#60a5fa", Badge: "#3b82f6", Text: "#ffffff"},
	KindStatusMessage: {Bg: "#1e3a8a", Border: "#3b82f6", Badge: "#2563eb", Text: "#ffffff"},
	KindChart:         {Bg: "#0369a1", Border: "#38bdf8", Badge: "#0284c7", Text: "#ffffff"},
}

// AllKinds returns the closed kind set in render order.
func AllKinds() []Kind {
	out := make([]Kind, len(kindOrder))
	copy(out, kindOrder)
	return out
}

// Descriptors returns every kind with its palette, in render order.
func Descriptors() []Descriptor {
	out := make([]Descriptor, 0, len(kindOrder))
	for _, k := range kindOrder {
		out = append(out, Descriptor{
			Kind:    k,
			Color:   string(k),
			Palette: palette[k],
		})
	}
	return out
}
