package monitor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler builds a Handler with the journal-like fixture
// already registered — covers the happy path scenarios end-to-end.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	m := NewManager()
	m.Register(journalLikeFixture())
	return NewHandler(m)
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// /api/monitor/sources
// ─────────────────────────────────────────────────────────────────────

func TestSources_GETReturnsRegisteredList(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodGet, "/api/monitor/sources", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sources []SourceInfo `json:"sources"`
	}
	decodeJSON(t, rec, &body)
	if len(body.Sources) != 1 {
		t.Fatalf("len = %d, want 1", len(body.Sources))
	}
	if body.Sources[0].Name != "journal" {
		t.Errorf("name = %q", body.Sources[0].Name)
	}
}

func TestSources_RejectsNonGET(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/sources", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────
// /api/monitor/query
// ─────────────────────────────────────────────────────────────────────

func TestQuery_HappyPath(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/query", `{"source":"journal","group_by":["op"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res Result
	decodeJSON(t, rec, &res)
	if len(res.Series) != 3 {
		t.Errorf("expected 3 series (create/update/delete), got %d", len(res.Series))
	}
}

func TestQuery_RejectsNonPOST(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodGet, "/api/monitor/query", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestQuery_BadJSONReturns400(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/query", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	var body errorBody
	decodeJSON(t, rec, &body)
	if body.Error != "bad-body" {
		t.Errorf("error = %q, want bad-body", body.Error)
	}
}

func TestQuery_UnknownSourceReturns404(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/query", `{"source":"ghost"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	var body errorBody
	decodeJSON(t, rec, &body)
	if body.Error != "unknown-source" {
		t.Errorf("error = %q, want unknown-source", body.Error)
	}
}

func TestQuery_BadBinReturns400(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/query", `{"source":"journal","bin":"banana"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	var body errorBody
	decodeJSON(t, rec, &body)
	if body.Error != "bad-query" {
		t.Errorf("error = %q, want bad-query", body.Error)
	}
}

func TestQuery_FromAfterToReturns400(t *testing.T) {
	h := newTestHandler(t)
	body := `{"source":"journal","from":"2026-05-09T12:00:00Z","to":"2026-05-09T08:00:00Z"}`
	rec := do(t, h, http.MethodPost, "/api/monitor/query", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestQuery_RoundTripsBinnedResult(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodPost, "/api/monitor/query", `{"source":"journal","group_by":["op"],"bin":"1h"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Confirm the binned result shape made it through JSON round-trip
	// without losing the Points / Ts fields.
	var got Result
	decodeJSON(t, rec, &got)
	for _, s := range got.Series {
		if s.Total != 0 {
			t.Errorf("Total should be zero on binned series: %+v", s)
		}
		if len(s.Points) == 0 {
			t.Errorf("expected non-empty Points on binned series: %+v", s)
		}
		for _, p := range s.Points {
			if p.Ts.IsZero() {
				t.Errorf("Point.Ts is zero: %+v", p)
			}
		}
	}
}

func TestHandler_ContentTypeHeader(t *testing.T) {
	h := newTestHandler(t)
	rec := do(t, h, http.MethodGet, "/api/monitor/sources", "")
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHandler_QueryEndpoint_AcceptsEmptyBodyError(t *testing.T) {
	// Empty body to POST shouldn't crash — should produce a clean 400.
	h := newTestHandler(t)
	r := httptest.NewRequest(http.MethodPost, "/api/monitor/query", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (got body: %s)", rec.Code, rec.Body.String())
	}
}
