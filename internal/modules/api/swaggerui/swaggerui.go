// Package swaggerui ships a vendored swagger-ui-dist (Apache-2.0)
// alongside the Formidable-specific shell HTML and back-link script.
// All assets live in this directory so a single //go:embed grabs them
// without crossing the api package boundary.
//
// File layout:
//
//	dist/                   ← upstream swagger-ui-dist 5.30.3 (verbatim)
//	  swagger-ui.css
//	  swagger-ui-bundle.js
//	  swagger-ui-standalone-preset.js
//	  LICENSE               ← Apache-2.0
//	index.html              ← Formidable's docs page shell (ours)
//	swagger-back.js         ← Back-to-Wiki pill (ours; ported from JS)
package swaggerui

import (
	"embed"
	"io/fs"
	"path"
)

//go:embed dist/swagger-ui.css dist/swagger-ui-bundle.js dist/swagger-ui-standalone-preset.js
//go:embed index.html swagger-back.js
var assets embed.FS

// File returns the raw bytes for one of the swagger-ui assets, plus a
// MIME type suitable for HTTP. Returns nil + "" + ok=false when the
// name doesn't map to one of the bundled files — caller should 404.
//
// Accepted names (rooted at /api/docs/):
//
//	""                                       → index.html
//	"index.html"                              → index.html
//	"swagger-ui.css"                          → dist/swagger-ui.css
//	"swagger-ui-bundle.js"                    → dist/swagger-ui-bundle.js
//	"swagger-ui-standalone-preset.js"         → dist/swagger-ui-standalone-preset.js
//	"swagger-back.js"                         → swagger-back.js
func File(name string) (data []byte, mime string, ok bool) {
	rel, mime, ok := resolve(name)
	if !ok {
		return nil, "", false
	}
	b, err := fs.ReadFile(assets, rel)
	if err != nil {
		return nil, "", false
	}
	return b, mime, true
}

// resolve maps the public path component (under /api/docs/) to the
// embedded file path + MIME type. Centralised so File and any future
// listing helper agree on the contract.
func resolve(name string) (rel, mime string, ok bool) {
	switch name {
	case "", "index.html":
		return "index.html", "text/html; charset=utf-8", true
	case "swagger-back.js":
		return "swagger-back.js", "text/javascript; charset=utf-8", true
	case "swagger-ui.css":
		return "dist/swagger-ui.css", "text/css; charset=utf-8", true
	case "swagger-ui-bundle.js":
		return "dist/swagger-ui-bundle.js", "text/javascript; charset=utf-8", true
	case "swagger-ui-standalone-preset.js":
		return "dist/swagger-ui-standalone-preset.js", "text/javascript; charset=utf-8", true
	}
	// Reject anything with a path separator — defensive against future
	// expansions and traversal attempts.
	if name != path.Base(name) {
		return "", "", false
	}
	return "", "", false
}
