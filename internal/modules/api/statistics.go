package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// statObjectEntry is one named statistical object in the catalog listing.
// Kind is "dsl" or "composite"; DSL is present for plain objects, empty
// for composites (whose structure lives on the template, not as a string).
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

// adhocBody is the POST /api/statistics/{tpl} request: an inline DSL to
// evaluate without naming a stored object.
type adhocBody struct {
	DSL string `json:"dsl"`
}

// statistics answers /api/statistics/{tpl}: GET lists the template's named
// statistical objects; POST evaluates an ad-hoc DSL against it. The grid is
// the engine's presentation-free JSON (axes / measures / cells[].values /
// cells[].pct), ready for an external consumer to reshape (e.g. into an R
// data.frame).
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
		// A scaling is a reusable weighting referenced by other objects; it
		// has no grid of its own, so no evaluation href.
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
		// A bad DSL is a client error; the engine surfaces the parse/eval
		// reason, which we don't leak as a slug.
		writeJSONError(w, http.StatusUnprocessableEntity, "bad-dsl")
		return
	}
	writeJSON(w, http.StatusOK, grid)
}

// statistic answers GET /api/statistics/{tpl}/{name}: evaluate one named
// object to its grid (or, for a composite, its parent+branches grid). The
// object's kind is resolved from the catalog so the right evaluator runs.
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
		// A scaling is a weighting referenced by other objects, not a grid;
		// there is nothing to evaluate on its own.
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

// statTemplate validates the {tpl} segment and enforces the same
// collection-enabled gate as the rest of the API surface, so statistics are
// exposed exactly for the templates the API already publishes. Returns the
// ".yaml" filename and ok=false (after writing the error) when blocked.
func (h *Handler) statTemplate(w http.ResponseWriter, r *http.Request) (string, bool) {
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", false
	}
	filename := stem + ".yaml"
	if !h.dp.IsCollectionEnabled(r.Context(), filename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", false
	}
	return filename, true
}
