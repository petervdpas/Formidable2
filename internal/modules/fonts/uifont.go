package fonts

import "strings"

// The app's own typeface is a profile setting, and the choice set is
// backend-owned like every other list: the picker renders what is returned
// here, and what it stores is an ID this package can resolve back to a CSS
// stack. The frontend never assembles a font list of its own.

// Built-in stacks. The system entry is what tokens.css falls back to, repeated
// here because the resolved stack is written onto the document root at runtime
// and must not depend on a stylesheet the setting is trying to override.
const (
	stackSystem = `"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", sans-serif`
	stackSerif  = `Georgia, Cambria, "Times New Roman", Times, serif`
	stackMono   = `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`
)

// UIFontSystem is the ID of the default stack, and the value a profile carries
// when the user has not chosen: empty, so an untouched profile means "system".
const UIFontSystem = ""

// UIFont is one selectable interface font. ID is what the profile stores; Stack
// is the CSS font-family value to write onto the root. Uploaded marks a font
// from <AppRoot>/fonts/, whose face comes from FontFaceCSS.
type UIFont struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Stack    string `json:"stack"`
	Uploaded bool   `json:"uploaded"`
}

// builtinUIFonts is the closed set every profile can pick from without
// uploading anything. Labels are English names of the typeface class, not UI
// copy: the picker shows them verbatim, since translating "Serif" per locale
// would say less than the word already does.
func builtinUIFonts() []UIFont {
	return []UIFont{
		{ID: UIFontSystem, Label: "System", Stack: stackSystem},
		{ID: "serif", Label: "Serif", Stack: stackSerif},
		{ID: "mono", Label: "Monospace", Stack: stackMono},
	}
}

// UIFonts returns the built-in stacks followed by one entry per uploaded font
// family. An uploaded family keeps the system stack behind it, so a font file
// that fails to load leaves the app readable rather than falling back to
// whatever the platform picks for an unknown name.
func (m *Manager) UIFonts() ([]UIFont, error) {
	out := builtinUIFonts()
	infos, err := m.List()
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, f := range out {
		seen[strings.ToLower(f.ID)] = true
	}
	for _, fi := range infos {
		family := strings.TrimSpace(fi.Family)
		if family == "" || seen[strings.ToLower(family)] {
			continue
		}
		seen[strings.ToLower(family)] = true
		out = append(out, UIFont{
			ID:       family,
			Label:    family,
			Stack:    `"` + family + `", ` + stackSystem,
			Uploaded: true,
		})
	}
	return out, nil
}

// ResolveUIFont turns a stored ID back into the font to draw with. An ID that
// no longer resolves (the font file was deleted, the profile came from another
// machine) falls back to the system stack: an unreadable app is a worse answer
// than a different typeface.
func (m *Manager) ResolveUIFont(id string) UIFont {
	fonts, err := m.UIFonts()
	if err != nil {
		fonts = builtinUIFonts()
	}
	want := strings.TrimSpace(id)
	for _, f := range fonts {
		if strings.EqualFold(f.ID, want) {
			return f
		}
	}
	return builtinUIFonts()[0]
}
