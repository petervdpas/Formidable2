// Package nav owns formidable:// URL routing - parsing, validation,
// and translating "follow this link" into a config-state change the
// rest of the app reacts to.
//
// One source of truth so all three consumers of formidable:// links
// agree on shape and resolution:
//   - the FormFieldLink Vue component (bare-link click in the editor)
//   - the render module's HTML output (clicks inside the rendered preview)
//   - the future internal HTTP server (server-side rewrite to /template/.../form/...)
//
// Mirrors `utils/linkBehavior.js` + `modules/handlers/linkHandler.js`
// from the original Formidable, with the parser regex + validation
// brought together so the HTTP server doesn't have to re-implement it.
package nav

// Target is the parsed shape of a `formidable://<template>:<datafile>#<fragment>`
// URL. Fragment is optional and not yet acted on (mirrors original);
// kept on the wire so future scroll-to-anchor work doesn't need a
// migration.
type Target struct {
	Template string `json:"template"`
	Datafile string `json:"datafile"`
	Fragment string `json:"fragment,omitempty"`
}

// Result is what NavigateToFormidable returns to the frontend. Success
// false carries Error so Vue can toast the reason without redoing the
// validation. Target is filled even on failure when the URL parsed but
// the (template, datafile) pair didn't resolve - useful for diagnostics.
type Result struct {
	Success bool    `json:"success"`
	Target  *Target `json:"target,omitempty"`
	Error   string  `json:"error,omitempty"`
}

// Wails event name. Backend emits when a navigation target has been
// validated + config has been updated; the frontend's global listener
// switches the active workspace. Payload is *Target.
const EventChanged = "nav:changed"
