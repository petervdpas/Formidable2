package datadb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func apiServer(t *testing.T) http.Handler {
	t.Helper()
	return Handler(openSample(t), Docs{})
}

func getJSON(t *testing.T, h http.Handler, path string, into any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if into != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("decode %s: %v (body=%s)", path, err, rec.Body.String())
		}
	}
	return rec.Code
}

func TestAPITemplates(t *testing.T) {
	h := apiServer(t)
	var tcs []TemplateCount
	if code := getJSON(t, h, "/api/templates", &tcs); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(tcs) != 2 {
		t.Fatalf("want 2 templates, got %+v", tcs)
	}
}

func TestAPIRecordsForTemplate(t *testing.T) {
	h := apiServer(t)
	var recs []RecordRef
	if code := getJSON(t, h, "/api/templates/kostenplaats.yaml", &recs); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %+v", recs)
	}
}

func TestAPIRecordByGUID(t *testing.T) {
	h := apiServer(t)
	var rec RecordFull
	if code := getJSON(t, h, "/api/records/k-1", &rec); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if rec.Title != "Marketing" || !strings.Contains(string(rec.Payload), "MKT") {
		t.Fatalf("record wrong: %+v", rec)
	}
}

func TestAPIRecordMissing404(t *testing.T) {
	if code := getJSON(t, apiServer(t), "/api/records/nope", nil); code != http.StatusNotFound {
		t.Fatalf("missing record status = %d, want 404", code)
	}
}

func TestAPISearch(t *testing.T) {
	h := apiServer(t)
	var hits []RecordRef
	if code := getJSON(t, h, "/api/search?q=engineering", &hits); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(hits) != 1 || hits[0].GUID != "k-2" {
		t.Fatalf("search hits = %+v", hits)
	}
}

func TestAPIRejectsNonGET(t *testing.T) {
	h := apiServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/api/records/k-1", strings.NewReader("{}")))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405", method, rec.Code)
		}
	}
}

func TestAPIRootRedirectsToDocs(t *testing.T) {
	rec := httptest.NewRecorder()
	apiServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/api/docs/" {
		t.Fatalf("/api/ = %d -> %q, want 302 -> /api/docs/", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAPIOpenAPISpec(t *testing.T) {
	h := apiServer(t)
	var spec map[string]any
	if code := getJSON(t, h, "/api/openapi.json", &spec); code != http.StatusOK {
		t.Fatalf("openapi.json status %d", code)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("spec not an openapi doc: %+v", spec["openapi"])
	}
	if _, ok := spec["paths"].(map[string]any)["/api/templates"]; !ok {
		t.Fatal("spec missing /api/templates path")
	}
}

// sampleData is a data schema shaped like the live API's DataSchemaForTemplate
// output: loop children flat, tables as arrays of row objects.
func sampleData() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code":   map[string]any{"type": "string"},
			"budget": map[string]any{"type": "number"},
			"rows":   map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		},
	}
}

func TestBuildOpenAPIEnrichesFromCollections(t *testing.T) {
	spec := BuildOpenAPI([]Collection{
		{Filename: "kostenplaats.yaml", Name: "Kostenplaats", Data: sampleData()},
	})
	if !strings.Contains(string(spec), "Fields_kostenplaats") {
		t.Fatal("per-collection schema not attached")
	}
	if !strings.Contains(string(spec), "\"kostenplaats.yaml\"") {
		t.Fatal("filename not enumerated on the tpl parameter")
	}
	if !strings.Contains(string(spec), "budget") {
		t.Fatal("field not in the schema")
	}
}

func TestAPIServesPackedSpec(t *testing.T) {
	packed := BuildOpenAPI([]Collection{{Filename: "z.yaml", Name: "Zed", Data: sampleData()}})
	h := Handler(openSample(t), Docs{OpenAPI: packed})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "z.yaml") {
		t.Fatalf("packed spec not served: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIContext(t *testing.T) {
	// Generic (unpacked) context still describes the data model.
	h := apiServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/context", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("context status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("context content-type = %q", ct)
	}
	for _, want := range []string{"Formidable", "guid", "/api/search", "relations"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("generic context missing %q", want)
		}
	}

	// Packed context lists the bundle's own collections, marking array fields.
	packed := BuildContext([]Collection{{Filename: "kostenplaats.yaml", Name: "Kostenplaats", Data: sampleData()}})
	h2 := Handler(openSample(t), Docs{Context: packed})
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/context", nil))
	body := rec2.Body.String()
	if !strings.Contains(body, "kostenplaats.yaml") || !strings.Contains(body, "Collections in this bundle") {
		t.Fatalf("packed context not served: %s", body)
	}
	if !strings.Contains(body, "rows[]") {
		t.Fatalf("array field not marked with []: %s", body)
	}
}

func TestAPIDocsShellAndAssets(t *testing.T) {
	h := apiServer(t)
	if rec := httptest.NewRecorder(); true {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/docs/", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "swagger-ui") {
			t.Fatalf("/api/docs/ = %d, body missing swagger-ui", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/docs/swagger-ui.css", nil))
	if rec.Code != http.StatusOK || len(rec.Body.Bytes()) == 0 {
		t.Fatalf("swagger-ui.css = %d, len %d", rec.Code, rec.Body.Len())
	}
}

func TestAPICORSHeader(t *testing.T) {
	h := apiServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing permissive CORS header")
	}
}
