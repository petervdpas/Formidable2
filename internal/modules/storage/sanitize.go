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
	}

	// Resolve id — prefer explicit options, then raw.meta, then _meta,
	// then raw.data.id. If the template declares a guid field and we
	// still don't have one, generate it.
	id := firstNonEmpty(
		opts.ID,
		stringOrEmpty(rawMeta["id"]),
		stringOrEmpty(injected["id"]),
		stringOrEmpty(rawData["id"]),
	)
	if id == "" {
		for _, f := range fields {
			if f.Type == "guid" {
				id = uuid.NewString()
				break
			}
		}
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
	addTags(rawMeta["tags"])
	if injected != nil {
		addTags(injected["tags"])
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
	created := firstNonEmpty(
		stringOrEmpty(rawMeta["created"]),
		stringOrEmpty(injected["created"]),
		opts.Created,
		now,
	)
	updated := firstNonEmpty(
		opts.Updated,
		stringOrEmpty(injected["updated"]),
		now,
	)

	flagged := false
	if opts.Flagged != nil {
		flagged = *opts.Flagged
	}
	if v, ok := rawMeta["flagged"]; ok {
		if b, ok := v.(bool); ok {
			flagged = b
		}
	} else if injected != nil {
		if b, ok := injected["flagged"].(bool); ok {
			flagged = b
		}
	}

	flagState := opts.FlagState
	if v, ok := rawMeta["flag_state"]; ok {
		if s, ok := v.(string); ok {
			flagState = s
		}
	} else if injected != nil {
		if s, ok := injected["flag_state"].(string); ok {
			flagState = s
		}
	}

	templateName := firstNonEmpty(
		stringOrEmpty(rawMeta["template"]),
		stringOrEmpty(injected["template"]),
		opts.TemplateName,
		"unknown",
	)

	authorName := firstNonEmpty(
		stringOrEmpty(rawMeta["author_name"]),
		stringOrEmpty(injected["author_name"]),
		opts.AuthorName,
		"Unknown",
	)
	authorEmail := firstNonEmpty(
		stringOrEmpty(rawMeta["author_email"]),
		stringOrEmpty(injected["author_email"]),
		opts.AuthorEmail,
		"unknown@example.com",
	)

	return Form{
		Meta: FormMeta{
			ID:          id,
			AuthorName:  authorName,
			AuthorEmail: authorEmail,
			Template:    templateName,
			Created:     created,
			Updated:     updated,
			Flagged:     flagged,
			FlagState:   flagState,
			Tags:        tagList,
		},
		Data: data,
	}
}

// splitEnvelope detects the {meta, data} disk envelope vs. the bare
// payload. A user field literally named "data" or "meta" must NOT
// trick us — we only treat the input as an envelope when both keys
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
