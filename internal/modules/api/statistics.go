package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// statObjectEntry is one named statistical object in the catalog listing.
type statObjectEntry struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind"`
	DSL   string `json:"dsl,omitempty"`
	Href  string `json:"href,omitempty"`
}

// statisticsListResponse is the body of GET /api/statistics/{tpl}.
type statisticsListResponse struct {
	Template   string            `json:"template"`
	Statistics []statObjectEntry `json:"statistics"`
}

// adhocBody is the POST /api/statistics/{tpl} request: an inline DSL to evaluate.
type adhocBody struct {
	DSL string `json:"dsl"`
}

// statistics answers /api/statistics/{tpl}: GET lists named objects, POST evaluates an ad-hoc DSL.
func (h *Handler) statistics(w http.ResponseWriter, r *http.Request) {
	tplFilename, ok := h.statTemplate(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.statisticsList(w, r, tplFilename)
	case http.MethodPost:
		h.statisticsAdhoc(w, r, tplFilename)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	}
}

func (h *Handler) statisticsList(w http.ResponseWriter, r *http.Request, tplFilename string) {
	objs, err := h.stats.ListObjects(tplFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	stem := strings.TrimSuffix(tplFilename, ".yaml")
	out := statisticsListResponse{Template: stem, Statistics: make([]statObjectEntry, 0, len(objs))}
	for _, o := range objs {
		kind := "dsl"
		switch {
		case o.Composite != nil:
			kind = "composite"
		case o.Scaling != nil:
			kind = "scaling"
		}
		entry := statObjectEntry{
			Name:  o.Name,
			Label: o.Label,
			Kind:  kind,
			DSL:   o.DSL,
		}
		// A scaling has no grid of its own, so no evaluation href.
		if kind != "scaling" {
			entry.Href = "/api/statistics/" + stem + "/" + o.Name
		}
		out.Statistics = append(out.Statistics, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) statisticsAdhoc(w http.ResponseWriter, r *http.Request, tplFilename string) {
	var b adhocBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid-json")
		return
	}
	if strings.TrimSpace(b.DSL) == "" {
		writeJSONError(w, http.StatusBadRequest, "bad-dsl")
		return
	}
	grid, err := h.stats.EvaluateDSL(tplFilename, b.DSL)
	if err != nil {
		// Bad DSL is a client error; the engine's parse/eval reason isn't leaked as a slug.
		writeJSONError(w, http.StatusUnprocessableEntity, "bad-dsl")
		return
	}
	writeJSON(w, http.StatusOK, grid)
}

// statistic answers GET /api/statistics/{tpl}/{name}, evaluating one object (composite or plain) to its grid.
func (h *Handler) statistic(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	tplFilename, ok := h.statTemplate(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !validStem(name) {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	objs, err := h.stats.ListObjects(tplFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	var composite, scaling bool
	var found bool
	for _, o := range objs {
		if o.Name == name {
			found = true
			composite = o.Composite != nil
			scaling = o.Scaling != nil
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}
	if scaling {
		// A scaling has nothing to evaluate on its own.
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	if composite {
		cg, err := h.stats.EvaluateComposite(tplFilename, name)
		if err != nil {
			writeJSONError(w, http.StatusUnprocessableEntity, "evaluate-failed")
			return
		}
		writeJSON(w, http.StatusOK, cg)
		return
	}
	grid, err := h.stats.EvaluateObject(tplFilename, name)
	if err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, "evaluate-failed")
		return
	}
	writeJSON(w, http.StatusOK, grid)
}

// statTemplate validates {tpl} and applies the collection-enabled gate; ok=false means the error was already written.
func (h *Handler) statTemplate(w http.ResponseWriter, r *http.Request) (string, bool) {
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", false
	}
	filename := stem + ".yaml"
	if !h.dp.IsCollectionExposed(r.Context(), filename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", false
	}
	return filename, true
}
