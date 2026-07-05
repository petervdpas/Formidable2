package template

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The slide field's value is a free-positioned canvas of reveal.js content
// elements. A block's Kind is a reveal element type (text, image, video, embed,
// code, math, table, list, quote, mermaid); the editor and renderer map each to
// the right editor and reveal-appropriate markup. It compiles to a reveal
// <section>, so backgrounds/fragments/transitions/notes come from reveal.

// SlideCanvasWidth/Height is the fixed authoring stage; block positions are
// pixels within it. The editor scales this to fit; rendering uses it 1:1.
const (
	SlideCanvasWidth  = 1280
	SlideCanvasHeight = 720
)

// SlideBlock is one positioned reveal.js content element. Kind is a reveal
// element type (text, image, video, embed, code, math, table, list, quote,
// mermaid), not a form field type. Fragment is a reveal fragment animation
// ("" = none) that steps the element in; Lang is the language of a code block.
// Z-order is the block's index in SlideDoc.Blocks.
type SlideBlock struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Content  any               `json:"content"`
	X        int               `json:"x"`
	Y        int               `json:"y"`
	W        int               `json:"w"`
	H        int               `json:"h"`
	Fragment string            `json:"fragment,omitempty"`
	Lang     string            `json:"lang,omitempty"`
	Style    map[string]string `json:"style,omitempty"` // per-element CSS (font-size, color, text-align, …)
}

// styleKeyOrder is the deterministic order CSS declarations are emitted in, so a
// block's inline style is stable across renders. Only these known properties are
// emitted (a fixed allowlist, not arbitrary CSS).
var styleKeyOrder = []string{
	"font-family", "font-size", "font-weight", "font-style", "color", "text-align",
	"line-height", "letter-spacing", "background", "padding",
}

// InlineStyle renders a block's Style map as a CSS declaration string in a
// stable order, keeping only allowlisted properties.
func (b SlideBlock) InlineStyle() string {
	if len(b.Style) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, k := range styleKeyOrder {
		if v := strings.TrimSpace(b.Style[k]); v != "" {
			sb.WriteString(k)
			sb.WriteString(":")
			sb.WriteString(v)
			sb.WriteString(";")
		}
	}
	return sb.String()
}

// SlideDoc is the stored value of a slide field: the per-slide reveal content.
// Blocks are the elements; Background/Transition/Notes are reveal's per-slide
// attributes (background color/image, transition override, speaker notes).
type SlideDoc struct {
	Blocks     []SlideBlock `json:"blocks"`
	Background string       `json:"background,omitempty"`
	Transition string       `json:"transition,omitempty"`
	Notes      string       `json:"notes,omitempty"`
}

// SlideCanvasSize reads the deck's authored canvas size from the slide field's
// options (deck-wide config, shared by every slide), defaulting to the fixed
// 1280x720. These dimensions drive the editor stage, the rendered slide, and
// (later) reveal's width/height at init.
func SlideCanvasSize(f Field) (w, h int) {
	// Any option whose label carries two integers is a format ("1920 x 1080
	// (16:9)"), wherever it sits - a clean canvas_format row, or (from an earlier
	// migration glitch) the legacy canvas_width row. Fall back to the separate
	// canvas_width/canvas_height rows for genuinely old templates.
	for _, opt := range f.Options {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		if cw, ch, ok := parseCanvasFormat(fmt.Sprint(m["label"])); ok {
			return cw, ch
		}
	}
	return optionInt(f, "canvas_width", SlideCanvasWidth), optionInt(f, "canvas_height", SlideCanvasHeight)
}

// SlideFormats is the allowed set of canvas formats (aspect ratio + resolution),
// the single source of truth for the field editor's Format dropdown and the
// dimensions parsed from a chosen preset. Backend-owned, like ListItemTypes.
func SlideFormats() []string {
	return []string{
		"1280 x 720 (16:9)",
		"1920 x 1080 (16:9)",
		"1024 x 768 (4:3)",
	}
}

// parseCanvasFormat pulls the first two integers out of a format label, so
// "1280 x 720 (16:9)" -> 1280, 720 (the ratio digits after are ignored).
func parseCanvasFormat(label string) (w, h int, ok bool) {
	nums := strings.FieldsFunc(label, func(r rune) bool { return r < '0' || r > '9' })
	if len(nums) < 2 {
		return 0, 0, false
	}
	w, _ = strconv.Atoi(nums[0])
	h, _ = strconv.Atoi(nums[1])
	if w > 0 && h > 0 {
		return w, h, true
	}
	return 0, 0, false
}


func optionInt(f Field, key string, def int) int {
	for _, opt := range f.Options {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		if v, _ := m["value"].(string); v != key {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(m["label"]))); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// SlideAccent is the deck-wide accent colour (progress bar fill + nav arrows),
// or "" to leave reveal's defaults. Deck-wide config on the slide field.
func SlideAccent(f Field) string {
	for _, opt := range f.Options {
		if m, ok := opt.(map[string]any); ok {
			if v, _ := m["value"].(string); v == "accent_color" {
				return strings.TrimSpace(fmt.Sprint(m["label"]))
			}
		}
	}
	return ""
}

// SlideProgressHeight is the reveal progress bar thickness in px (default 3).
func SlideProgressHeight(f Field) int {
	return optionInt(f, "progress_height", 3)
}

// SlideBlockKindDescriptor names one reveal element the block palette offers.
// Name is the reveal element kind; LabelKey is its i18n label.
type SlideBlockKindDescriptor struct {
	Name     string `json:"name"`
	LabelKey string `json:"label_key"`
}

// builtinSlideBlockKinds is the reveal.js element palette; display order is
// significant. These are reveal content types, not form field types - the
// editor and renderer map each to the right editor/markup.
var builtinSlideBlockKinds = []SlideBlockKindDescriptor{
	{Name: "text", LabelKey: "workspace.templates.slide.kind.text"},
	{Name: "image", LabelKey: "workspace.templates.slide.kind.image"},
	{Name: "video", LabelKey: "workspace.templates.slide.kind.video"},
	{Name: "embed", LabelKey: "workspace.templates.slide.kind.embed"},
	{Name: "code", LabelKey: "workspace.templates.slide.kind.code"},
	{Name: "math", LabelKey: "workspace.templates.slide.kind.math"},
	{Name: "table", LabelKey: "workspace.templates.slide.kind.table"},
	{Name: "list", LabelKey: "workspace.templates.slide.kind.list"},
	{Name: "quote", LabelKey: "workspace.templates.slide.kind.quote"},
	{Name: "mermaid", LabelKey: "workspace.templates.slide.kind.mermaid"},
	{Name: "shape", LabelKey: "workspace.templates.slide.kind.shape"},
}

// SlideFontDescriptor names one font choice for a slide text block. Value is the
// CSS font-family stack stored in the block's style. A generic family carries an
// i18n LabelKey (translatable); a named font carries a literal Label (a proper
// noun). Every stack ends in a generic family, so a named font degrades to it
// where the actual face is absent (e.g. the headless-Chrome PDF baker), never to
// a missing-glyph box.
type SlideFontDescriptor struct {
	Value    string `json:"value"`
	Label    string `json:"label,omitempty"`
	LabelKey string `json:"label_key,omitempty"`
}

var builtinSlideFonts = []SlideFontDescriptor{
	// Generic families (translatable) first, then the standard web-safe named
	// fonts grouped sans / serif / monospace / cursive / display.
	{Value: "", LabelKey: "workspace.storage.slide.font.default"},
	{Value: "sans-serif", LabelKey: "workspace.storage.slide.font.sans"},
	{Value: "serif", LabelKey: "workspace.storage.slide.font.serif"},
	{Value: "monospace", LabelKey: "workspace.storage.slide.font.mono"},

	{Value: "Arial, Helvetica, sans-serif", Label: "Arial"},
	{Value: `"Arial Black", Gadget, sans-serif`, Label: "Arial Black"},
	{Value: `"Helvetica Neue", Helvetica, Arial, sans-serif`, Label: "Helvetica Neue"},
	{Value: "Verdana, Geneva, sans-serif", Label: "Verdana"},
	{Value: "Tahoma, Geneva, sans-serif", Label: "Tahoma"},
	{Value: `"Trebuchet MS", Helvetica, sans-serif`, Label: "Trebuchet MS"},
	{Value: `"Gill Sans", "Gill Sans MT", Calibri, sans-serif`, Label: "Gill Sans"},
	{Value: `"Segoe UI", system-ui, sans-serif`, Label: "Segoe UI"},
	{Value: "Geneva, Tahoma, sans-serif", Label: "Geneva"},
	{Value: `"Lucida Sans", "Lucida Grande", sans-serif`, Label: "Lucida Sans"},
	{Value: `"Century Gothic", AppleGothic, sans-serif`, Label: "Century Gothic"},

	{Value: "Georgia, serif", Label: "Georgia"},
	{Value: `"Times New Roman", Times, serif`, Label: "Times New Roman"},
	{Value: "Garamond, serif", Label: "Garamond"},
	{Value: `"Palatino Linotype", "Book Antiqua", Palatino, serif`, Label: "Palatino"},
	{Value: `Baskerville, "Baskerville Old Face", serif`, Label: "Baskerville"},
	{Value: "Cambria, Georgia, serif", Label: "Cambria"},

	{Value: `"Courier New", Courier, monospace`, Label: "Courier New"},
	{Value: `Consolas, "Courier New", monospace`, Label: "Consolas"},
	{Value: `"Lucida Console", Monaco, monospace`, Label: "Lucida Console"},
	{Value: "Monaco, Consolas, monospace", Label: "Monaco"},

	{Value: `"Comic Sans MS", "Comic Sans", cursive`, Label: "Comic Sans MS"},
	{Value: `"Brush Script MT", cursive`, Label: "Brush Script MT"},

	{Value: `Impact, "Arial Black", sans-serif`, Label: "Impact"},
	{Value: `Copperplate, "Copperplate Gothic Light", fantasy`, Label: "Copperplate"},
}

// SlideFonts returns a defensive copy of the slide text font vocabulary.
func SlideFonts() []SlideFontDescriptor {
	out := make([]SlideFontDescriptor, len(builtinSlideFonts))
	copy(out, builtinSlideFonts)
	return out
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
