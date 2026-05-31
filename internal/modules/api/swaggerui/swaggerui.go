// Package swaggerui ships a vendored swagger-ui-dist 5.30.3 (Apache-2.0; see dist/LICENSE)
// alongside Formidable's shell index.html and swagger-back.js, all embedded from this directory.
package swaggerui

import (
	"embed"
	"io/fs"
	"path"
)

//go:embed dist/swagger-ui.css dist/swagger-ui-bundle.js dist/swagger-ui-standalone-preset.js
//go:embed index.html swagger-back.js
var assets embed.FS

// File returns the bytes + MIME for a bundled swagger-ui asset; ok=false when the name is unknown (caller 404s).
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

// resolve maps a public path component (under /api/docs/) to the embedded file path + MIME type.
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
	// Reject any path separator (defensive against traversal).
	if name != path.Base(name) {
		return "", "", false
	}
	return "", "", false
}
