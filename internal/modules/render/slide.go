package render

import (
	"errors"
	"fmt"
	"html"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// videoEmbedURL maps a YouTube/Vimeo watch or share URL to its embeddable player
// URL, so a video block can host those (a bare <video> can only play direct
// media). Returns "" for anything else (kept as a <video src>).
func videoEmbedURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	switch host {
	case "youtu.be":
		if id := strings.Trim(u.Path, "/"); id != "" {
			return "https://www.youtube-nocookie.com/embed/" + id
		}
	case "youtube.com", "m.youtube.com", "youtube-nocookie.com":
		if u.Path == "/watch" {
			if id := u.Query().Get("v"); id != "" {
				return "https://www.youtube-nocookie.com/embed/" + id
			}
		}
		if strings.HasPrefix(u.Path, "/embed/") {
			return raw
		}
	case "vimeo.com":
		if id := strings.Trim(u.Path, "/"); id != "" && !strings.Contains(id, "/") {
			return "https://player.vimeo.com/video/" + id
		}
	case "player.vimeo.com":
		return raw
	}
	return ""
}

// RevealDeck is what the reveal.js viewer needs: the deck body (one <section>
// per slide) and the authored canvas size so the frontend sizes reveal to the
// same aspect ratio the editor used.
type RevealDeck struct {
	HTML   string `json:"html"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Deck-wide chrome from the slide field: accent colour (progress bar + nav
	// arrows) and progress bar thickness in px. Accent "" means reveal defaults.
	Accent   string `json:"accent"`
	Progress int    `json:"progress"`
	// FontFaceCSS holds @font-face rules for user-uploaded fonts (each font inlined
	// as a data: URI). The viewer injects it into a <style> so those fonts render.
	FontFaceCSS string `json:"fontFaceCss"`
}

// BuildDeck renders an ordered set of records into reveal.js slide sections
// (see buildDeck), memoized by (template, datafiles) and keyed on the collection
// revision when SetRevFunc is wired: a cache hit reuses the HTML, any write bumps
// the rev and rebuilds. Without a rev source it always builds fresh.
func (m *Manager) BuildDeck(templateName string, datafiles []string) (RevealDeck, error) {
	deck, err := m.resolveDeck(templateName, datafiles)
	if err != nil {
		return deck, err
	}
	// Font-face CSS is added outside the deck cache so an uploaded/removed font
	// takes effect on the next build without waiting for a collection write.
	if m.fontFace != nil {
		if css, ferr := m.fontFace(); ferr == nil {
			deck.FontFaceCSS = css
		}
	}
	return deck, nil
}

// resolveDeck is the cached deck build (see BuildDeck).
func (m *Manager) resolveDeck(templateName string, datafiles []string) (RevealDeck, error) {
	if m.revFn == nil {
		return m.buildDeck(templateName, datafiles)
	}
	rev, err := m.revFn()
	if err != nil {
		// Can't establish the invalidation key: build fresh, don't cache.
		return m.buildDeck(templateName, datafiles)
	}
	key := templateName + "\x00" + strings.Join(datafiles, "\x00")

	m.deckMu.Lock()
	if e, ok := m.deckCache[key]; ok && e.rev == rev {
		deck := e.deck
		m.deckMu.Unlock()
		return deck, nil
	}
	m.deckMu.Unlock()

	deck, err := m.buildDeck(templateName, datafiles)
	if err != nil {
		return deck, err
	}
	m.deckMu.Lock()
	if m.deckCache == nil {
		m.deckCache = map[string]cachedDeck{}
	}
	m.deckCache[key] = cachedDeck{rev: rev, deck: deck}
	m.deckMu.Unlock()
	return deck, nil
}

// buildDeck renders an ordered set of records into reveal.js slide sections.
// Each record's slide field becomes one <section> holding the positioned canvas
// plus reveal's per-slide attributes (background/transition) and speaker notes.
// datafiles must already be in deck order (form.DeckOrder / SequenceOrder).
func (m *Manager) buildDeck(templateName string, datafiles []string) (RevealDeck, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return RevealDeck{}, fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	var slide *template.Field
	for i := range tpl.Fields {
		if tpl.Fields[i].Type == "slide" {
			slide = &tpl.Fields[i]
			break
		}
	}
	if slide == nil {
		return RevealDeck{}, errors.New("render: template has no slide field")
	}
	w, h := template.SlideCanvasSize(*slide)
	var sb strings.Builder
	for _, df := range datafiles {
		loaded := m.storage.LoadForm(templateName, df)
		if loaded == nil {
			continue
		}
		opts := m.optionsFor(templateName, df)
		sb.WriteString(m.slideSection(loaded.Data[slide.Key], slide, opts))
	}
	return RevealDeck{
		HTML:     sb.String(),
		Width:    w,
		Height:   h,
		Accent:   template.SlideAccent(*slide),
		Progress: template.SlideProgressHeight(*slide),
	}, nil
}

// slideSection wraps one slide doc as a reveal <section>: the positioned canvas,
// per-slide background/transition attributes, and a speaker-notes aside.
func (m *Manager) slideSection(v any, f *template.Field, opts *Options) string {
	doc, _ := template.ParseSlideDoc(v)
	var attrs strings.Builder
	if doc.Background != "" {
		fmt.Fprintf(&attrs, ` data-background-color="%s"`, html.EscapeString(doc.Background))
	}
	if doc.Transition != "" {
		fmt.Fprintf(&attrs, ` data-transition="%s"`, html.EscapeString(doc.Transition))
	}
	notes := ""
	if strings.TrimSpace(doc.Notes) != "" {
		nh, _ := RenderHTML(doc.Notes)
		notes = `<aside class="notes">` + nh + `</aside>`
	}
	return "<section" + attrs.String() + ">" + renderSlide(v, f, opts) + notes + "</section>"
}

// renderSlide turns a slide document into positioned HTML. Each block is
// rendered by the same emitter its kind uses everywhere else (DRY): markdown
// through goldmark, mermaid to a fenced diagram, image/table/list to their
// markup. Each block's HTML is wrapped in an absolutely-positioned box on the
// canvas (size from the field's deck-wide options); z-order is the block's
// order in the document. Empty doc -> "".
func renderSlide(v any, f *template.Field, opts *Options) string {
	doc, err := template.ParseSlideDoc(v)
	if err != nil || len(doc.Blocks) == 0 {
		return ""
	}
	w, h := template.SlideCanvasWidth, template.SlideCanvasHeight
	if f != nil {
		w, h = template.SlideCanvasSize(*f)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb,
		`<div class="slide-canvas" style="position:relative;width:%dpx;height:%dpx">`,
		w, h)
	for i, b := range doc.Blocks {
		inner, _ := RenderHTML(emitSlideBlock(b.Kind, b.Content, b.Lang, opts))
		cls := "slide-block slide-block-" + b.Kind
		if sc := template.SlideShadowClass(b.Shadow); sc != "" {
			cls += " " + sc
			if dc := template.SlideShadowDirClass(b.ShadowDir); dc != "" {
				cls += " " + dc
			}
		}
		if b.Fragment != "" {
			cls += " fragment " + b.Fragment
		}
		fmt.Fprintf(&sb,
			`<div class="%s" style="position:absolute;left:%dpx;top:%dpx;width:%dpx;height:%dpx;z-index:%d;%s"><div class="slide-fit">%s</div></div>`,
			cls, b.X, b.Y, b.W, b.H, i, b.InlineStyle(), inner)
	}
	sb.WriteString("</div>")
	return sb.String()
}

// RenderSlideBlockHTML renders one block's content to HTML using the same
// per-kind emitter the full slide uses, so the canvas editor previews exactly
// what the deck will render. templateName scopes image URLs.
func (m *Manager) RenderSlideBlockHTML(templateName, kind string, content any) string {
	opts := m.optionsFor(templateName, "")
	html, _ := RenderHTML(emitSlideBlock(kind, content, "", opts))
	return html
}

// emitSlideBlock renders one reveal block's content to markdown/HTML, dispatching
// on the reveal element kind. text renders as markdown; code/math/quote map to
// their markdown/reveal forms; video/embed emit reveal media elements; image/
// table/list/mermaid reuse the shared emitters. lang is the code language.
func emitSlideBlock(kind string, content any, lang string, opts *Options) string {
	switch kind {
	case "mermaid":
		return emitMermaid(content)
	case "image":
		if url := emitImage(content, opts); url != "" {
			return "![](" + url + ")"
		}
		return ""
	case "table":
		return slideTableMarkdown(content)
	case "list":
		return emitList(content)
	case "quote":
		return blockquoteMarkdown(stringify(content))
	case "code":
		return "```" + strings.TrimSpace(lang) + "\n" + stringify(content) + "\n```"
	case "math":
		src := strings.TrimSpace(stringify(content))
		if src == "" {
			return ""
		}
		// A hydratable block (mirrors pre.mermaid): the frontend renders the
		// LaTeX with KaTeX. Raw HTML survives goldmark (WithUnsafe) verbatim.
		return `<pre class="katex-math">` + html.EscapeString(src) + `</pre>`
	case "video":
		src := strings.TrimSpace(stringify(content))
		if src == "" {
			return ""
		}
		// A YouTube/Vimeo page URL can't play in a bare <video> (that needs a
		// direct .mp4/.webm); host those in the platform's iframe player instead.
		if embed := videoEmbedURL(src); embed != "" {
			return `<iframe src="` + html.EscapeString(embed) + `" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>`
		}
		return `<video src="` + html.EscapeString(src) + `" controls playsinline preload="metadata" muted data-autoplay></video>`
	case "embed":
		url := strings.TrimSpace(stringify(content))
		if url == "" {
			return ""
		}
		return `<iframe data-src="` + url + `" allowfullscreen></iframe>`
	case "shape":
		// An imported SVG is stored as a file in the template's images folder and
		// rendered as an <img> (one clean line goldmark passes, and the browser
		// sandboxes an img-loaded SVG). A primitive shape renders inline instead.
		if m, ok := content.(map[string]any); ok {
			if fn, _ := m["svgFile"].(string); strings.TrimSpace(fn) != "" {
				url := emitImage(fn, opts)
				if url == "" {
					return ""
				}
				// A tint recolours a (monochrome) SVG: use it as a CSS mask over a
				// solid fill so its alpha takes the chosen colour. As safe as an
				// <img> (the SVG is a mask image, never executed).
				if tint := sanitizeColor(strings.TrimSpace(stringify(m["tint"])), ""); tint != "" {
					u := html.EscapeString(url)
					return `<div class="slide-shape-tint" style="background-color:` + tint +
						`;-webkit-mask-image:url(` + u + `);mask-image:url(` + u + `)"></div>`
				}
				return "![](" + url + ")"
			}
		}
		return emitShape(content)
	default: // text (markdown) and anything else
		return stringify(content)
	}
}

// shapeProps is a shape block's rendered look, read from its content map.
// The block's box (x/y/w/h) gives position and size; these give the variant
// (rectangle/ellipse/line/arrow) and paint.
type shapeProps struct {
	shape       string
	fill        string
	stroke      string
	strokeWidth string
	direction   string // line/arrow only: horizontal|vertical|diagonal-down|diagonal-up
}

var shapeVariants = map[string]bool{
	"rectangle": true, "ellipse": true, "triangle": true, "line": true, "arrow": true,
}

var lineDirections = map[string]bool{
	"horizontal": true, "vertical": true, "diagonal-down": true, "diagonal-up": true,
}

// lineEndpoints maps a direction to the two endpoints (in the 0..100 viewBox)
// the line/arrow spans. The box then stretches that to its size: a horizontal
// line runs across the box's vertical centre, a vertical line down its centre,
// diagonals corner to corner. An arrow points toward the second endpoint.
func lineEndpoints(dir string) (x1, y1, x2, y2 string) {
	switch dir {
	case "vertical":
		return "50", "0", "50", "100"
	case "diagonal-down":
		return "0", "0", "100", "100"
	case "diagonal-up":
		return "0", "100", "100", "0"
	default: // horizontal
		return "0", "50", "100", "50"
	}
}

// colorToken keeps a user-supplied SVG paint only if it is a simple, safe token
// (hex, rgb()/rgba(), a bare CSS color name, or none/transparent). Anything else
// falls back, so a crafted color string can't break out of the attribute.
var colorToken = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+|rgba?\([0-9.,%\s]+\)|none|transparent)$`)

// nonIDChars strips everything but [A-Za-z0-9] so a color can seed a valid,
// collision-stable marker id (arrows of different colors get different markers).
var nonIDChars = regexp.MustCompile(`[^A-Za-z0-9]`)

func sanitizeColor(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" || !colorToken.MatchString(s) {
		return fallback
	}
	return s
}

func readShapeProps(content any) shapeProps {
	p := shapeProps{shape: "rectangle", fill: "#3b82f6", stroke: "#1e3a8a", strokeWidth: "2", direction: "horizontal"}
	m, ok := content.(map[string]any)
	if !ok {
		return p
	}
	if s, _ := m["shape"].(string); shapeVariants[s] {
		p.shape = s
	}
	if d, _ := m["direction"].(string); lineDirections[d] {
		p.direction = d
	}
	p.fill = sanitizeColor(stringify(m["fill"]), p.fill)
	p.stroke = sanitizeColor(stringify(m["stroke"]), p.stroke)
	if n, err := strconv.ParseFloat(strings.TrimSpace(stringify(m["strokeWidth"])), 64); err == nil && n >= 0 && n <= 100 {
		p.strokeWidth = strconv.FormatFloat(n, 'f', -1, 64)
	}
	return p
}

// emitShape renders a shape block as inline SVG that fills its box. The viewBox
// is a unit square stretched to the box (preserveAspectRatio="none"); strokes
// use vector-effect="non-scaling-stroke" so they stay an even pixel width under
// that non-uniform scale, and overflow:visible keeps edge strokes from clipping.
// A line/arrow spans the box diagonal (the two-corner model). Raw SVG survives
// goldmark (WithUnsafe) like the math block's raw <pre>.
func emitShape(content any) string {
	p := readShapeProps(content)
	const open = `<svg class="slide-shape" viewBox="0 0 100 100" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg" style="width:100%;height:100%;overflow:visible">`
	switch p.shape {
	case "ellipse":
		return open + fmt.Sprintf(`<ellipse cx="50" cy="50" rx="50" ry="50" fill="%s" stroke="%s" stroke-width="%s" vector-effect="non-scaling-stroke"/></svg>`,
			p.fill, p.stroke, p.strokeWidth)
	case "triangle":
		return open + fmt.Sprintf(`<polygon points="50,0 100,100 0,100" fill="%s" stroke="%s" stroke-width="%s" stroke-linejoin="round" vector-effect="non-scaling-stroke"/></svg>`,
			p.fill, p.stroke, p.strokeWidth)
	case "line":
		x1, y1, x2, y2 := lineEndpoints(p.direction)
		return open + fmt.Sprintf(`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" vector-effect="non-scaling-stroke"/></svg>`,
			x1, y1, x2, y2, p.stroke, p.strokeWidth)
	case "arrow":
		x1, y1, x2, y2 := lineEndpoints(p.direction)
		mid := "fmt-arrow-" + nonIDChars.ReplaceAllString(p.stroke, "")
		return open + fmt.Sprintf(`<defs><marker id="%s" markerWidth="10" markerHeight="10" refX="7" refY="3" orient="auto" markerUnits="strokeWidth"><path d="M0,0 L8,3 L0,6 Z" fill="%s"/></marker></defs><line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" marker-end="url(#%s)" vector-effect="non-scaling-stroke"/></svg>`,
			mid, p.stroke, x1, y1, x2, y2, p.stroke, p.strokeWidth, mid)
	default: // rectangle
		return open + fmt.Sprintf(`<rect x="0" y="0" width="100" height="100" fill="%s" stroke="%s" stroke-width="%s" vector-effect="non-scaling-stroke"/></svg>`,
			p.fill, p.stroke, p.strokeWidth)
	}
}

// blockquoteMarkdown prefixes every line with "> " so goldmark builds a real
// blockquote (reveal styles it as a quote).
func blockquoteMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = "> " + ln
	}
	return strings.Join(lines, "\n")
}

// slideTableMarkdown renders a table block's 2D array as a GFM table, treating
// the first row as the header and inserting the delimiter row goldmark needs to
// recognise it as a table (emitTable omits that, so its output renders as a
// paragraph). Empty or non-array content yields nothing.
func slideTableMarkdown(v any) string {
	rows, ok := v.([]any)
	if !ok || len(rows) == 0 {
		return ""
	}
	line := func(r any) string {
		cells, _ := r.([]any)
		strs := make([]string, len(cells))
		for i, c := range cells {
			strs[i] = stringify(c)
		}
		return "| " + strings.Join(strs, " | ") + " |"
	}
	header, _ := rows[0].([]any)
	n := len(header)
	if n == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(line(rows[0]))
	sb.WriteString("\n|")
	for range n {
		sb.WriteString(" --- |")
	}
	for _, r := range rows[1:] {
		sb.WriteString("\n")
		sb.WriteString(line(r))
	}
	return sb.String()
}
