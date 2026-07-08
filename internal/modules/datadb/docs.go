package datadb

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/api/swaggerui"
)

// docsHTML is the Swagger UI shell for the bundle's data API. It loads the
// bundled swagger-ui assets from /api/docs/ and the spec from /api/openapi.json,
// all same-origin, so it works fully offline inside the Viewer.
const docsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Formidable Bundle API</title>
  <link rel="stylesheet" href="/api/docs/swagger-ui.css" />
  <style>body { margin: 0; background: #fff; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/api/docs/swagger-ui-bundle.js"></script>
  <script src="/api/docs/swagger-ui-standalone-preset.js"></script>
  <script>
    window.addEventListener("DOMContentLoaded", function () {
      window.ui = SwaggerUIBundle({
        url: "/api/openapi.json",
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        plugins: [SwaggerUIBundle.plugins.DownloadUrl],
        layout: "StandaloneLayout"
      });
    });
  </script>
</body>
</html>`

// serveDocs serves the Swagger UI shell and its bundled assets under /api/docs/.
func serveDocs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/docs/")
	if name == "" || name == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, docsHTML)
		return
	}
	data, mime, ok := swaggerui.File(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// Collection describes one collection in the bundle for the spec and context:
// its filename, display name, and the JSON Schema of its record data. Data is
// produced by the export backend from the live API's own schema builder, so the
// field typing (loops, tables, lists, relations, dates, enums) mirrors the real
// Formidable API rather than being re-derived here.
type Collection struct {
	Filename string
	Name     string
	Data     map[string]any // JSON Schema object; its "properties" are the fields
}

// BuildOpenAPI produces the bundle's OpenAPI document from its actual
// collections: the {tpl} path parameter is enumerated to the real filenames,
// each collection's data schema is attached as a component, and the collections
// are listed in the description. Packed by the export backend as _/openapi.json;
// the Viewer serves it in place of the generic spec.
func BuildOpenAPI(cols []Collection) []byte {
	spec := baseOpenAPISpec()

	paths := spec["paths"].(map[string]any)
	tplParam := paths["/api/templates/{tpl}"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)[0].(map[string]any)
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)

	names := make([]any, 0, len(cols))
	lines := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Filename)
		if c.Data != nil {
			schemas[schemaName(c.Filename)] = c.Data
		}
		lines = append(lines, "- "+c.Name+" (`"+c.Filename+"`)")
	}
	if len(names) > 0 {
		tplParam["schema"].(map[string]any)["enum"] = names
	}
	if len(lines) > 0 {
		spec["info"].(map[string]any)["description"] =
			"Read-only access to the collection records packed in this bundle.\n\nCollections:\n" + strings.Join(lines, "\n")
	}

	out, _ := json.Marshal(spec)
	return out
}

// schemaName is the component-schema name for a template's fields, e.g.
// "kostenplaats.yaml" -> "Fields_kostenplaats".
func schemaName(filename string) string {
	return "Fields_" + strings.TrimSuffix(filename, ".yaml")
}

// baseOpenAPISpec describes the read-only data API's endpoints. It is the
// generic spec served when no per-template spec was packed.
func baseOpenAPISpec() map[string]any {
	ref := func(name string) map[string]any {
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	arrayOf := func(name string) map[string]any {
		return map[string]any{"type": "array", "items": ref(name)}
	}
	jsonResponse := func(desc string, schema map[string]any) map[string]any {
		return map[string]any{
			"description": desc,
			"content":     map[string]any{"application/json": map[string]any{"schema": schema}},
		}
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Formidable Bundle API",
			"version":     "1.0.0",
			"description": "Read-only access to the collection records packed in this bundle.",
		},
		"servers":  []any{map[string]any{"url": "/"}},
		"security": []any{map[string]any{"ApiKeyAuth": []any{}}},
		"paths": map[string]any{
			"/api/context": map[string]any{
				"get": map[string]any{
					"summary":     "Agent context primer",
					"description": "Markdown explaining how to read this bundle's data model and API. Public (no key).",
					"security":    []any{}, // overrides the global requirement: open
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Markdown primer",
							"content":     map[string]any{"text/markdown": map[string]any{"schema": map[string]any{"type": "string"}}},
						},
					},
				},
			},
			"/api/templates": map[string]any{
				"get": map[string]any{
					"summary":     "List templates",
					"description": "Every collection template in the bundle with its record count.",
					"responses": map[string]any{
						"200": jsonResponse("Templates in the bundle", arrayOf("TemplateCount")),
					},
				},
			},
			"/api/templates/{tpl}": map[string]any{
				"get": map[string]any{
					"summary": "List a template's records",
					"parameters": []any{map[string]any{
						"name": "tpl", "in": "path", "required": true,
						"description": "Template filename, e.g. kostenplaats.yaml",
						"schema":      map[string]any{"type": "string"},
					}},
					"responses": map[string]any{
						"200": jsonResponse("Records of the template", arrayOf("RecordRef")),
					},
				},
			},
			"/api/records/{guid}": map[string]any{
				"get": map[string]any{
					"summary": "Get one record",
					"parameters": []any{map[string]any{
						"name": "guid", "in": "path", "required": true,
						"description": "Record guid (globally unique)",
						"schema":      map[string]any{"type": "string"},
					}},
					"responses": map[string]any{
						"200": jsonResponse("The record with its full payload", ref("RecordFull")),
						"404": map[string]any{"description": "No record with that guid"},
					},
				},
			},
			"/api/search": map[string]any{
				"get": map[string]any{
					"summary": "Full-text search",
					"parameters": []any{map[string]any{
						"name": "q", "in": "query", "required": true,
						"description": "Search terms",
						"schema":      map[string]any{"type": "string"},
					}},
					"responses": map[string]any{
						"200": jsonResponse("Matching records", arrayOf("RecordRef")),
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]any{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
			"schemas": map[string]any{
				"TemplateCount": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"template": map[string]any{"type": "string"},
						"count":    map[string]any{"type": "integer"},
					},
				},
				"RecordRef": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"template": map[string]any{"type": "string"},
						"guid":     map[string]any{"type": "string"},
						"title":    map[string]any{"type": "string"},
					},
				},
				"RecordFull": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"template": map[string]any{"type": "string"},
						"guid":     map[string]any{"type": "string"},
						"title":    map[string]any{"type": "string"},
						"payload": map[string]any{
							"type":        "object",
							"description": "The record's field values, facets, tags, and relations.",
						},
					},
				},
			},
		},
	}
}
