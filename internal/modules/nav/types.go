// Package nav owns formidable:// URL routing: parsing, validation, and
// translating "follow this link" into a config-state change.
//
// One source of truth so all consumers of formidable:// links agree on
// shape and resolution: the FormFieldLink Vue component, the render
// module's HTML output, and the future internal HTTP server.
package nav

// Target is the parsed shape of a `formidable://<template>:<datafile>#<fragment>`
// URL. Fragment is optional and not yet acted on; kept on the wire so
// future scroll-to-anchor work doesn't need a migration.
type Target struct {
	Template string `json:"template"`
	Datafile string `json:"datafile"`
	Fragment string `json:"fragment,omitempty"`
}

// Result is what NavigateToFormidable returns to the frontend. On
// failure Error carries the reason; Target is filled even on failure
// when the URL parsed but the pair didn't resolve, for diagnostics.
type Result struct {
	Success bool    `json:"success"`
	Target  *Target `json:"target,omitempty"`
	Error   string  `json:"error,omitempty"`
}

// EventChanged is emitted after a navigation target is validated and
// config updated; the frontend listener switches workspace. Payload is *Target.
const EventChanged = "nav:changed"
