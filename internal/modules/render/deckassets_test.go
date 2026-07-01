package render

import (
	"io/fs"
	"strings"
	"testing"
)

func TestDeckAssets_AccessorsNonEmpty(t *testing.T) {
	if len(RevealJS()) == 0 {
		t.Error("RevealJS() is empty")
	}
	if !strings.Contains(RevealCSS(), ".reveal") {
		t.Error("RevealCSS() missing .reveal rules")
	}
	if !strings.Contains(DeckCSS(), ".slide-canvas") {
		t.Error("DeckCSS() missing shared slide-canvas rule")
	}
	if !strings.Contains(string(DeckInitJS()), "Reveal") {
		t.Error("DeckInitJS() missing reveal bootstrap")
	}
}

func TestKatexFS_HasCSSAndFonts(t *testing.T) {
	fsys := KatexFS()
	if _, err := fs.Stat(fsys, "katex.min.css"); err != nil {
		t.Errorf("katex.min.css not in KatexFS: %v", err)
	}
	if _, err := fs.Stat(fsys, "katex.min.js"); err != nil {
		t.Errorf("katex.min.js not in KatexFS: %v", err)
	}
	// katex.min.css requests fonts as `fonts/KaTeX_*.woff2`; that dir must resolve.
	entries, err := fs.ReadDir(fsys, "fonts")
	if err != nil {
		t.Fatalf("fonts dir not in KatexFS: %v", err)
	}
	var woff2 int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".woff2") {
			woff2++
		}
	}
	if woff2 == 0 {
		t.Error("KatexFS fonts dir has no .woff2 files")
	}
}
