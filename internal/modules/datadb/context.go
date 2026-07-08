package datadb

import (
	"sort"
	"strings"
)

// contextPreamble explains, to an AI agent or a first-time caller, what a
// Formidable bundle is and how its data model and API fit together. It is the
// static part of the context primer; BuildContext appends the bundle's own
// collections after it.
const contextPreamble = `# Formidable data bundle

This is a read-only Formidable information bundle: a self-contained dataset an
author exported from Formidable and handed to you, served over a small REST API
for programmatic and AI-agent use. Nothing here can be changed through the API.

## Data model

- A **template** is a collection of records (one kind of thing), identified by
  its filename, e.g. ` + "`kostenplaats.yaml`" + `.
- A **record** is one entry in a collection. Every record has a globally unique
  **guid**, a human **title**, and a **payload**.
- A record's **payload** contains:
  - ` + "`fields`" + `: the record's own authored field values.
  - ` + "`facets`" + `: categorical tags (each facet carries one selected value),
    used to filter and group records.
  - ` + "`tags`" + `: free-text labels.
  - ` + "`relations`" + `: links to other records, keyed by the target template
    filename, each a list of target guids. Follow a link by fetching
    ` + "`/api/records/{guid}`" + `.
- Field values may contain ` + "`formidable://<template>:<datafile>`" + ` links;
  those are internal cross-references between records.

## How to query

Base path ` + "`/api`" + `, GET only:

- ` + "`GET /api/templates`" + ` — the collections in this bundle, with counts.
- ` + "`GET /api/templates/{tpl}`" + ` — a collection's records (guid + title).
- ` + "`GET /api/records/{guid}`" + ` — one record with its full payload.
- ` + "`GET /api/search?q=<terms>`" + ` — full-text search across records.

The machine-readable schema is ` + "`GET /api/openapi.json`" + ` (interactive docs
at ` + "`/api/docs/`" + `). The data endpoints require an API key, sent as an
` + "`X-API-Key`" + ` header, an ` + "`Authorization: Bearer`" + ` token, or a
` + "`?key=`" + ` query parameter. This context, the docs, and the spec are open
so you can orient yourself before authorizing.

## Suggested approach

1. ` + "`GET /api/templates`" + ` to see what the bundle holds.
2. ` + "`GET /api/openapi.json`" + ` for the exact field schema of each collection.
3. Use ` + "`GET /api/search`" + ` to locate records, then ` + "`GET /api/records/{guid}`" + `
   for detail and to follow relations to other records.
`

// BuildContext produces the agent-context primer for this bundle: the static
// preamble plus a listing of the bundle's actual collections and their fields.
// Field names come from each collection's data schema (the same one the live API
// uses), and array-typed fields (loops, tables, lists) are suffixed with [].
// The export backend packs it; the Viewer serves it at /api/context.
func BuildContext(cols []Collection) []byte {
	var b strings.Builder
	b.WriteString(contextPreamble)

	if len(cols) > 0 {
		b.WriteString("\n## Collections in this bundle\n\n")
		for _, c := range cols {
			b.WriteString("- **")
			b.WriteString(c.Name)
			b.WriteString("** (`")
			b.WriteString(c.Filename)
			b.WriteString("`)")
			if keys := schemaFieldKeys(c.Data); len(keys) > 0 {
				b.WriteString(" — fields: ")
				b.WriteString(strings.Join(keys, ", "))
			}
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// schemaFieldKeys lists the property keys of a data schema, sorted, with
// array-typed fields suffixed "[]". See /api/openapi.json for the full types.
func schemaFieldKeys(data map[string]any) []string {
	props, ok := data["properties"].(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k, v := range props {
		if m, ok := v.(map[string]any); ok && m["type"] == "array" {
			k += "[]"
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
