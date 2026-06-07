package storage

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Sanitize normalises a raw form payload against a template's fields. Raw may be the {meta, data}
// envelope or the bare {field..., _meta} shape; missing fields get their per-type default. Tags are
// collected, deduped, lowercased, sorted. Audit-block precedence is in resolveAuditEntry. Mints a GUID
// when the template declares a guid field and nothing carries an id.
func Sanitize(raw map[string]any, fields []template.Field, opts SanitizeOptions) Form {
	rawData, rawMeta := splitEnvelope(raw)
	injected, _ := raw["_meta"].(map[string]any)

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
		// Virtual fields (facet) seed no data slot; skip so old payloads can't smuggle a value into one.
		if template.IsVirtualFieldType(f.Type) {
			continue
		}
		if v, ok := rawData[f.Key]; ok {
			data[f.Key] = v
		} else if f.Default != nil {
			data[f.Key] = f.Default
		} else {
			data[f.Key] = defaultForType(f.Type)
		}
		// Legacy "" for an array-shaped field -> typed empty, so a save heals the shape on disk.
		if s, ok := data[f.Key].(string); ok && s == "" {
			if def, isArr := defaultForType(f.Type).([]any); isArr {
				data[f.Key] = def
			}
		}
		// api stores reference id(s); unwrap the legacy {id|guid, ...} snapshot to its
		// id so old picks heal to the new shape on save.
		if f.Type == "api" {
			data[f.Key] = coerceAPIRef(data[f.Key])
		}
	}

	// The data-block guid field is the identity source; meta.id mirrors it. Resolve field, then preserved
	// opts.ID, then stored meta.id, then injected id; mint one when a guid field exists but nothing carries an id.
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

	// Mirror id back into the guid field so disk carries it in both meta.id and data (no drift for readers).
	if id != "" && guidKey != "" {
		data[guidKey] = id
	}

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
	// With a tags-typed field present it is the single source of truth; don't union stale meta.tags back in,
	// or removed tags resurrect on every save.
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
	facets = seedFacetFieldDefaults(facets, fields)

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

// seedFacetFieldDefaults seeds meta.facets[key] from a facet field's Default only when no entry exists;
// an explicit {set:false} counts as existing so Default doesn't resurrect a cleared picker.
func seedFacetFieldDefaults(facets map[string]FacetState, fields []template.Field) map[string]FacetState {
	for _, f := range fields {
		if f.Type != "facet" || f.FacetKey == "" {
			continue
		}
		def, ok := f.Default.(string)
		if !ok || def == "" {
			continue
		}
		if facets == nil {
			facets = map[string]FacetState{}
		}
		if _, present := facets[f.FacetKey]; present {
			continue
		}
		facets[f.FacetKey] = FacetState{Set: true, Selected: def}
	}
	return facets
}

// syncFormFacets normalizes meta facets against the template on save: an undeclared
// facet key is dropped, and a selection that is no longer a declared option becomes
// the facet field's default (when valid) or empty, leaving Set as-is.
// legacyFlagFacetKey is the facet synthesized from the old flagged/flag_state
// pair; it predates the facets system and may not be declared, so save must not
// drop it (the doctor still backstops a genuinely orphaned one).
const legacyFlagFacetKey = "flag"

func syncFormFacets(facets map[string]FacetState, tpl *template.Template) {
	if facets == nil || tpl == nil {
		return
	}
	for key, st := range facets {
		if key == legacyFlagFacetKey {
			continue
		}
		fc := declaredFacet(tpl, key)
		if fc == nil {
			delete(facets, key)
			continue
		}
		if st.Selected != "" && !facetOptionExists(fc, st.Selected) {
			def := facetFieldDefault(tpl, key)
			if def != "" && facetOptionExists(fc, def) {
				st.Selected = def
			} else {
				st.Selected = ""
			}
			facets[key] = st
		}
	}
}

func declaredFacet(tpl *template.Template, key string) *template.Facet {
	for i := range tpl.Facets {
		if tpl.Facets[i].Key == key {
			return &tpl.Facets[i]
		}
	}
	return nil
}

func facetOptionExists(fc *template.Facet, label string) bool {
	for _, o := range fc.Options {
		if o.Label == label {
			return true
		}
	}
	return false
}

func facetFieldDefault(tpl *template.Template, facetKey string) string {
	for _, f := range tpl.Fields {
		if f.Type == "facet" && f.FacetKey == facetKey {
			if def, ok := f.Default.(string); ok {
				return def
			}
		}
	}
	return ""
}

// resolveFacets returns the per-form facets from opts, then new-shape meta, then legacy flagged/flag_state
// migrated into a single "flag" entry; nil when nothing has state.
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

// resolveAuditEntry resolves one Created/Updated block: opts wins when its At is set, then the nested
// new-shape object, then legacy flat author + timestamp, finally now+Unknown so the block is always well-formed.
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
	// Legacy: rawObj was a flat timestamp string (not an object).
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

// auditEntryFromAny pulls At/Name/Email from a nested map; false when not a map or no recognisable fields.
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

// splitEnvelope detects the {meta, data} envelope vs the bare payload; only treats input as an envelope
// when both keys are non-nil objects, so a user field named "data"/"meta" can't trick it.
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

// skipLoop marks every loop-body key skip=true so the outer scanner skips them; returns the matching loopstop index.
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
		// nil = unpicked. A picked value is a reference id (single) or a list of ids (to-many).
		return nil
	default:
		return ""
	}
}

// coerceAPIRef normalises an api-field value to its reference id(s): a bare id
// string (single cardinality) or a []any of id strings (to-many). It unwraps the
// legacy {id|guid, ...columns} snapshot to just the id so old picks heal on save.
// nil or an unrecognised shape becomes nil (unpicked).
func coerceAPIRef(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return t
	case map[string]any:
		return apiRefID(t)
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			switch ev := e.(type) {
			case string:
				if ev != "" {
					out = append(out, ev)
				}
			case map[string]any:
				if id := apiRefID(ev); id != "" {
					out = append(out, id)
				}
			}
		}
		return out
	default:
		return nil
	}
}

// apiRefID pulls the reference id out of a legacy api snapshot map (id, then guid).
func apiRefID(m map[string]any) string {
	if s, ok := m["id"].(string); ok && s != "" {
		return s
	}
	if s, ok := m["guid"].(string); ok && s != "" {
		return s
	}
	return ""
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
