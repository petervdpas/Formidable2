package pdf

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"

	"github.com/petervdpas/formidable2/internal/modules/render"
)

const mermaidBakeTimeout = 30 * time.Second

// mermaidFenceRe matches a ```mermaid fenced block at line start, capturing
// the diagram source.
var mermaidFenceRe = regexp.MustCompile("(?ms)^```mermaid[ \\t]*\\r?\\n(.*?)\\r?\\n```[ \\t]*$")

// mermaidResult is one diagram's bake outcome: Svg on success, Err on a parse
// failure (Svg empty).
type mermaidResult struct {
	Svg string
	Err string
}

// bakeMermaidSVG replaces ```mermaid fences in the markdown body with an inline
// SVG. A valid diagram becomes mermaid's own (vector) SVG; a broken one becomes
// a self-made error SVG carrying the message. Both go in as a markdown image
// (picoloom sanitizes raw HTML, so an image node is the only way through, and
// SVG stays crisp). Best-effort: whole-bake failure leaves all fences and is
// logged. No fences -> no browser launch.
func (m *Manager) bakeMermaidSVG(body, browserBin string) string {
	locs := mermaidFenceRe.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		return body
	}
	sources := make([]string, len(locs))
	for i, loc := range locs {
		sources[i] = body[loc[2]:loc[3]]
	}

	results, err := renderMermaid(sources, browserBin)
	if err != nil {
		m.log.Warn("pdf: mermaid bake skipped; leaving fences", "err", err, "blocks", len(sources))
		return body
	}

	var b strings.Builder
	last, baked, failed := 0, 0, 0
	for i, loc := range locs {
		b.WriteString(body[last:loc[0]])
		switch {
		case i < len(results) && strings.HasPrefix(strings.TrimSpace(results[i].Svg), "<svg"):
			writeSVGImage(&b, results[i].Svg)
			baked++
		case i < len(results) && results[i].Err != "":
			writeSVGImage(&b, mermaidErrorSVG(results[i].Err))
			failed++
		default:
			b.WriteString(body[loc[0]:loc[1]]) // keep the fence
		}
		last = loc[1]
	}
	b.WriteString(body[last:])
	m.log.Debug("pdf: mermaid baked", "blocks", len(sources), "baked", baked, "failed", failed)
	return b.String()
}

// writeSVGImage emits an SVG as a base64 data-URI markdown image, padded with
// blank lines so picoloom's markdown parser treats it as a block.
func writeSVGImage(b *strings.Builder, svg string) {
	b.WriteString("\n\n![diagram](data:image/svg+xml;base64,")
	b.WriteString(base64.StdEncoding.EncodeToString([]byte(svg)))
	b.WriteString(")\n\n")
}

// renderMermaid renders each source to its (vector) SVG via mermaid.render in
// one headless page. Index-aligned with sources; a source that fails to parse
// carries its Err (Svg empty).
func renderMermaid(sources []string, browserBin string) (results []mermaidResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			results, err = nil, fmt.Errorf("mermaid render: %v", r)
		}
	}()

	l := launcher.New().Headless(true)
	if browserBin != "" {
		l = l.Bin(browserBin)
	}
	controlURL, lerr := l.Launch()
	if lerr != nil {
		return nil, fmt.Errorf("launch browser: %w", lerr)
	}
	defer l.Cleanup()

	browser := rod.New().ControlURL(controlURL)
	if cerr := browser.Connect(); cerr != nil {
		return nil, fmt.Errorf("connect browser: %w", cerr)
	}
	defer browser.MustClose()

	page := browser.MustPage("about:blank").Timeout(mermaidBakeTimeout)
	page.MustWaitLoad()
	if serr := page.AddScriptTag("", string(render.MermaidJS())); serr != nil {
		return nil, fmt.Errorf("inject mermaid.js: %w", serr)
	}

	out := page.MustEval(mermaidRenderJS, sources)
	results = make([]mermaidResult, len(sources))
	for i, item := range out.Arr() {
		if i < len(results) {
			results[i] = mermaidResult{Svg: item.Get("svg").Str(), Err: item.Get("err").Str()}
		}
	}
	return results, nil
}

// mermaidRenderJS renders each source to {svg} or {err} (parse failure). The
// npm build exposes the API as __esbuild_esm_mermaid_nm.mermaid.
const mermaidRenderJS = `
async (sources) => {
  const ns = window.__esbuild_esm_mermaid_nm;
  const m = (window.mermaid && window.mermaid.render) ? window.mermaid
          : (ns && ns.mermaid ? ns.mermaid : null);
  if (!m || !m.render) throw new Error("mermaid global not found after inject");
  m.initialize({ startOnLoad: false, securityLevel: "strict", theme: "default" });
  const out = [];
  for (let i = 0; i < sources.length; i++) {
    try {
      const r = await m.render("mmbake" + i, sources[i]);
      out.push({ svg: r.svg, err: "" });
    } catch (e) {
      out.push({ svg: "", err: String((e && e.message) || e) });
    }
  }
  return out;
}`

// mermaidErrorSVG builds a self-made error card (vector SVG) carrying the
// message, so a broken diagram reads as a clear error in the PDF rather than
// raw source or a broken image.
func mermaidErrorSVG(msg string) string {
	lines := wrapText(collapseWS(msg), 78)
	if len(lines) > 8 {
		lines = append(lines[:8], "…")
	}
	const (
		padX  = 18
		width = 720
		lineH = 18
	)
	titleY := 34
	bodyTop := titleY + 24
	height := bodyTop + len(lines)*lineH + 16

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		width, height, width, height)
	fmt.Fprintf(&b, `<rect x="1" y="1" width="%d" height="%d" rx="6" fill="#fdecea" stroke="#e9a8a4"/>`,
		width-2, height-2)
	fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="sans-serif" font-size="16" font-weight="bold" fill="#b71c1c">Mermaid diagram error</text>`,
		padX, titleY)
	y := bodyTop + 12
	for _, ln := range lines {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-family="monospace" font-size="12" fill="#7a201d">%s</text>`,
			padX, y, escapeXMLText(ln))
		y += lineH
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// wrapText greedily wraps on spaces at roughly max runes per line.
func wrapText(s string, max int) []string {
	if s == "" {
		return []string{""}
	}
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= max:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func escapeXMLText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
