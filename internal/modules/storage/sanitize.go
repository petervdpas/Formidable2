package storage

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Sanitize normalises a raw form payload against a template's fields.
// The raw input may be in two shapes:
//
//	envelope: {meta:{...}, data:{...}}
//	bare:     {field1: ..., field2: ..., _meta: {...}}
//
// Either is accepted. Missing fields receive their default (per type).
// Loop fields preserve their array shape (no per-cell type defaults).
// Tags are collected from `tags`-typed fields plus options.Tags +
// raw.meta.tags + raw._meta.tags, deduped, lowercased, sorted.
//
// Created and Updated audit blocks are resolved with precedence:
//
//	opts.{Created,Updated}  (when .At != "")
//	  > raw.meta nested object  (new shape)
//	  > raw.meta flat author_name/email + flat created/updated string
//	    (legacy on-disk shape - both blocks adopt the single legacy
//	    author since we can't reconstruct historical update authorship)
//	  > {At: now, Name: "Unknown", Email: "unknown@example.com"}
//
// Mirrors `schemas/meta.schema.js` behaviour, with an explicit GUID
// generation rather than JS's `null` placeholder when the template
// declares a `guid` field.
func Sanitize(raw map[string]any, fields []template.Field, opts SanitizeOptions) Form {
	rawData, rawMeta := splitEnvelope(raw)
	injected, _ := raw["_meta"].(map[string]any)

	// Walk the fields, filling defaults.
	data := make(map[string]any, len(fields))
	skip := map[string]bool{}

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			loopKey := f.Key
			if existing, ok := rawData[loopKey]; ok {
				data[loopKey] = existing
			} else {
				data[loopKey] = []any{}
			}
			endIdx := skipLoop(fields, i+1, loopKey, skip)
			i = endIdx
			continue
		}
		if skip[f.Key] {
			continue
		}
		if v, ok := rawData[f.Key]; ok {
			data[f.Key] = v
		} else if f.Default != nil {
			data[f.Key] = f.Default
		} else {
			data[f.Key] = defaultForType(f.Type)
		}
		// A legacy form may carry "" for an empty array-shaped field
		// (multioption/list/table). Normalise the unset sentinel to the
		// typed empty so the shape is always valid and a save heals disk.
		if s, ok := data[f.Key].(string); ok && s == "" {
			if def, isArr := defaultForType(f.Type).([]any); isArr {
				data[f.Key] = def
			}
		}
	}

	// The guid field in the data block is the identity source; meta.id
	// mirrors it. Resolve field-first, then fall back to an explicitly
	// preserved id (SaveForm passes prev.Meta.ID so meta backfills an
	// empty field), then the stored meta.id, then any injected id. When
	// the template declares a guid field and nothing carries an id yet,
	// mint one.
	guidKey := ""
	for _, f := range fields {
		if f.Type == "guid" {
			guidKey = f.Key
			break
		}
	}
	id := firstNonEmpty(
		stringOrEmpty(rawData[guidKey]),
		opts.ID,
		stringOrEmpty(rawMeta["id"]),
		stringOrEmpty(injected["id"]),
	)
	if id == "" && guidKey != "" {
		id = uuid.NewString()
	}

	// Mirror the resolved id back into the guid field so disk carries it
	// in BOTH meta.id and data, and downstream readers (CSV export, the
	// API, the integrity doctor) never see drift.
	if id != "" && guidKey != "" {
		data[guidKey] = id
	}

	// Tags: collect from options + raw meta + injected + tags-typed fields.
	tags := map[string]struct{}{}
	addTags := func(in any) {
		switch v := in.(type) {
		case []string:
			for _, t := range v {
				addNormalizedTag(tags, t)
			}
		case []any:
			for _, t := range v {
				if s, ok := t.(string); ok {
					addNormalizedTag(tags, s)
				} else if m, ok := t.(map[string]any); ok {
					if s, ok := m["value"].(string); ok {
						addNormalizedTag(tags, s)
					}
				}
			}
		case string:
			for _, piece := range strings.FieldsFunc(v, func(r rune) bool {
				return r == ',' || r == ';'
			}) {
				addNormalizedTag(tags, piece)
			}
		}
	}
	addTags(opts.Tags)
	hasTagsField := false
	for _, f := range fields {
		if f.Type == "tags" {
			hasTagsField = true
			break
		}
	}
	// When the template owns a tags-typed field, that field is the
	// single source of truth - the stale `meta.tags` / `_meta.tags`
	// carried on the envelope (round-tripped from BuildView) must NOT
	// union back in, or removed tags resurrect on every save.
	if !hasTagsField {
		addTags(rawMeta["tags"])
		if injected != nil {
			addTags(injected["tags"])
		}
	}
	for _, f := range fields {
		if f.Type != "tags" {
			continue
		}
		addTags(data[f.Key])
	}

	tagList := make([]string, 0, len(tags))
	for t := range tags {
		tagList = append(tagList, t)
	}
	sort.Strings(tagList)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	legacyAuthor := AuditEntry{
		Name:  firstNonEmpty(stringOrEmpty(rawMeta["author_name"]), stringOrEmpty(injected["author_name"])),
		Email: firstNonEmpty(stringOrEmpty(rawMeta["author_email"]), stringOrEmpty(injected["author_email"])),
	}
	created := resolveAuditEntry(opts.Created, rawMeta["created"], injected["created"], legacyAuthor, now)
	updated := resolveAuditEntry(opts.Updated, rawMeta["updated"], injected["updated"], legacyAuthor, now)

	facets := resolveFacets(opts.Facets, rawMeta, injected)

	templateName := firstNonEmpty(
		stringOrEmpty(rawMeta["template"]),
		stringOrEmpty(injected["template"]),
		opts.TemplateName,
		"unknown",
	)

	return Form{
		Meta: FormMeta{
			ID:       id,
			Template: templateName,
			Created:  created,
			Updated:  updated,
			Facets:   facets,
			Tags:     tagList,
		},
		Data: data,
	}
}

// resolveFacets returns the per-form facets map using opts when set
// (non-nil), otherwise reading rawMeta["facets"] / injected["facets"]
// in new shape, otherwise migrating legacy `flagged` + `flag_state`
// into a single "flag" entry. Returns nil when nothing has state.
func resolveFacets(optsFacets map[string]FacetState, rawMeta, injected map[string]any) map[string]FacetState {
	if optsFacets != nil {
		return cloneFacets(optsFacets)
	}
	if f, ok := facetsFromAny(rawMeta["facets"]); ok {
		return f
	}
	if injected != nil {
		if f, ok := facetsFromAny(injected["facets"]); ok {
			return f
		}
	}
	legacyFlagged := false
	if v, ok := rawMeta["flagged"]; ok {
		if b, ok := v.(bool); ok {
			legacyFlagged = b
		}
	} else if injected != nil {
		if b, ok := injected["flagged"].(bool); ok {
			legacyFlagged = b
		}
	}
	legacyState := ""
	if v, ok := rawMeta["flag_state"]; ok {
		if s, ok := v.(string); ok {
			legacyState = s
		}
	} else if injected != nil {
		if s, ok := injected["flag_state"].(string); ok {
			legacyState = s
		}
	}
	if legacyFlagged || legacyState != "" {
		return map[string]FacetState{
			"flag": {Set: legacyFlagged, Selected: legacyState},
		}
	}
	return nil
}

func cloneFacets(in map[string]FacetState) map[string]FacetState {
	out := make(map[string]FacetState, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func facetsFromAny(v any) (map[string]FacetState, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, false
	}
	out := make(map[string]FacetState, len(m))
	for key, raw := range m {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		set, _ := entry["set"].(bool)
		selected, _ := entry["selected"].(string)
		out[key] = FacetState{Set: set, Selected: selected}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// resolveAuditEntry applies precedence rules for one of the Created /
// Updated blocks. opts wins outright when its At is non-empty (the
// caller has explicitly chosen it - typically SaveForm preserving prev
// or stamping current profile). Otherwise read the nested new-shape
// object from rawMeta / injected, falling back to legacy flat author
// + the matching flat timestamp string. If everything is missing,
// stamp `now` with the "Unknown" author so the meta block is always
// well-formed.
func resolveAuditEntry(opts AuditEntry, rawObj, injectedObj any, legacy AuditEntry, now string) AuditEntry {
	if opts.At != "" {
		return opts
	}
	if entry, ok := auditEntryFromAny(rawObj); ok {
		return entry
	}
	if entry, ok := auditEntryFromAny(injectedObj); ok {
		return entry
	}
	// Legacy: flat author plus the matching flat created/updated string
	// already lived on rawObj (was a string, not an object). Pick it up.
	at := ""
	if s, ok := rawObj.(string); ok {
		at = s
	} else if s, ok := injectedObj.(string); ok {
		at = s
	}
	if at == "" && legacy.Name == "" && legacy.Email == "" {
		return AuditEntry{At: now, Name: "Unknown", Email: "unknown@example.com"}
	}
	if at == "" {
		at = now
	}
	name := legacy.Name
	email := legacy.Email
	if name == "" {
		name = "Unknown"
	}
	if email == "" {
		email = "unknown@example.com"
	}
	return AuditEntry{At: at, Name: name, Email: email}
}

// auditEntryFromAny pulls At/Name/Email out of a nested map shape. The
// map may come from JSON decoding (map[string]any). Returns false when
// the value isn't a map or has no recognisable fields, so the caller
// can fall through to legacy-shape handling.
func auditEntryFromAny(v any) (AuditEntry, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return AuditEntry{}, false
	}
	at, _ := m["at"].(string)
	name, _ := m["name"].(string)
	email, _ := m["email"].(string)
	if at == "" && name == "" && email == "" {
		return AuditEntry{}, false
	}
	return AuditEntry{At: at, Name: name, Email: email}, true
}

// splitEnvelope detects the {meta, data} disk envelope vs. the bare
// payload. A user field literally named "data" or "meta" must NOT
// trick us - we only treat the input as an envelope when both keys
// are non-nil objects.
func splitEnvelope(raw map[string]any) (data, meta map[string]any) {
	dataAny, dOk := raw["data"]
	metaAny, mOk := raw["meta"]
	dataObj, _ := dataAny.(map[string]any)
	metaObj, _ := metaAny.(map[string]any)
	if dOk && mOk && dataObj != nil && metaObj != nil {
		return dataObj, metaObj
	}
	if metaObj != nil {
		return raw, metaObj
	}
	return raw, map[string]any{}
}

// skipLoop walks `fields` starting at start, marking every key inside
// the loop body as skip[key]=true so the outer scanner doesn't consume
// them as top-level fields. Returns the index of the matching loopstop
// (or last field if unpaired).
func skipLoop(fields []template.Field, start int, loopKey string, skip map[string]bool) int {
	depth := 1
	for i := start; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			depth++
			continue
		}
		if f.Type == "loopstop" {
			depth--
			if depth == 0 && f.Key == loopKey {
				return i
			}
		}
		skip[f.Key] = true
	}
	return len(fields) - 1
}

func defaultForType(t string) any {
	switch t {
	case "boolean":
		return false
	case "number":
		return 0
	case "range":
		return 50
	case "multioption", "list", "table":
		return []any{}
	case "api":
		// Unset api field has no record picked yet. nil distinguishes
		// "no value" from a stamped {guid, ...} map; consumers can
		// treat it as the picker's empty state.
		return nil
	default:
		return ""
	}
}

func firstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

func stringOrEmpty(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func addNormalizedTag(set map[string]struct{}, raw string) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return
	}
	set[t] = struct{}{}
}
