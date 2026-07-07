package standalone

import (
	"fmt"
	"html"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/render"
)

// composeDoc wraps concatenated record bodies in a self-contained document:
// prose CSS always inlined, KaTeX CSS + the client libs added only when the
// body needs them.
func composeDoc(title, body string) string {
	needMath := strings.Contains(body, "katex-math")
	needMermaid := containsMermaid(body)

	var sb strings.Builder
	writeDocHead(&sb, title, func(css *strings.Builder) {
		css.WriteString(render.ProseCSS())
		if needMath {
			css.WriteString("\n")
			css.WriteString(katexInlineCSS())
		}
	})
	sb.WriteString(`<body class="formidable-prose">` + "\n")
	sb.WriteString(body)
	if needMermaid {
		writeInlineScript(&sb, render.MermaidJS())
	}
	if needMath {
		writeInlineScript(&sb, katexJS())
	}
	if needMermaid || needMath {
		writeInlineScript(&sb, standaloneInitScript)
	}
	sb.WriteString("\n</body>\n</html>\n")
	return sb.String()
}

// composeDeck mirrors the wiki deck page (wiki templates/deck.html) with every
// asset inlined and the topbar dropped.
func composeDeck(title string, deck render.RevealDeck) string {
	needMath := strings.Contains(deck.HTML, "katex-math")
	needMermaid := containsMermaid(deck.HTML)

	var sb strings.Builder
	writeDocHead(&sb, title+" - Formidable Slides", func(css *strings.Builder) {
		css.WriteString(standaloneStageCSS)
		css.WriteString("\n")
		css.WriteString(render.RevealCSS())
		css.WriteString("\n")
		css.WriteString(render.ProseCSS())
		css.WriteString("\n")
		css.WriteString(render.DeckCSS())
		if needMath {
			css.WriteString("\n")
			css.WriteString(katexInlineCSS())
		}
		if deck.FontFaceCSS != "" {
			css.WriteString("\n")
			css.WriteString(deck.FontFaceCSS)
		}
	})

	accentClass, accentVar := "", ""
	if deck.Accent != "" {
		accentClass = " deck-accented"
		accentVar = "--deck-accent:" + deck.Accent + ";"
	}
	sb.WriteString(`<body class="deck-page">` + "\n")
	sb.WriteString(`<main class="deck-stage">` + "\n")
	fmt.Fprintf(&sb,
		`<div class="reveal%s" data-width="%d" data-height="%d" style="--deck-progress-h:%dpx;%s">`+"\n",
		accentClass, deck.Width, deck.Height, deck.Progress, accentVar)
	sb.WriteString(`<div class="slides formidable-prose">`)
	sb.WriteString(deck.HTML)
	sb.WriteString("</div>\n</div>\n</main>\n")

	writeInlineScript(&sb, render.RevealJS())
	if needMath {
		writeInlineScript(&sb, katexJS())
	}
	if needMermaid {
		writeInlineScript(&sb, render.MermaidJS())
	}
	writeInlineScript(&sb, render.DeckInitJS())
	sb.WriteString("\n</body>\n</html>\n")
	return sb.String()
}

// writeDocHead writes the DOCTYPE + head with an escaped title and one inlined
// <style> block populated by css.
func writeDocHead(sb *strings.Builder, title string, css func(*strings.Builder)) {
	sb.WriteString("<!DOCTYPE html>\n")
	sb.WriteString(`<html lang="en">` + "\n<head>\n")
	sb.WriteString(`<meta charset="utf-8">` + "\n")
	sb.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	sb.WriteString("<title>")
	sb.WriteString(html.EscapeString(title))
	sb.WriteString("</title>\n<style>\n")
	css(sb)
	sb.WriteString("\n</style>\n</head>\n")
}

// writeInlineScript emits a <script> with the given JS inlined verbatim. The
// sources are our own vendored/embedded assets (trusted), so no escaping.
func writeInlineScript(sb *strings.Builder, js []byte) {
	sb.WriteString("<script>\n")
	sb.Write(js)
	sb.WriteString("\n</script>\n")
}

// containsMermaid reports whether HTML holds a client-mode mermaid block, which
// goldmark emits as <pre class="mermaid">.
func containsMermaid(s string) bool {
	return strings.Contains(s, `class="mermaid"`)
}
