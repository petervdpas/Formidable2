package render

import (
	"embed"
	_ "embed"
	"io/fs"
)

// Deck client assets, vendored and embedded so server-side surfaces (the wiki
// HTTP server) can serve a self-contained reveal.js presentation without a CDN.
// The Vue previewer uses its own npm-bundled copies; the shared seam between the
// two is BuildDeck (section bodies) and deck.css (content styling), not these
// client libs. Mirrors fulldoc.go's ProseCSS/MermaidJS pattern.
//
//go:embed assets/reveal.js
var revealScript []byte

//go:embed assets/reveal.css
var revealStylesheet string

//go:embed assets/deck.css
var deckStylesheet string

//go:embed assets/deck-init.js
var deckInitScript []byte

// katexFS holds the whole vendored katex dist (css + js + fonts). katex.min.css
// references its fonts relatively (fonts/KaTeX_*.woff2), so a consumer must serve
// the css and the fonts dir under one URL prefix.
//
//go:embed assets/katex
var katexEmbed embed.FS

// RevealJS returns the vendored reveal.js (UMD; sets the global `Reveal`).
func RevealJS() []byte { return revealScript }

// RevealCSS returns the vendored reveal.js core stylesheet.
func RevealCSS() string { return revealStylesheet }

// DeckCSS returns the shared deck-content stylesheet (same rules the in-app
// previewer imports), so the wiki deck matches the editor.
func DeckCSS() string { return deckStylesheet }

// DeckInitJS returns the plain-JS reveal bootstrap (init + KaTeX/mermaid hydration).
func DeckInitJS() []byte { return deckInitScript }

// KatexFS returns the vendored katex dist rooted at its own directory, so a
// consumer serving it at `/prefix/` keeps katex.min.css's relative `fonts/` URLs
// resolving (e.g. `/prefix/fonts/KaTeX_Main-Regular.woff2`).
func KatexFS() fs.FS {
	sub, err := fs.Sub(katexEmbed, "assets/katex")
	if err != nil {
		// Unreachable (embed guarantees the dir); panic so a future rename surfaces.
		panic("render: katex fs setup: " + err.Error())
	}
	return sub
}
