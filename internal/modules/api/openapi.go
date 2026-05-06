package api

import (
	"context"
	"fmt"
	"maps"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// buildOpenAPISpec assembles the OpenAPI 3.0.3 document. Built from
// the live template set on every request — Swagger UI consumers see
// schema changes the moment a template is saved, no regen step.
//
// Read-only slice: paths cover GET endpoints only. POST/PUT/PATCH/
// DELETE/batch are noted in the package's TODO and will join the
// spec when the write endpoints land.
func buildOpenAPISpec(ctx context.Context, dp Provider, tpl Templates) (map[string]any, error) {
	tps, err := dp.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve collection-enabled templates with full field definitions.
	enabledStems := []string{}
	dataSchemas := map[string]any{}
	itemSchemas := map[string]any{}
	for _, t := range tps {
		if !t.EnableCollection || t.GuidField == "" {
			continue
		}
		full, err := tpl.LoadTemplate(t.Filename)
		if err != nil || full == nil {
			// A template can be in the index but missing on disk
			// (race during deletion). Skip — the spec still reflects
			// what's reachable.
			continue
		}
		stem := t.Stem
		dataSchemas["Data_"+stem] = dataSchemaForTemplate(full)
		itemSchemas["Item_"+stem] = map[string]any{
			"allOf": []any{
				map[string]any{"$ref": "#/components/schemas/ItemBase"},
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data": map[string]any{
							"$ref": "#/components/schemas/Data_" + stem,
						},
					},
					"required": []string{"data"},
				},
			},
		}
		enabledStems = append(enabledStems, stem)
	}

	schemas := map[string]any{
		"ItemBase":     schemaItemBase(),
		"ItemSummary":  schemaItemSummary(),
		"TemplateRow":  schemaTemplateRow(),
		"CountResponse": schemaCountResponse(),
		"ListResponse": schemaListResponse(),
		"TemplateField":  schemaTemplateField(),
		"TemplateDesign": schemaTemplateDesign(),
	}
	maps.Copy(schemas, dataSchemas)
	maps.Copy(schemas, itemSchemas)

	templateParam := map[string]any{
		"name":     "template",
		"in":       "path",
		"required": true,
		"schema": map[string]any{
			"type": "string",
			"enum": enabledStems,
		},
		"description": "Template id (stem)",
	}
	idParam := map[string]any{
		"name":     "id",
		"in":       "path",
		"required": true,
		"schema":   map[string]any{"type": "string"},
		"description": "Item GUID",
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Formidable Collections API",
			"version": "2.0.0",
			"description": "Read-only access to collection-enabled templates. " +
				"Per-template data schemas are derived from the template's fields.",
		},
		"servers": []any{
			map[string]any{"url": "/api"},
		},
		"components": map[string]any{
			"parameters": map[string]any{
				"TemplateParam": templateParam,
				"IdParam":       idParam,
			},
			"schemas": schemas,
		},
		"paths": pathsForReadAPI(),
	}, nil
}

// pathsForReadAPI declares every GET/HEAD route the package serves.
// Refs into components/parameters keep the bodies short and the
// templated values stay deduped.
func pathsForReadAPI() map[string]any {
	param := func(name string) map[string]any {
		return map[string]any{"$ref": "#/components/parameters/" + name}
	}
	jsonOK := func(refName string) map[string]any {
		return map[string]any{
			"description": "OK",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/" + refName},
				},
			},
		}
	}
	textOK := func(contentType string) map[string]any {
		return map[string]any{
			"description": "OK",
			"content": map[string]any{
				contentType: map[string]any{
					"schema": map[string]any{"type": "string"},
				},
			},
		}
	}

	return map[string]any{
		"/collections": map[string]any{
			"get": map[string]any{
				"summary": "List collection-enabled templates",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "OK",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "array",
									"items": map[string]any{
										"$ref": "#/components/schemas/TemplateRow",
									},
								},
							},
						},
					},
				},
			},
		},
		"/collections/{template}": map[string]any{
			"get": map[string]any{
				"summary": "List items in a collection (paged)",
				"parameters": []any{
					param("TemplateParam"),
					queryInt("limit", 100),
					queryInt("offset", 0),
					queryString("q"),
					queryString("tags"),
				},
				"responses": map[string]any{
					"200": jsonOK("ListResponse"),
					"304": map[string]any{"description": "Not Modified"},
					"403": errResp("collection-disabled"),
				},
			},
		},
		"/collections/{template}/count": map[string]any{
			"get": map[string]any{
				"summary":    "Count items in a collection",
				"parameters": []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonOK("CountResponse"),
					"403": errResp("collection-disabled"),
				},
			},
		},
		"/collections/{template}/{id}": map[string]any{
			"get": map[string]any{
				"summary":    "Fetch one item by GUID",
				"parameters": []any{param("TemplateParam"), param("IdParam")},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "OK",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ItemBase"},
							},
						},
					},
					"304": map[string]any{"description": "Not Modified"},
					"403": errResp("collection-disabled"),
					"404": errResp("not-found"),
				},
			},
			"head": map[string]any{
				"summary":    "ETag/freshness check for one item",
				"parameters": []any{param("TemplateParam"), param("IdParam")},
				"responses": map[string]any{
					"200": map[string]any{"description": "OK (headers only)"},
					"403": map[string]any{"description": "collection-disabled"},
					"404": map[string]any{"description": "not-found"},
				},
			},
		},
		"/collections/{template}/design": map[string]any{
			"get": map[string]any{
				"summary":    "Template design (fields, options, markdown_template)",
				"parameters": []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonOK("TemplateDesign"),
					"403": errResp("collection-disabled"),
					"404": errResp("template-not-found"),
				},
			},
		},
		"/collections/{template}/export.ndjson": map[string]any{
			"get": map[string]any{
				"summary":    "Export the collection as NDJSON (streaming)",
				"parameters": []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": textOK("application/x-ndjson"),
					"304": map[string]any{"description": "Not Modified"},
					"403": errResp("collection-disabled"),
				},
			},
		},
		"/collections/{template}/export.csv": map[string]any{
			"get": map[string]any{
				"summary":    "Export the collection summary as CSV",
				"parameters": []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": textOK("text/csv"),
					"304": map[string]any{"description": "Not Modified"},
					"403": errResp("collection-disabled"),
				},
			},
		},
	}
}

func queryInt(name string, dflt int) map[string]any {
	return map[string]any{
		"name": name,
		"in":   "query",
		"schema": map[string]any{
			"type":    "integer",
			"default": dflt,
		},
	}
}

func queryString(name string) map[string]any {
	return map[string]any{
		"name":   name,
		"in":     "query",
		"schema": map[string]any{"type": "string"},
	}
}

func errResp(slug string) map[string]any {
	return map[string]any{
		"description": slug,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type":     "object",
					"required": []string{"error"},
					"properties": map[string]any{
						"error": map[string]any{"type": "string", "example": slug},
					},
				},
			},
		},
	}
}

// dataSchemaForTemplate builds an object schema with one property per
// non-container field. GUID fields are listed in `required`.
func dataSchemaForTemplate(t *template.Template) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, f := range t.Fields {
		if f.Key == "" {
			continue
		}
		key, schema := fieldToProperty(f)
		if key == "" || schema == nil {
			continue
		}
		props[key] = schema
		if f.Type == "guid" {
			required = append(required, key)
		}
	}
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// fieldToProperty maps a single template.Field to a JSON Schema entry
// (key + body). Container fields (loopstart/loopstop) return ("", nil)
// — they're not stored in form data.
func fieldToProperty(f template.Field) (string, map[string]any) {
	schema := map[string]any{}
	switch f.Type {
	case "loopstart", "loopstop":
		return "", nil
	case "guid":
		schema["type"] = "string"
		schema["description"] = "GUID field"
	case "text", "textarea", "latex", "code":
		schema["type"] = "string"
	case "number":
		schema["type"] = "number"
	case "boolean":
		schema["type"] = "boolean"
	case "date":
		schema["type"] = "string"
		schema["format"] = "date"
	case "dropdown", "radio":
		values, labels := optionPairs(f.Options)
		schema["type"] = "string"
		schema["enum"] = values
		if len(labels) > 0 {
			schema["x-enum-labels"] = labels
		}
	case "multioption":
		values, _ := optionPairs(f.Options)
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string", "enum": values}
	case "range":
		schema["type"] = "number"
		// options are emitted as kv pairs (min/max/step). Read those
		// out and project to JSON Schema keywords.
		for _, raw := range f.Options {
			if m, ok := raw.(map[string]any); ok {
				v := m["value"]
				label := m["label"]
				switch fmt.Sprint(v) {
				case "min":
					schema["minimum"] = label
				case "max":
					schema["maximum"] = label
				case "step":
					schema["multipleOf"] = label
				}
			}
		}
	case "list":
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string"}
	case "table":
		values, _ := optionPairs(f.Options)
		colProps := map[string]any{}
		for _, c := range values {
			colProps[fmt.Sprint(c)] = map[string]any{"type": "string"}
		}
		schema["type"] = "array"
		schema["description"] = "Array of row objects keyed by column id"
		schema["items"] = map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           colProps,
		}
	case "image":
		schema["type"] = "string"
		schema["format"] = "uri"
		schema["description"] = "Path/URL to image"
	case "link":
		schema["type"] = "string"
		schema["format"] = "uri"
	case "tags":
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string"}
	case "api":
		// Persisted selection: either bare id (string) or {id, ...mapped}.
		mapped := map[string]any{}
		for _, m := range f.Map {
			if m.Key != "" {
				mapped[m.Key] = map[string]any{"type": "string"}
			}
		}
		mapped["id"] = map[string]any{"type": "string"}
		schema["oneOf"] = []any{
			map[string]any{"type": "string", "description": "Selected item id"},
			map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"properties":           mapped,
				"required":             []string{"id"},
			},
		}
		schema["description"] = "API-linked value: either the selected id or {id, ...mapped fields}."
	default:
		schema["type"] = "string"
	}
	if f.Description != "" {
		// Don't clobber a type-specific description (image/api/etc.) —
		// only set when we haven't already.
		if _, has := schema["description"]; !has {
			schema["description"] = f.Description
		}
	}
	return f.Key, schema
}

// optionPairs splits template.Field.Options into parallel (values, labels)
// arrays. Bare scalars become value=label; map entries use both fields.
func optionPairs(opts []any) ([]any, []any) {
	values := make([]any, 0, len(opts))
	labels := make([]any, 0, len(opts))
	hasLabel := false
	for _, o := range opts {
		switch v := o.(type) {
		case map[string]any:
			val := stringify(v["value"])
			label := val
			if l, ok := v["label"]; ok && l != nil {
				label = stringify(l)
				hasLabel = true
			}
			values = append(values, val)
			labels = append(labels, label)
		default:
			s := stringify(o)
			values = append(values, s)
			labels = append(labels, s)
		}
	}
	if !hasLabel {
		labels = nil
	}
	return values, labels
}

// ── shared schema definitions ────────────────────────────────────────

func schemaItemBase() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"template": map[string]any{"type": "string"},
			"id":       map[string]any{"type": "string", "description": "GUID"},
			"filename": map[string]any{"type": "string"},
			"title":    map[string]any{"type": "string"},
			"meta":     map[string]any{"type": "object", "additionalProperties": true},
			"data":     map[string]any{"type": "object", "additionalProperties": true},
			"links": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"self": map[string]any{"type": "string"},
					"html": map[string]any{"type": "string"},
				},
			},
			"rev": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"etag": map[string]any{"type": "string"},
				},
			},
		},
		"required": []string{"template", "id", "filename"},
	}
}

func schemaItemSummary() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":       map[string]any{"type": "string"},
			"filename": map[string]any{"type": "string"},
			"title":    map[string]any{"type": "string"},
			"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"hrefSelf": map[string]any{"type": "string"},
			"hrefHtml": map[string]any{"type": "string"},
		},
		"required": []string{"id", "filename", "title"},
	}
}

func schemaTemplateRow() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":   map[string]any{"type": "string"},
			"name": map[string]any{"type": "string"},
			"href": map[string]any{"type": "string"},
		},
		"required": []string{"id", "name", "href"},
	}
}

func schemaCountResponse() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"template": map[string]any{"type": "string"},
			"total":    map[string]any{"type": "integer"},
		},
		"required": []string{"template", "total"},
	}
}

func schemaListResponse() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collectionEnabled": map[string]any{"type": "boolean"},
			"template":          map[string]any{"type": "string"},
			"total":             map[string]any{"type": "integer"},
			"limit":             map[string]any{"type": "integer"},
			"offset":            map[string]any{"type": "integer"},
			"items": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/ItemSummary"},
			},
		},
		"required": []string{"collectionEnabled", "template", "total", "limit", "offset", "items"},
	}
}

func schemaTemplateField() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]any{
			"key":         map[string]any{"type": "string"},
			"type":        map[string]any{"type": "string"},
			"label":       map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"options": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{"type": "string"},
						"label": map[string]any{"type": "string"},
					},
				},
			},
		},
		"required": []string{"key", "type", "label"},
	}
}

func schemaTemplateDesign() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":               map[string]any{"type": "string"},
			"filename":           map[string]any{"type": "string"},
			"item_field":         map[string]any{"type": "string"},
			"markdown_template":  map[string]any{"type": "string"},
			"sidebar_expression": map[string]any{"type": "string"},
			"enable_collection":  map[string]any{"type": "boolean"},
			"fields": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/TemplateField"},
			},
		},
		"required": []string{"name", "filename", "enable_collection", "fields"},
	}
}
