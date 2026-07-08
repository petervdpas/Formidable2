package datadb

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler is the pack's read-only REST API over db, for agents and scripts. It
// is GET-only: any other method is refused with 405 before routing, so the pack
// can never be mutated through it. Responses are JSON.
//
//	GET /api/                     Swagger UI (redirect to /api/docs/)
//	GET /api/openapi.json         the OpenAPI spec
//	GET /api/templates            list templates with record counts
//	GET /api/templates/{tpl}      records of one template (guid + title)
//	GET /api/records/{guid}       one record with its full payload
//	GET /api/search?q=            full-text search across records
//	GET /api/context              agent primer: how to read this bundle
//	GET /api/openapi.json         machine-readable schema
//	GET /api/docs/                interactive Swagger UI
//
// docs carries the packed per-bundle spec and context; either may be nil, in
// which case a generic version is served.
func Handler(db *DB, docs Docs) http.Handler {
	mux := http.NewServeMux()

	// Discovery: /api/ opens the interactive docs; the spec, context, and UI
	// assets sit beside it so an agent (or a human) can orient offline.
	mux.HandleFunc("/api/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/docs/", http.StatusFound)
	})
	mux.HandleFunc("/api/docs/", serveDocs)
	mux.HandleFunc("/api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if len(docs.OpenAPI) > 0 {
			_, _ = w.Write(docs.OpenAPI)
			return
		}
		_ = json.NewEncoder(w).Encode(baseOpenAPISpec())
	})
	mux.HandleFunc("/api/context", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		if len(docs.Context) > 0 {
			_, _ = w.Write(docs.Context)
			return
		}
		_, _ = w.Write(BuildContext(nil))
	})

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

// Docs are the packed, per-bundle discovery artifacts served alongside the
// data: the OpenAPI spec and the agent-context primer. Either may be empty, in
// which case a generic version is served.
type Docs struct {
	OpenAPI []byte
	Context []byte
}

// RequiresAuth reports whether an /api path serves protected data. Discovery
// (the docs UI, the OpenAPI spec, the context primer, the /api/ redirect) is
// public so an agent can orient and authorize; the data endpoints are gated.
// The token itself is enforced by the Viewer, which holds it; this only
// classifies the path.
func RequiresAuth(path string) bool {
	switch {
	case path == "/api" || path == "/api/":
		return false
	case path == "/api/openapi.json", path == "/api/context":
		return false
	case strings.HasPrefix(path, "/api/docs"):
		return false
	default:
		return strings.HasPrefix(path, "/api/")
	}
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
