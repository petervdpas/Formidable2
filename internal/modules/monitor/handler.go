package monitor

import (
	"encoding/json"
	"errors"
	"net/http"
)

// errorBody mirrors the api package's `{ "error": "<slug>" }` envelope
// so external consumers see one consistent error shape across both
// /api/collections/* and /api/monitor/*.
type errorBody struct {
	Error string `json:"error"`
}

// Handler exposes /api/monitor/* as an http.Handler. The composition
// root mounts the returned handler alongside api.Handler — both share
// the same underlying Manager / Source registry as the Wails Service.
type Handler struct {
	m *Manager
}

// NewHandler builds the monitor HTTP handler. Returns http.Handler so
// callers compose through the standard interface; routes are private.
func NewHandler(m *Manager) http.Handler {
	h := &Handler{m: m}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/monitor/sources", h.sources)
	mux.HandleFunc("/api/monitor/query", h.query)
	return mux
}

// sources answers GET /api/monitor/sources with the registered Source
// descriptors. POST/PUT/etc are 405 — read-only.
func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sources": h.m.ListSources(),
	})
}

// query answers POST /api/monitor/query — body is the Query JSON,
// response is the Result JSON. POST so the body has room for filter,
// groupBy, time range without URL-encoded juggling.
func (h *Handler) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method-not-allowed")
		return
	}
	var q Query
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "bad-body")
		return
	}
	res, err := h.m.Run(q)
	if err != nil {
		// ErrUnknownSource is a 404; everything else (bad bin, from > to)
		// is a 400. A future "source backend down" failure would map
		// to 503 — not yet a concern with only the synchronous JournalSource.
		if errors.Is(err, ErrUnknownSource) {
			writeError(w, http.StatusNotFound, "unknown-source")
			return
		}
		writeError(w, http.StatusBadRequest, "bad-query")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, slug string) {
	writeJSON(w, status, errorBody{Error: slug})
}
