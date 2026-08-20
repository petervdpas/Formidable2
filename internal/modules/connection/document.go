package connection

import (
	"fmt"
	"strings"

	oasyaml "github.com/oasdiff/yaml"
	"github.com/petervdpas/formidable2/internal/modules/api/swaggerui"
)

// The panel renders an uploaded document the way every other API tool does,
// with Swagger UI. Formidable already vendors swagger-ui-dist for its own REST
// surface, so the same copy is reused here: a second one on the frontend would
// drift from it, and the two would eventually render the same document
// differently.

// SpecDocument is an uploaded document prepared for the renderer.
type SpecDocument struct {
	File string `json:"file"`

	// JSON is the document as JSON, converted when the upload was YAML and
	// escaped so it cannot close the script tag that carries it. Swagger UI
	// takes an object, and handing it the spec directly avoids any fetch,
	// which matters inside a webview with no server behind it.
	JSON string `json:"json"`

	// Format is the version marker the document carries. A Swagger 2.0
	// document stays 2.0: the renderer handles it natively, and converting
	// would show the author a document they never uploaded.
	Format string `json:"format"`
}

// SpecDocument reads a stored document and returns it as JSON.
func (m *Manager) SpecDocument(specFile string) (SpecDocument, error) {
	if err := checkSpecFile(specFile); err != nil {
		return SpecDocument{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return SpecDocument{}, fmt.Errorf("connection: no filesystem")
	}
	raw, err := m.fs.LoadFile(specPath(specFile))
	if err != nil {
		return SpecDocument{}, fmt.Errorf("connection: cannot read spec %q: %w", specFile, err)
	}

	jsonData, err := oasyaml.YAMLToJSON([]byte(raw))
	if err != nil {
		return SpecDocument{}, fmt.Errorf("connection: spec %q is neither valid JSON nor YAML: %w", specFile, err)
	}
	format, err := detectFormat(jsonData)
	if err != nil {
		return SpecDocument{}, err
	}

	// A JSON upload goes through untouched, so the rendered document is the
	// one on disk rather than a re-serialised copy of it.
	body := string(jsonData)
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		body = raw
	}
	return SpecDocument{File: specFile, JSON: scriptSafeJSON(body), Format: format}, nil
}

// scriptSafeJSON escapes the characters that let a document embedded in a
// <script> block escape it. Outside a string, JSON cannot contain any of them,
// and inside one the \u form parses back to the same character, so this is
// lossless rather than a filter that could be worked around.
//
// U+2028 and U+2029 go too: JSON allows them raw, JavaScript source does not.
func scriptSafeJSON(doc string) string {
	return strings.NewReplacer(
		"<", `\u003c`,
		">", `\u003e`,
		"&", `\u0026`,
		"\u2028", `\u2028`,
		"\u2029", `\u2029`,
	).Replace(doc)
}

// SwaggerAssetBundle is the vendored renderer, for a frontend with no server to
// fetch it from.
type SwaggerAssetBundle struct {
	CSS    string `json:"css"`
	Bundle string `json:"bundle"`
	Preset string `json:"preset"`
}

// SwaggerAssets returns the bundled swagger-ui files. They are static and
// version-pinned, so a caller is expected to ask once and keep them.
func SwaggerAssets() (SwaggerAssetBundle, error) {
	var out SwaggerAssetBundle
	for name, dst := range map[string]*string{
		"swagger-ui.css":                  &out.CSS,
		"swagger-ui-bundle.js":            &out.Bundle,
		"swagger-ui-standalone-preset.js": &out.Preset,
	} {
		data, _, ok := swaggerui.File(name)
		if !ok {
			return SwaggerAssetBundle{}, fmt.Errorf("connection: bundled asset %q is missing", name)
		}
		*dst = string(data)
	}
	return out, nil
}
