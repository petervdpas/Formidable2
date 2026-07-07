package standalone

import (
	_ "embed"
	"encoding/base64"
	"io/fs"
	"regexp"
	"sync"

	"github.com/petervdpas/formidable2/internal/modules/render"
)

// standaloneInitScript hydrates mermaid + KaTeX in an exported document (no
// reveal); the deck export reuses render.DeckInitJS instead. See its header.
//
//go:embed assets/standalone-init.js
var standaloneInitScript []byte

// standaloneStageCSS lays out the reveal stage full-bleed for the deck export,
// replacing the wiki's deck-page.css chrome (which assumed a fixed topbar). No
// topbar here: nothing to navigate back to in a server-less file.
const standaloneStageCSS = `html,body.deck-page{height:100%;margin:0}
body.deck-page{background:#1b1b1f;overflow:hidden;font-family:system-ui,sans-serif}
.deck-stage{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:#1b1b1f;overflow:hidden}
.deck-stage .reveal{width:100%;height:100%}`

var (
	katexCSSOnce    sync.Once
	katexCSSInlined string

	// reWoffTTF drops the woff/ttf @font-face sources so only the inlined woff2
	// remains (every target browser supports woff2; keeping broken relative refs
	// would 404 in a file:// page).
	reWoffTTF = regexp.MustCompile(`,url\(fonts/[^)]+\.(?:woff|ttf)\) format\("(?:woff|truetype)"\)`)
	// reWoff2 captures a relative woff2 font URL for data-URI inlining.
	reWoff2 = regexp.MustCompile(`url\(fonts/([^)]+\.woff2)\)`)
)

// katexInlineCSS returns render's katex.min.css with its woff2 fonts inlined as
// data URIs and the woff/ttf sources stripped, so KaTeX math renders in a
// server-less page. Computed once (the dist is embedded and immutable).
func katexInlineCSS() string {
	katexCSSOnce.Do(func() {
		raw, err := fs.ReadFile(render.KatexFS(), "katex.min.css")
		if err != nil {
			return
		}
		css := reWoffTTF.ReplaceAllString(string(raw), "")
		css = reWoff2.ReplaceAllStringFunc(css, func(match string) string {
			name := reWoff2.FindStringSubmatch(match)[1]
			data, err := fs.ReadFile(render.KatexFS(), "fonts/"+name)
			if err != nil {
				return match
			}
			return "url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(data) + ")"
		})
		katexCSSInlined = css
	})
	return katexCSSInlined
}

// katexJS returns render's vendored katex.min.js bytes from the embedded dist.
func katexJS() []byte {
	data, err := fs.ReadFile(render.KatexFS(), "katex.min.js")
	if err != nil {
		return nil
	}
	return data
}
