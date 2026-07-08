package datadb

import (
	"encoding/json"
	"net/http"
)

// Handler is the pack's read-only REST API over db, for agents and scripts. It
// is GET-only: any other method is refused with 405 before routing, so the pack
// can never be mutated through it. Responses are JSON.
//
//	GET /api/templates            list templates with record counts
//	GET /api/templates/{tpl}      records of one template (guid + title)
//	GET /api/records/{guid}       one record with its full payload
//	GET /api/search?q=            full-text search across records
func Handler(db *DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/templates", func(w http.ResponseWriter, r *http.Request) {
		tcs, err := db.Templates()
		respond(w, tcs, err)
	})
	mux.HandleFunc("/api/templates/{tpl}", func(w http.ResponseWriter, r *http.Request) {
		recs, err := db.Records(r.PathValue("tpl"))
		respond(w, recs, err)
	})
	mux.HandleFunc("/api/records/{guid}", func(w http.ResponseWriter, r *http.Request) {
		rec, ok, err := db.Record(r.PathValue("guid"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}
		respond(w, rec, nil)
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		hits, err := db.Search(r.URL.Query().Get("q"))
		respond(w, hits, err)
	})

	return getOnly(mux)
}

// getOnly refuses any method other than GET or HEAD before the request is
// routed, so nothing under /api can mutate state. It also allows cross-origin
// GETs, since the served data is already public within the bundle.
func getOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			next.ServeHTTP(w, r)
		case http.MethodOptions:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(w, "read-only API: method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
