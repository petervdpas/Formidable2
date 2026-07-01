package api

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// guid answers GET /api/guid with a fresh UUID v4.
func (h *Handler) guid(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"guid": uuid.NewString()})
}

// upsertBody is the shared shape for POST/PUT/PATCH bodies; both maps are optional at the wire level.
type upsertBody struct {
	Meta map[string]any `json:"meta"`
	Data map[string]any `json:"data"`
}

// readUpsertBody decodes the request body; an empty body yields both maps absent.
func readUpsertBody(r *http.Request) (*upsertBody, error) {
	var b upsertBody
	if r.Body == nil {
		return &b, nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

// create answers POST /api/collections/{tpl}. ?upsert=true overwrites; otherwise an existing GUID is 409.
// Auto-mints the GUID when missing so create can be a single request.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	tplFilename := stem + ".yaml"
	if !h.dp.IsCollectionExposed(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	t, err := h.tpl.LoadTemplate(tplFilename)
	if err != nil || t == nil {
		// Enabled but unloadable: race during deletion, treat as disabled.
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	body, err := readUpsertBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}
	if body.Data == nil {
		body.Data = map[string]any{}
	}
	if body.Meta == nil {
		body.Meta = map[string]any{}
	}

	guidKey := guidKeyOf(t)
	if guidKey == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	guidVal, _ := body.Data[guidKey].(string)
	if strings.TrimSpace(guidVal) == "" {
		guidVal = uuid.NewString()
		body.Data[guidKey] = guidVal
	}

	upsert := strings.EqualFold(r.URL.Query().Get("upsert"), "true")
	existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, guidVal)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if exists && !upsert {
		writeJSONError(w, http.StatusConflict, "already-exists")
		return
	}

	filename := ""
	isCreate := true
	if exists && existing != nil {
		filename = existing.Filename
		isCreate = false
	} else {
		filename, err = h.deriveNewFilename(t, body.Data, guidVal)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal-error")
			return
		}
	}

	envelope := map[string]any{"meta": body.Meta, "data": body.Data}
	res := h.wr.SaveForm(r.Context(), tplFilename, filename, envelope)
	if !res.Success {
		writeJSONError(w, http.StatusInternalServerError, "save-failed")
		return
	}

	status := http.StatusOK
	if isCreate {
		status = http.StatusCreated
	}
	w.Header().Set("Location", "/api/collections/"+stem+"/"+guidVal)
	writeJSON(w, status, itemResponseFromWrite(stem, guidVal, filename, body.Meta, body.Data))
}

// itemPut answers PUT /api/collections/{tpl}/{id}, replacing by GUID (?upsert=true creates with 201).
// A body guid, if present, must equal the path {id} (else 409); when absent the path id is injected.
func (h *Handler) itemPut(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	id := r.PathValue("id")
	if !validStem(stem) || id == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	tplFilename := stem + ".yaml"
	if !h.dp.IsCollectionExposed(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	t, err := h.tpl.LoadTemplate(tplFilename)
	if err != nil || t == nil {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	guidKey := guidKeyOf(t)
	if guidKey == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	body, err := readUpsertBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}
	if body.Data == nil {
		body.Data = map[string]any{}
	}
	if body.Meta == nil {
		body.Meta = map[string]any{}
	}

	if v, ok := body.Data[guidKey]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" && s != id {
			writeJSONError(w, http.StatusConflict, "guid-mismatch")
			return
		}
	}
	body.Data[guidKey] = id

	upsert := strings.EqualFold(r.URL.Query().Get("upsert"), "true")
	existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !exists && !upsert {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	filename := ""
	isCreate := !exists
	if exists && existing != nil {
		filename = existing.Filename
	} else {
		filename, err = h.deriveNewFilename(t, body.Data, id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal-error")
			return
		}
	}

	envelope := map[string]any{"meta": body.Meta, "data": body.Data}
	res := h.wr.SaveForm(r.Context(), tplFilename, filename, envelope)
	if !res.Success {
		writeJSONError(w, http.StatusInternalServerError, "save-failed")
		return
	}

	status := http.StatusOK
	if isCreate {
		status = http.StatusCreated
	}
	w.Header().Set("Location", "/api/collections/"+stem+"/"+id)
	writeJSON(w, status, itemResponseFromWrite(stem, id, filename, body.Meta, body.Data))
}

// itemPatch answers PATCH /api/collections/{tpl}/{id}, shallow-merging meta/data (per-key override,
// not deep). Optional If-Match against the collection's weak ETag guards lost updates (412 on mismatch).
func (h *Handler) itemPatch(w http.ResponseWriter, r *http.Request) {
	stem, tplFilename, t, ok := h.writeGuard(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	guidKey := guidKeyOf(t)
	if guidKey == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !exists || existing == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	// Optimistic concurrency against the collection-rev weak ETag (any write invalidates it).
	if ifMatch := r.Header.Get("If-Match"); ifMatch != "" {
		rev, err := h.dp.CollectionRev(r.Context(), tplFilename)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal-error")
			return
		}
		if ifMatch != makeETag(rev) {
			writeJSONError(w, http.StatusPreconditionFailed, "precondition-failed")
			return
		}
	}

	body, err := readUpsertBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}

	// nil here is a delete race; treat as 404 (consistent with item GET).
	prior := h.st.LoadForm(tplFilename, existing.Filename)
	if prior == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	mergedMeta := mergeMaps(formMetaAsMap(prior.Meta), body.Meta)
	mergedData := mergeMaps(prior.Data, body.Data)

	if v, present := mergedData[guidKey]; present {
		if s, _ := v.(string); strings.TrimSpace(s) != "" && s != id {
			writeJSONError(w, http.StatusConflict, "guid-mismatch")
			return
		}
	}
	mergedData[guidKey] = id

	envelope := map[string]any{"meta": mergedMeta, "data": mergedData}
	res := h.wr.SaveForm(r.Context(), tplFilename, existing.Filename, envelope)
	if !res.Success {
		writeJSONError(w, http.StatusInternalServerError, "save-failed")
		return
	}
	writeJSON(w, http.StatusOK, itemResponseFromWrite(stem, id, existing.Filename, mergedMeta, mergedData))
}

// batch answers POST /api/collections/{tpl}/batch. Per-item failures accumulate in the response
// `errors` array; a well-formed request always returns 200. ?mode=create (default)|replace|merge.
func (h *Handler) batch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}
	stem, tplFilename, t, ok := h.writeGuard(w, r)
	if !ok {
		return
	}
	guidKey := guidKeyOf(t)
	if guidKey == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "create"
	}
	switch mode {
	case "create", "replace", "merge":
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid-mode")
		return
	}

	var body struct {
		Items []struct {
			Meta map[string]any `json:"meta"`
			Data map[string]any `json:"data"`
		} `json:"items"`
		ItemsPresent bool `json:"-"`
	}
	// Decode raw to distinguish "items" absent from explicit-null.
	var raw map[string]json.RawMessage
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&raw); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid-json")
			return
		}
	}
	if itemsRaw, present := raw["items"]; present {
		if err := json.Unmarshal(itemsRaw, &body.Items); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid-json")
			return
		}
		body.ItemsPresent = true
	}
	if !body.ItemsPresent {
		writeJSONError(w, http.StatusBadRequest, "items-missing")
		return
	}

	created := []map[string]any{}
	updated := []map[string]any{}
	errs := []map[string]any{}
	for i, item := range body.Items {
		data := item.Data
		if data == nil {
			data = map[string]any{}
		}
		meta := item.Meta
		if meta == nil {
			meta = map[string]any{}
		}
		guidVal, _ := data[guidKey].(string)
		if strings.TrimSpace(guidVal) == "" {
			errs = append(errs, map[string]any{
				"index": i,
				"error": "guid-missing",
			})
			continue
		}

		existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, guidVal)
		if err != nil {
			errs = append(errs, map[string]any{
				"index": i,
				"id":    guidVal,
				"error": "internal-error",
			})
			continue
		}

		if mode == "create" && exists {
			errs = append(errs, map[string]any{
				"index": i,
				"id":    guidVal,
				"error": "already-exists",
			})
			continue
		}

		var envelope map[string]any
		var filename string
		if exists && existing != nil {
			filename = existing.Filename
			if mode == "merge" {
				prior := h.st.LoadForm(tplFilename, filename)
				priorMeta := map[string]any{}
				priorData := map[string]any{}
				if prior != nil {
					priorMeta = formMetaAsMap(prior.Meta)
					priorData = prior.Data
				}
				mergedData := mergeMaps(priorData, data)
				mergedData[guidKey] = guidVal
				envelope = map[string]any{
					"meta": mergeMaps(priorMeta, meta),
					"data": mergedData,
				}
			} else {
				// replace: full overwrite, but still force guid
				data[guidKey] = guidVal
				envelope = map[string]any{"meta": meta, "data": data}
			}
		} else {
			data[guidKey] = guidVal
			fname, derr := h.deriveNewFilename(t, data, guidVal)
			if derr != nil {
				errs = append(errs, map[string]any{
					"index": i,
					"id":    guidVal,
					"error": "internal-error",
				})
				continue
			}
			filename = fname
			envelope = map[string]any{"meta": meta, "data": data}
		}

		res := h.wr.SaveForm(r.Context(), tplFilename, filename, envelope)
		if !res.Success {
			errs = append(errs, map[string]any{
				"index":   i,
				"id":      guidVal,
				"error":   "save-failed",
				"message": res.Error,
			})
			continue
		}
		row := map[string]any{"id": guidVal, "filename": filename}
		if exists {
			updated = append(updated, row)
		} else {
			created = append(created, row)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"template":   stem,
		"mode":       mode,
		"totalItems": len(body.Items),
		"created":    created,
		"updated":    updated,
		"errors":     errs,
	})
}

// fieldPatch answers PATCH /api/collections/{tpl}/{id}/field/{key}: single-field update.
// Refuses the guid key (409), rejects unknown keys (400), force-sets data[guidKey]=id so the
// form stays addressable. Body is either {"value": ...} or a raw scalar/array/object.
func (h *Handler) fieldPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", "PATCH")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}
	stem, tplFilename, t, ok := h.writeGuard(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	key := r.PathValue("key")

	guidKey := guidKeyOf(t)
	if guidKey == "" {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	if key == guidKey {
		writeJSONError(w, http.StatusConflict, "guid-immutable")
		return
	}
	if !templateHasField(t, key) {
		writeJSONError(w, http.StatusBadRequest, "unknown-field")
		return
	}

	existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !exists || existing == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	value, err := readFieldValue(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}

	prior := h.st.LoadForm(tplFilename, existing.Filename)
	if prior == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	mergedData := mergeMaps(prior.Data, nil)
	mergedData[key] = value
	mergedData[guidKey] = id

	envelope := map[string]any{
		"meta": formMetaAsMap(prior.Meta),
		"data": mergedData,
	}
	res := h.wr.SaveForm(r.Context(), tplFilename, existing.Filename, envelope)
	if !res.Success {
		writeJSONError(w, http.StatusInternalServerError, "save-failed")
		return
	}

	rev, _ := h.dp.CollectionRev(r.Context(), tplFilename)
	writeJSON(w, http.StatusOK, map[string]any{
		"template": stem,
		"id":       id,
		"filename": existing.Filename,
		"changed":  map[string]any{key: value},
		"rev":      map[string]any{"etag": makeETag(rev)},
	})
}

// readFieldValue extracts the value for a single-field PATCH; an empty body is rejected
// so a stray PATCH .../field/x doesn't silently null out the field.
func readFieldValue(r *http.Request) (any, error) {
	if r.Body == nil {
		return nil, errors.New("api: empty body")
	}
	var raw any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	// Detect the envelope by "value" presence (not truthiness: {"value": null} sets the field to null).
	if obj, ok := raw.(map[string]any); ok {
		if v, present := obj["value"]; present {
			return v, nil
		}
	}
	return raw, nil
}

// templateHasField reports whether any field (including containers) has this key.
func templateHasField(t *template.Template, key string) bool {
	for _, f := range t.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// itemDelete answers DELETE /api/collections/{tpl}/{id}: 204 on success, 404 unresolved, 500 on storage failure.
func (h *Handler) itemDelete(w http.ResponseWriter, r *http.Request) {
	_, tplFilename, _, ok := h.writeGuard(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	existing, exists, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !exists || existing == nil {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}
	if err := h.wr.DeleteForm(tplFilename, existing.Filename); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "delete-failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeGuard runs the checks every write handler shares (valid stem, collection-enabled, loadable template);
// ok=false means the 403 was already written.
func (h *Handler) writeGuard(w http.ResponseWriter, r *http.Request) (string, string, *template.Template, bool) {
	stem := r.PathValue("tpl")
	id := r.PathValue("id")
	if !validStem(stem) || (id != "" && strings.ContainsAny(id, `/\`)) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", "", nil, false
	}
	tplFilename := stem + ".yaml"
	if !h.dp.IsCollectionExposed(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", "", nil, false
	}
	t, err := h.tpl.LoadTemplate(tplFilename)
	if err != nil || t == nil {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", "", nil, false
	}
	return stem, tplFilename, t, true
}

// mergeMaps shallow-merges over into base, returning a fresh map (caller can mutate freely).
func mergeMaps(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	maps.Copy(out, base)
	maps.Copy(out, over)
	return out
}

// deriveNewFilename slugifies the item-field value with a -2/-3 suffix on collision,
// falling back to <guid>.meta.json when the item-field is unset or slugifies to nothing.
func (h *Handler) deriveNewFilename(t *template.Template, data map[string]any, guid string) (string, error) {
	itemField := strings.TrimSpace(t.ItemField)
	var rawVal any
	if itemField != "" && data != nil {
		rawVal = data[itemField]
	}
	base := slugify(stringify(rawVal))
	if base == "" {
		return guid + ".meta.json", nil
	}
	candidate := base + ".meta.json"
	for n := 2; n < 1000; n++ {
		if h.st.LoadForm(t.Filename, candidate) == nil {
			return candidate, nil
		}
		candidate = base + "-" + strconv.Itoa(n) + ".meta.json"
	}
	return guid + ".meta.json", errors.New("api: filename collision exhaustion")
}

// slugify lowercases, strips diacritics, collapses non-alphanumeric runs to "-", caps to 80 chars.
// Must match the original JS slugify so existing forms stay reachable.
func slugify(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := true // suppresses leading dashes
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			if folded := stripDiacritic(r); folded >= 'a' && folded <= 'z' {
				b.WriteRune(folded)
				prevDash = false
			} else if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	return out
}

// stripDiacritic returns the ASCII-folded equivalent of a Latin-1 letter, or 0 when no fold applies.
func stripDiacritic(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ã', 'ä', 'å', 'ą':
		return 'a'
	case 'ç', 'ć', 'č':
		return 'c'
	case 'é', 'è', 'ê', 'ë', 'ę':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ñ', 'ń':
		return 'n'
	case 'ó', 'ò', 'ô', 'õ', 'ö', 'ø':
		return 'o'
	case 'ś', 'š', 'ß':
		return 's'
	case 'ú', 'ù', 'û', 'ü', 'ů':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	case 'ź', 'ž', 'ż':
		return 'z'
	}
	return 0
}

// guidKeyOf returns the template's guid-field key, or "" when no field has type=guid.
func guidKeyOf(t *template.Template) string {
	for _, f := range t.Fields {
		if f.Type == "guid" {
			return f.Key
		}
	}
	return ""
}

// itemResponseFromWrite shapes a write success body with the same envelope as the read endpoint.
func itemResponseFromWrite(stem, id, filename string, meta, data map[string]any) itemResponse {
	if meta == nil {
		meta = map[string]any{}
	}
	if data == nil {
		data = map[string]any{}
	}
	return itemResponse{
		Template: stem,
		ID:       id,
		Filename: filename,
		Title:    pickWriteTitle(data, meta, filename),
		Meta:     meta,
		Data:     data,
		Links: itemLinks{
			Self: "/api/collections/" + stem + "/" + id,
			HTML: "/template/" + stem + "/form/" + filename,
		},
		// Rev is left empty on writes; clients re-GET if they need it.
	}
}

// pickWriteTitle uses data["title"] if present, else filename.
func pickWriteTitle(data, _ map[string]any, filename string) string {
	if t, _ := data["title"].(string); t != "" {
		return t
	}
	return filename
}
