package api

import (
	"context"
	"fmt"
	"maps"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// buildOpenAPISpec assembles the OpenAPI 3.0.3 document. Built from
// the live template set on every request - Swagger UI consumers see
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
	upsertSchemas := map[string]any{}
	upsertPartialSchemas := map[string]any{}
	// templateFacets carries the per-stem Facet slice so we can emit
	// typed facet.<key> query params on per-template list paths after
	// the generic paths are assembled. Stems without facets aren't
	// added - keeps the spec lean.
	templateFacets := map[string][]template.Facet{}
	for _, t := range tps {
		if !t.EnableCollection || t.GuidField == "" {
			continue
		}
		full, err := tpl.LoadTemplate(t.Filename)
		if err != nil || full == nil {
			// A template can be in the index but missing on disk
			// (race during deletion). Skip - the spec still reflects
			// what's reachable.
			continue
		}
		stem := t.Stem
		if len(full.Facets) > 0 {
			templateFacets[stem] = full.Facets
		}
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
		// Full-replace request body - mirrors POST/PUT shape.
		upsertSchemas["Upsert_"+stem] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"meta": map[string]any{"type": "object", "additionalProperties": true},
				"data": map[string]any{"$ref": "#/components/schemas/Data_" + stem},
			},
			"required": []string{"data"},
		}
		// Partial-merge request body - same Data_<stem> properties but
		// nothing required (PATCH allows omitting any subset).
		dataSchema := dataSchemas["Data_"+stem].(map[string]any)
		partialProps := dataSchema["properties"]
		upsertPartialSchemas["UpsertPartial_"+stem] = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"meta": map[string]any{"type": "object", "additionalProperties": true},
				"data": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
					"properties":           partialProps,
				},
			},
		}
		enabledStems = append(enabledStems, stem)
	}

	schemas := map[string]any{
		"ItemBase":       schemaItemBase(),
		"ItemSummary":    schemaItemSummary(),
		"FormMeta":         schemaFormMeta(),
		"FacetState":       schemaFacetState(),
		"FacetDefinition":  schemaFacetDefinition(),
		"FacetOption":      schemaFacetOption(),
		"FacetsResponse":   schemaFacetsResponse(),
		"AuditEntry":     schemaAuditEntry(),
		"TemplateRow":    schemaTemplateRow(),
		"CountResponse":  schemaCountResponse(),
		"ListResponse":   schemaListResponse(),
		"TemplateField":  schemaTemplateField(),
		"TemplateDesign": schemaTemplateDesign(),
		"GUIDResponse":   schemaGUIDResponse(),
		"FieldPatchBody": schemaFieldPatchBody(),
		"BatchRequest":   schemaBatchRequest(),
		"BatchResponse":  schemaBatchResponse(),
		"BatchResultRow": schemaBatchResultRow(),
		"BatchErrorRow":  schemaBatchErrorRow(),
	}
	maps.Copy(schemas, dataSchemas)
	maps.Copy(schemas, itemSchemas)
	maps.Copy(schemas, upsertSchemas)
	maps.Copy(schemas, upsertPartialSchemas)

	// Build oneOf-refs across the per-template schemas - used by the
	// write paths so a single POST/PUT body schema covers all enabled
	// templates without having to duplicate the path definition.
	upsertRefs := []any{}
	upsertPartialRefs := []any{}
	itemRefs := []any{}
	for _, stem := range enabledStems {
		upsertRefs = append(upsertRefs, map[string]any{"$ref": "#/components/schemas/Upsert_" + stem})
		upsertPartialRefs = append(upsertPartialRefs, map[string]any{"$ref": "#/components/schemas/UpsertPartial_" + stem})
		itemRefs = append(itemRefs, map[string]any{"$ref": "#/components/schemas/Item_" + stem})
	}

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
	keyParam := map[string]any{
		"name":     "key",
		"in":       "path",
		"required": true,
		"schema":   map[string]any{"type": "string"},
		"description": "Field key within the template",
	}

	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Formidable Collections API",
			"version": "2.0.0",
			"description": "CRUD over collection-enabled templates. Per-template " +
				"data schemas are derived from each template's fields, and list / " +
				"count endpoints accept `?facet.<key>=LABEL` query params for " +
				"per-facet AND filtering. Templates that declare facets also expose " +
				"a literal `/collections/<stem>` path with `facet.<key>` query " +
				"parameters typed as enums of their declared option labels.",
		},
		"servers": []any{
			map[string]any{"url": "/api"},
		},
		"components": map[string]any{
			"parameters": map[string]any{
				"TemplateParam": templateParam,
				"IdParam":       idParam,
				"KeyParam":      keyParam,
			},
			"schemas": schemas,
		},
		"paths": withFacetPaths(
			pathsForFullAPI(upsertRefs, upsertPartialRefs, itemRefs),
			templateFacets,
		),
	}, nil
}

// withFacetPaths appends per-template list paths to the spec for any
// template that declares facets. Each path mirrors GET /collections/
// {template} but adds typed facet.<key> query params with the
// template's option labels as an enum - so Swagger UI shows a real
// dropdown per facet instead of a generic string box. Templates with
// no facets are unaffected (no per-template path emitted; the generic
// /collections/{template} entry still serves them).
func withFacetPaths(paths map[string]any, facetsByStem map[string][]template.Facet) map[string]any {
	for stem, facets := range facetsByStem {
		params := []any{
			map[string]any{
				"name":     "limit",
				"in":       "query",
				"required": false,
				"schema":   map[string]any{"type": "integer", "default": 100},
			},
			map[string]any{
				"name":     "offset",
				"in":       "query",
				"required": false,
				"schema":   map[string]any{"type": "integer", "default": 0},
			},
			map[string]any{
				"name":   "q",
				"in":     "query",
				"schema": map[string]any{"type": "string"},
			},
			map[string]any{
				"name":   "tags",
				"in":     "query",
				"schema": map[string]any{"type": "string"},
				"description": "Comma-separated tag list (AND across entries).",
			},
		}
		for _, f := range facets {
			labels := make([]any, len(f.Options))
			for i, o := range f.Options {
				labels[i] = o.Label
			}
			params = append(params, map[string]any{
				"name": "facet." + f.Key,
				"in":   "query",
				"schema": map[string]any{
					"type": "string",
					"enum": labels,
				},
				"description": "Filter by facet `" + f.Key + "`. Records match when meta.facets." + f.Key + ".set is true and selected equals the given value. Multiple facet.* params AND together.",
			})
		}
		paths["/collections/"+stem] = map[string]any{
			"get": map[string]any{
				"summary": "List items in `" + stem + "` (facet-filterable)",
				"description": "Per-template list endpoint. Same behaviour as /collections/{template} but documents the declared facets as typed query parameters with their option labels as enums.",
				"parameters": params,
				"responses": map[string]any{
					"200": map[string]any{
						"description": "OK",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ListResponse"},
							},
						},
					},
					"304": map[string]any{"description": "Not Modified"},
					"400": errResp("unknown_facet"),
					"403": errResp("collection-disabled"),
				},
			},
		}
	}
	return paths
}

// pathsForFullAPI declares every route the package serves - read +
// write. Refs into components/parameters keep the bodies short and
// the templated values stay deduped. The per-template upsertRefs /
// upsertPartialRefs / itemRefs slices are used as `oneOf` lists so
// one path covers every enabled template.
func pathsForFullAPI(upsertRefs, upsertPartialRefs, itemRefs []any) map[string]any {
	paths := pathsForReadAPI()

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
	itemOneOf := func(desc string) map[string]any {
		return map[string]any{
			"description": desc,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"oneOf": itemRefs},
				},
			},
		}
	}
	upsertBody := func() map[string]any {
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"oneOf": upsertRefs},
				},
			},
		}
	}
	upsertPartialBody := func() map[string]any {
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"oneOf": upsertPartialRefs},
				},
			},
		}
	}
	upsertQuery := []any{
		map[string]any{
			"name":     "upsert",
			"in":       "query",
			"required": false,
			"schema":   map[string]any{"type": "boolean", "default": false},
			"description": "Allow create when the item is missing.",
		},
	}

	// /api/guid utility.
	paths["/guid"] = map[string]any{
		"get": map[string]any{
			"summary":   "Mint a fresh UUID v4",
			"responses": map[string]any{"200": jsonOK("GUIDResponse")},
		},
	}

	// /api/images/{template}/{filename} - image bytes (or data URL).
	paths["/images/{template}/{filename}"] = map[string]any{
		"get": map[string]any{
			"summary":     "Fetch image bytes (or data URL)",
			"description": "Returns the raw image bytes by default, or the `data:<mime>;base64,…` URL string when `?format=url` is set. The slideout's `<img src=…>` reaches this route through Wails AssetMiddleware so the markdown stays free of inlined base64.",
			"parameters": []any{
				param("TemplateParam"),
				map[string]any{
					"name":     "filename",
					"in":       "path",
					"required": true,
					"schema":   map[string]any{"type": "string"},
				},
				map[string]any{
					"name":        "format",
					"in":          "query",
					"required":    false,
					"description": "raw (default) returns image bytes with the file's MIME type; url returns the `data:` URL string.",
					"schema": map[string]any{
						"type":    "string",
						"enum":    []any{"raw", "url"},
						"default": "raw",
					},
				},
			},
			"responses": map[string]any{
				"200": map[string]any{
					"description": "Image bytes or data URL string.",
					"content": map[string]any{
						"image/*":                  map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
						"text/plain; charset=utf-8": map[string]any{"schema": map[string]any{"type": "string"}},
					},
				},
				"400": errResp("bad-format"),
				"404": errResp("not-found"),
				"405": errResp("method-not-allowed"),
			},
		},
	}

	// /collections/{template} - extend the existing GET entry with POST.
	if entry, ok := paths["/collections/{template}"].(map[string]any); ok {
		entry["post"] = map[string]any{
			"summary":     "Create item (or upsert with ?upsert=true)",
			"description": "Auto-mints a GUID server-side when the body's data[guidKey] is empty.",
			"parameters":  append([]any{param("TemplateParam")}, upsertQuery...),
			"requestBody": upsertBody(),
			"responses": map[string]any{
				"200": itemOneOf("Replaced (only with ?upsert=true)"),
				"201": itemOneOf("Created"),
				"400": errResp("invalid-json"),
				"403": errResp("collection-disabled"),
				"409": errResp("already-exists"),
			},
		}
	}

	// /collections/{template}/{id} - extend the existing GET/HEAD with
	// PUT/PATCH/DELETE.
	if entry, ok := paths["/collections/{template}/{id}"].(map[string]any); ok {
		entry["put"] = map[string]any{
			"summary":    "Replace item by GUID (or upsert)",
			"parameters": append([]any{param("TemplateParam"), param("IdParam")}, upsertQuery...),
			"requestBody": upsertBody(),
			"responses": map[string]any{
				"200": itemOneOf("Replaced"),
				"201": itemOneOf("Created (only with ?upsert=true and missing)"),
				"400": errResp("invalid-json"),
				"403": errResp("collection-disabled"),
				"404": errResp("not-found"),
				"409": errResp("guid-mismatch"),
			},
		}
		entry["patch"] = map[string]any{
			"summary":    "Merge update (partial) by GUID",
			"parameters": []any{param("TemplateParam"), param("IdParam")},
			"requestBody": upsertPartialBody(),
			"responses": map[string]any{
				"200": itemOneOf("OK"),
				"400": errResp("invalid-json"),
				"403": errResp("collection-disabled"),
				"404": errResp("not-found"),
				"409": errResp("guid-mismatch"),
				"412": errResp("precondition-failed"),
			},
		}
		entry["delete"] = map[string]any{
			"summary":    "Delete item by GUID",
			"parameters": []any{param("TemplateParam"), param("IdParam")},
			"responses": map[string]any{
				"204": map[string]any{"description": "No Content"},
				"403": errResp("collection-disabled"),
				"404": errResp("not-found"),
			},
		}
	}

	// /collections/{template}/{id}/field/{key} - single-field PATCH.
	paths["/collections/{template}/{id}/field/{key}"] = map[string]any{
		"patch": map[string]any{
			"summary":    "Update a single field by key",
			"parameters": []any{param("TemplateParam"), param("IdParam"), param("KeyParam")},
			"requestBody": map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/FieldPatchBody"},
					},
				},
			},
			"responses": map[string]any{
				"200": jsonOK("ItemBase"),
				"400": errResp("unknown-field"),
				"403": errResp("collection-disabled"),
				"404": errResp("not-found"),
				"409": errResp("guid-immutable"),
			},
		},
	}

	// /collections/{template}/batch - bulk apply.
	paths["/collections/{template}/batch"] = map[string]any{
		"post": map[string]any{
			"summary":     "Bulk create / replace / merge",
			"description": "Per-item failures are collected in `errors` rather than aborting the batch.",
			"parameters": []any{
				param("TemplateParam"),
				map[string]any{
					"name":     "mode",
					"in":       "query",
					"required": false,
					"schema": map[string]any{
						"type":    "string",
						"enum":    []string{"create", "replace", "merge"},
						"default": "create",
					},
					"description": "create = fail on existing; replace = full upsert; merge = partial upsert.",
				},
			},
			"requestBody": map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"$ref": "#/components/schemas/BatchRequest"},
					},
				},
			},
			"responses": map[string]any{
				"200": jsonOK("BatchResponse"),
				"400": errResp("items-missing or invalid-mode"),
				"403": errResp("collection-disabled"),
			},
		},
	}

	return paths
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
				"summary":     "List items in a collection (paged)",
				"description": "Accepts optional `facet.<key>=LABEL` query params for per-facet AND filtering. Templates that declare facets also expose a typed literal path `/collections/<stem>` with each facet's options as a query-param enum - use that for Swagger UI dropdowns.",
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
					"400": errResp("unknown_facet"),
					"403": errResp("collection-disabled"),
				},
			},
		},
		"/collections/{template}/count": map[string]any{
			"get": map[string]any{
				"summary":     "Count items in a collection",
				"description": "Accepts the same `facet.<key>=LABEL` query params as the list endpoint; the returned `total` reflects the filtered set.",
				"parameters":  []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonOK("CountResponse"),
					"400": errResp("unknown_facet"),
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
				"summary":     "Template design (fields, options, markdown_template, facets)",
				"description": "Returns the full template metadata: fields, options, markdown_template, and the declared facets (same shape as /facets). Templates without facets omit the `facets` property entirely.",
				"parameters":  []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonOK("TemplateDesign"),
					"403": errResp("collection-disabled"),
					"404": errResp("template-not-found"),
				},
			},
		},
		"/collections/{template}/facets": map[string]any{
			"get": map[string]any{
				"summary":     "Template's filter contract (facets)",
				"description": "Returns the facets a consumer can pass on the list / count endpoints as `?facet.<key>=LABEL`. Each facet carries its stable key, FontAwesome icon, and mutually-exclusive options. Templates without facets return an empty array.",
				"parameters":  []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonOK("FacetsResponse"),
					"403": errResp("collection-disabled"),
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
		"/statistics/{template}": map[string]any{
			"get": map[string]any{
				"summary":     "List the template's named statistical objects",
				"description": "Each object is a DSL distribution or a composite (hop route). Evaluate one at /statistics/<stem>/<name>.",
				"parameters":  []any{param("TemplateParam")},
				"responses": map[string]any{
					"200": jsonInline(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"template": map[string]any{"type": "string"},
							"statistics": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":  map[string]any{"type": "string"},
										"label": map[string]any{"type": "string"},
										"kind":  map[string]any{"type": "string", "enum": []any{"dsl", "composite"}},
										"dsl":   map[string]any{"type": "string"},
										"href":  map[string]any{"type": "string"},
									},
								},
							},
						},
					}),
					"403": errResp("collection-disabled"),
				},
			},
			"post": map[string]any{
				"summary":     "Evaluate an ad-hoc statistical DSL",
				"description": "Body `{ \"dsl\": \"count() by F[...]\" }`. Returns the presentation-free grid (axes, measures, cells[].values, cells[].pct), ready to reshape (e.g. into an R data.frame).",
				"parameters":  []any{param("TemplateParam")},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type":     "object",
								"required": []any{"dsl"},
								"properties": map[string]any{
									"dsl": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"200": jsonInline(statGridSchema()),
					"400": errResp("bad-dsl"),
					"403": errResp("collection-disabled"),
					"422": errResp("bad-dsl"),
				},
			},
		},
		"/statistics/{template}/{name}": map[string]any{
			"get": map[string]any{
				"summary":     "Evaluate a named statistical object",
				"description": "Returns a rank-N grid for a plain object, or a composite grid (parent + per-branch child grids) for a composite.",
				"parameters": []any{
					param("TemplateParam"),
					map[string]any{
						"name":        "name",
						"in":          "path",
						"required":    true,
						"description": "Statistical object name",
						"schema":      map[string]any{"type": "string"},
					},
				},
				"responses": map[string]any{
					"200": jsonInline(statGridSchema()),
					"403": errResp("collection-disabled"),
					"404": errResp("not-found"),
					"422": errResp("evaluate-failed"),
				},
			},
		},
	}
}

// jsonInline wraps an inline schema as a 200 application/json response, for
// endpoints whose shapes don't warrant a named component schema.
func jsonInline(schema map[string]any) map[string]any {
	return map[string]any{
		"description": "OK",
		"content": map[string]any{
			"application/json": map[string]any{"schema": schema},
		},
	}
}

// statGridSchema loosely describes the engine's grid (and composite grid)
// JSON: rank-N axes/measures/sparse cells with server-computed percentages.
// Kept inline and permissive (additionalProperties) so composite grids
// validate against the same response without a second named schema.
func statGridSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"description":          "Rank-N grid (axes, measures, cells[].values, cells[].pct) or composite grid (parent, branches[].child).",
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
// - they're not stored in form data.
func fieldToProperty(f template.Field) (string, map[string]any) {
	schema := map[string]any{}
	switch f.Type {
	case "loopstart", "loopstop":
		return "", nil
	case "guid":
		schema["type"] = "string"
		schema["description"] = "GUID field"
	case "text", "textarea":
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
		// Don't clobber a type-specific description (image/api/etc.) -
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

func schemaFormMeta() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "On-disk meta block. Created is set on first save and locked thereafter; Updated is re-stamped on every save with the active profile's identity.",
		"properties": map[string]any{
			"id":       map[string]any{"type": "string", "description": "GUID; minted on first save when the template declares a guid field"},
			"template": map[string]any{"type": "string"},
			"created":  map[string]any{"$ref": "#/components/schemas/AuditEntry"},
			"updated":  map[string]any{"$ref": "#/components/schemas/AuditEntry"},
			"facets": map[string]any{
				"type":                 "object",
				"description":          "Per-facet state, keyed by template.facets[i].key. Each entry has a required `set` bool and optional `selected` option label.",
				"additionalProperties": map[string]any{"$ref": "#/components/schemas/FacetState"},
			},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}

func schemaFacetState() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Per-record state for one facet. `set` is the required toggle (mirrors legacy `flagged`); `selected` is an optional option label (mirrors legacy `flag_state`).",
		"properties": map[string]any{
			"set":      map[string]any{"type": "boolean"},
			"selected": map[string]any{"type": "string"},
		},
		"required": []any{"set"},
	}
}

func schemaAuditEntry() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Who-and-when audit entry. Used for FormMeta.created and FormMeta.updated.",
		"properties": map[string]any{
			"at":    map[string]any{"type": "string", "format": "date-time", "description": "RFC3339Nano timestamp"},
			"name":  map[string]any{"type": "string"},
			"email": map[string]any{"type": "string"},
		},
		"required": []string{"at", "name", "email"},
	}
}

func schemaItemBase() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"template": map[string]any{"type": "string"},
			"id":       map[string]any{"type": "string", "description": "GUID"},
			"filename": map[string]any{"type": "string"},
			"title":    map[string]any{"type": "string"},
			"meta":     map[string]any{"$ref": "#/components/schemas/FormMeta"},
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

func schemaGUIDResponse() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"guid": map[string]any{"type": "string", "description": "Fresh UUID v4"},
		},
		"required": []string{"guid"},
	}
}

func schemaFieldPatchBody() map[string]any {
	return map[string]any{
		"description": "Either { value: ... } or a raw JSON value.",
		"oneOf": []any{
			map[string]any{
				"type":                 "object",
				"required":             []string{"value"},
				"additionalProperties": false,
				"properties": map[string]any{
					"value": map[string]any{},
				},
			},
			map[string]any{"type": "string"},
			map[string]any{"type": "number"},
			map[string]any{"type": "boolean"},
			map[string]any{"type": "array", "items": map[string]any{}},
			map[string]any{"type": "object"},
		},
	}
}

func schemaBatchRequest() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"items"},
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"data"},
					"properties": map[string]any{
						"meta": map[string]any{"type": "object", "additionalProperties": true},
						"data": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
							"description":          "Must include the template's GUID field.",
						},
					},
				},
			},
		},
	}
}

func schemaBatchResultRow() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":       map[string]any{"type": "string"},
			"filename": map[string]any{"type": "string"},
		},
		"required": []string{"id", "filename"},
	}
}

func schemaBatchErrorRow() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"index":   map[string]any{"type": "integer"},
			"id":      map[string]any{"type": "string"},
			"error":   map[string]any{"type": "string"},
			"message": map[string]any{"type": "string"},
		},
		"required": []string{"index", "error"},
	}
}

func schemaBatchResponse() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"template": map[string]any{"type": "string"},
			"mode": map[string]any{
				"type": "string",
				"enum": []string{"create", "replace", "merge"},
			},
			"totalItems": map[string]any{"type": "integer"},
			"created": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/BatchResultRow"},
			},
			"updated": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/BatchResultRow"},
			},
			"errors": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/BatchErrorRow"},
			},
		},
		"required": []string{"template", "mode", "totalItems", "created", "updated", "errors"},
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
			"facets": map[string]any{
				"type":        "array",
				"description": "Filter contract - the same payload served by /collections/{tpl}/facets. Omitted for templates without facets.",
				"items":       map[string]any{"$ref": "#/components/schemas/FacetDefinition"},
			},
		},
		"required": []string{"name", "filename", "enable_collection", "fields"},
	}
}

func schemaFacetDefinition() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "One facet on a template: a stable key, a FontAwesome icon, and the mutually-exclusive options consumers can pass as ?facet.<key>=LABEL.",
		"properties": map[string]any{
			"key":  map[string]any{"type": "string"},
			"icon": map[string]any{"type": "string"},
			"options": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/FacetOption"},
			},
		},
		"required": []string{"key", "icon", "options"},
	}
}

func schemaFacetOption() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"label": map[string]any{"type": "string"},
			"color": map[string]any{"type": "string"},
		},
		"required": []string{"label", "color"},
	}
}

func schemaFacetsResponse() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Body of GET /collections/{tpl}/facets - the template's filter contract.",
		"properties": map[string]any{
			"template": map[string]any{"type": "string"},
			"facets": map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/FacetDefinition"},
			},
		},
		"required": []string{"template", "facets"},
	}
}
