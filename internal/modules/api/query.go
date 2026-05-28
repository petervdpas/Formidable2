package api

import (
	"encoding/json"
	"net/http"

	"github.com/petervdpas/formidable2/internal/modules/query"
)

// query answers POST /api/collections/{tpl}/query: run a constrained
// SELECT (the FDRM query surface) over the template's indexed values and
// return the typed Result. The {tpl} path segment is authoritative for
// the template - any Template in the body is ignored - so a caller can't
// redirect the query at a non-gated template. Gated by the same
// collection-enabled check as the rest of /api/* (not by EnabledTemplates).
func (h *Handler) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}
	tplFilename, ok := h.statTemplate(w, r)
	if !ok {
		return
	}
	var spec query.Spec
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}
	spec.Template = tplFilename
	res, err := h.qry.Run(spec)
	if err != nil {
		// A bad column index / filter op is a client error; the spec
		// reason isn't leaked as a slug.
		writeJSONError(w, http.StatusUnprocessableEntity, "bad-query")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
