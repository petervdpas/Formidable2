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
	return Handler(openSample(t))
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

func TestAPICORSHeader(t *testing.T) {
	h := apiServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing permissive CORS header")
	}
}
