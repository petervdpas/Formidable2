package template

import "encoding/json"

// The slide field's value is a free-positioned canvas of typed content blocks.
// A block is "an existing-field-typed value plus a position": its Kind is a
// field-type id, so the same renderer/editor used elsewhere handles its Content.

// SlideCanvasWidth/Height is the fixed authoring stage; block positions are
// pixels within it. The editor scales this to fit; rendering uses it 1:1.
const (
	SlideCanvasWidth  = 1280
	SlideCanvasHeight = 720
)

// SlideBlock is one positioned content block. Kind is a field-type id
// (textarea=markdown, mermaid, image, table, list); Content is that type's
// value. Z-order is the block's index in SlideDoc.Blocks.
type SlideBlock struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Content any    `json:"content"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	W       int    `json:"w"`
	H       int    `json:"h"`
}

// SlideDoc is the stored value of a slide field. It is an object (not a bare
// array) so slide-level options (background, transition) can be added later
// without changing the shape.
type SlideDoc struct {
	Blocks []SlideBlock `json:"blocks"`
}

// SlideBlockKindDescriptor names one kind the block palette offers. Name is the
// field-type id used to edit and render the block; LabelKey is its i18n label.
type SlideBlockKindDescriptor struct {
	Name     string `json:"name"`
	LabelKey string `json:"label_key"`
}

// builtinSlideBlockKinds is the canonical palette; display order is significant.
// Each Name is an existing field type, so blocks reuse that type's FormField
// component (edit) and emitter (render). Markdown blocks use the textarea type.
var builtinSlideBlockKinds = []SlideBlockKindDescriptor{
	{Name: "textarea", LabelKey: "workspace.templates.slide.kind.markdown"},
	{Name: "mermaid", LabelKey: "workspace.templates.slide.kind.mermaid"},
	{Name: "image", LabelKey: "workspace.templates.slide.kind.image"},
	{Name: "table", LabelKey: "workspace.templates.slide.kind.table"},
	{Name: "list", LabelKey: "workspace.templates.slide.kind.list"},
}

// SlideBlockKinds returns a defensive copy of the block palette (Wails-exposed
// so the editor reads the set from the backend, never a hardcoded JS list).
func SlideBlockKinds() []SlideBlockKindDescriptor {
	out := make([]SlideBlockKindDescriptor, len(builtinSlideBlockKinds))
	copy(out, builtinSlideBlockKinds)
	return out
}

// IsSlideBlockKind reports whether kind is an allowed block kind.
func IsSlideBlockKind(kind string) bool {
	for _, k := range builtinSlideBlockKinds {
		if k.Name == kind {
			return true
		}
	}
	return false
}

// ParseSlideDoc decodes a stored slide value (a decoded map[string]any) into a
// SlideDoc. A nil value is an empty doc. Round-trips via JSON so nested block
// content (e.g. a table's 2D array) is preserved exactly.
func ParseSlideDoc(v any) (SlideDoc, error) {
	var doc SlideDoc
	if v == nil {
		return doc, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}
