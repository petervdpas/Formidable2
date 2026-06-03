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
			writeSVGImage(&b, mermaidErrorSVG())
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

// bombPath is a single-path bomb glyph (viewBox ~0 0 28.19 28.19 after its
// normalizing translate), used for the PDF mermaid error graphic.
const bombPath = `m 110.96933,138.40551 c -2.33239,-0.73958 -4.24496,-2.13816 -5.82428,-4.25905 -1.73217,-2.32616 -2.24686,-6.61858 -1.14887,-9.5814 1.87363,-5.05583 7.01549,-7.83271 12.39121,-6.6919 l 1.57735,0.33473 0.94327,-0.8282 c 0.5188,-0.45551 1.11738,-0.8282 1.33018,-0.8282 0.2128,0 0.84289,0.41196 1.40021,0.91547 l 1.01332,0.91546 1.29261,-1.31234 c 1.31981,-1.33996 2.12949,-1.63979 2.4155,-0.89446 0.10682,0.27837 -0.31162,0.89578 -1.25341,1.84942 l -1.41376,1.43155 0.84823,0.87515 c 1.08358,1.11797 1.10751,2.05278 0.0777,3.03688 l -0.77049,0.73632 0.39177,1.4488 c 1.0871,4.02021 -0.75614,8.96297 -4.22534,11.33049 -2.56709,1.75189 -6.32484,2.38389 -9.04524,1.52128 z m -2.56646,-9.47152 c 0.17462,-0.17462 0.32011,-0.68064 0.32331,-1.12448 0.0162,-2.25354 2.123,-4.48643 4.53492,-4.80638 1.34843,-0.17888 1.95415,-0.73913 1.65158,-1.52761 -0.25459,-0.66345 -1.05744,-0.74891 -2.81228,-0.29934 -2.8836,0.73875 -5.00454,3.10433 -5.39841,6.0211 -0.11365,0.84166 -0.0601,1.3846 0.16323,1.65365 0.39935,0.48118 1.10159,0.51911 1.53765,0.0831 z m 19.76085,-8.94659 c -0.68419,-0.71414 -0.7219,-0.83951 -0.36106,-1.20035 0.22161,-0.22161 0.57289,-0.33395 0.78063,-0.24966 0.59076,0.23971 1.36537,1.44062 1.20268,1.86458 -0.22932,0.59761 -0.78972,0.4544 -1.62225,-0.41457 z m 0.50142,-3.02511 c -0.34818,-0.90734 0.0939,-1.23608 1.54966,-1.15233 1.28708,0.074 1.39203,0.12994 1.39203,0.74153 0,0.61159 -0.10495,0.6675 -1.39203,0.74153 -1.06607,0.0613 -1.42893,-0.0161 -1.54966,-0.33073 z m -6.78685,-3.20185 c -0.59484,-0.55157 -0.67698,-0.7684 -0.45101,-1.19063 0.36974,-0.69085 0.63987,-0.65164 1.43759,0.20866 1.33469,1.4394 0.45308,2.31689 -0.98658,0.98197 z m 5.76316,0.41527 c -0.23546,-0.38099 0.24812,-1.14592 1.31864,-2.08585 1.07689,-0.94553 1.25879,-0.98972 1.73109,-0.42063 0.28812,0.34716 0.164,0.5885 -0.81644,1.5875 -1.11638,1.13751 -1.89915,1.45961 -2.23329,0.91898 z m -2.6742,-0.90124 c -0.0774,-0.20164 -0.10341,-0.88624 -0.0579,-1.52136 0.0741,-1.03325 0.15244,-1.15474 0.7443,-1.15474 0.55856,0 0.67478,0.13945 0.7471,0.8965 0.13019,1.3629 -0.14735,2.1462 -0.76044,2.1462 -0.29284,0 -0.59574,-0.16497 -0.67311,-0.3666 z`

// mermaidErrorSVG draws the bomb glyph + a single "Syntax error in mermaid"
// line. No message - the editor shows that.
func mermaidErrorSVG() string {
	const width, height = 620, 150
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		width, height, width, height)
	fmt.Fprintf(&b, `<rect x="1" y="1" width="%d" height="%d" rx="10" fill="#fdf2f2" stroke="#f0b7b3"/>`,
		width-2, height-2)
	// Bomb glyph: outer transform positions+scales it; inner transform
	// normalizes the path's own coordinates to the origin.
	fmt.Fprintf(&b, `<g transform="translate(34,26) scale(3.4)" fill="#8b1a1a"><g transform="translate(-103.41885,-110.59837)"><path d="%s"/></g></g>`, bombPath)
	b.WriteString(`<text x="190" y="92" font-family="sans-serif" font-size="30" font-weight="bold" fill="#b71c1c">Syntax error in mermaid</text>`)
	b.WriteString(`</svg>`)
	return b.String()
}
