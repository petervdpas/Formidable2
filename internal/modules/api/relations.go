package api

import (
	"net/http"
	"net/url"
	"strings"
)

// itemRelations answers GET /api/collections/{tpl}/{id}/relations: the record's
// declared relations, each with this record's outgoing linked ids and a follow
// href. The same summary feeds the single-item GET's ?expand=relations.
func (h *Handler) itemRelations(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem, id, ok := h.itemPath(w, r)
	if !ok {
		return
	}
	_, found, err := h.dp.ResolveCollectionByID(r.Context(), stem+".yaml", id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}
	sums, err := h.relationSummaries(stem, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, relationsResponse{Template: stem, ID: id, Relations: sums})
}

// relationSummaries projects a record's outgoing edges per declared relation.
// Every declared relation is listed (so the set is a stable discovery surface),
// with this record's linked ids filled in (empty when it links nothing yet).
func (h *Handler) relationSummaries(stem, id string) ([]relationSummary, error) {
	if h.rel == nil {
		return []relationSummary{}, nil // a present-but-empty array, never JSON null
	}
	defs, err := h.rel.GetRelations(stem + ".yaml")
	if err != nil {
		return nil, err
	}
	out := make([]relationSummary, 0, len(defs))
	for _, d := range defs {
		toStem := strings.TrimSuffix(d.To, ".yaml")
		ids := make([]string, 0)
		for _, e := range d.Edges {
			if e.From == id {
				ids = append(ids, e.To)
			}
		}
		out = append(out, relationSummary{
			To:          toStem,
			Cardinality: d.Cardinality,
			Inverse:     d.Inverse,
			Count:       len(ids),
			IDs:         ids,
			Href: "/api/collections/" + url.PathEscape(stem) + "/" +
				url.PathEscape(id) + "/relations/" + url.PathEscape(toStem),
		})
	}
	return out, nil
}

// itemRelationFollow answers GET /api/collections/{tpl}/{id}/relations/{to}: the
// records reached by following one relation from this record (its outgoing
// edges), resolved to full items in the target collection. Paginated via
// limit/offset. Cross-template works: the target is resolved in its own
// collection. Edges to a since-deleted target are skipped (volatile).
func (h *Handler) itemRelationFollow(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem, id, ok := h.itemPath(w, r)
	if !ok {
		return
	}
	toStem := r.PathValue("to")
	if !validStem(toStem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	toFilename := toStem + ".yaml"

	_, found, err := h.dp.ResolveCollectionByID(r.Context(), stem+".yaml", id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}
	if h.rel == nil {
		writeJSONError(w, http.StatusNotFound, "no-relation")
		return
	}
	defs, err := h.rel.GetRelations(stem + ".yaml")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	var def *RelationDef
	for i := range defs {
		if defs[i].To == toFilename {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		writeJSONError(w, http.StatusNotFound, "no-relation")
		return
	}
	// A declared relation whose target collection is disabled is a misconfig:
	// 403, not a 200 with an empty item set (which would look like all-deleted).
	if !h.dp.IsCollectionEnabled(r.Context(), toFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	targetIDs := make([]string, 0)
	for _, e := range def.Edges {
		if e.From == id {
			targetIDs = append(targetIDs, e.To)
		}
	}
	total := len(targetIDs)

	q := r.URL.Query()
	limit := atoiDefault(q.Get("limit"), defaultListLimit)
	offset := atoiDefault(q.Get("offset"), 0)
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}
	lo := offset
	if lo > total {
		lo = total
	}
	hi := lo + limit
	if hi > total {
		hi = total
	}

	items := make([]relationItem, 0, hi-lo)
	for _, tid := range targetIDs[lo:hi] {
		citem, ok, err := h.dp.ResolveCollectionByID(r.Context(), toFilename, tid)
		if err != nil || !ok {
			continue // volatile: target deleted out from under the edge
		}
		form := h.st.LoadForm(toFilename, citem.Filename)
		meta := map[string]any{}
		data := map[string]any{}
		if form != nil {
			meta = formMetaAsMap(form.Meta)
			if form.Data != nil {
				data = form.Data
			}
		}
		items = append(items, relationItem{
			Template: citem.Template,
			ID:       citem.ID,
			Filename: citem.Filename,
			Title:    citem.Title,
			Meta:     meta,
			Data:     data,
			Links:    itemLinks{Self: citem.HrefSelf, HTML: citem.HrefHTML},
		})
	}

	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, relationFollowResponse{
		Template:    stem,
		ID:          id,
		To:          toStem,
		Cardinality: def.Cardinality,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		Items:       items,
	})
}

// wantsExpand reports whether the comma-separated ?expand= list contains what.
func wantsExpand(q url.Values, what string) bool {
	for _, part := range strings.Split(q.Get("expand"), ",") {
		if strings.TrimSpace(part) == what {
			return true
		}
	}
	return false
}
