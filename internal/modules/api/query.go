package api

import (
	"encoding/json"
	"net/http"

	"github.com/petervdpas/formidable2/internal/modules/query"
)

// query answers POST /api/collections/{tpl}/query, running a constrained SELECT over indexed values.
// The {tpl} path segment is authoritative; a body Template is ignored so a caller can't redirect at an ungated template.
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
		// Bad column index/filter op is a client error; the reason isn't leaked as a slug.
		writeJSONError(w, http.StatusUnprocessableEntity, "bad-query")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
