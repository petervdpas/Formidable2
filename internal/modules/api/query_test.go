package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/query"
)

// stubQuery is a hand-rolled Query for the endpoint tests; it records the
// spec it received and returns canned output without the datacore.
type stubQuery struct {
	res      query.Result
	err      error
	lastSpec query.Spec
}

func (s *stubQuery) Run(spec query.Spec) (query.Result, error) {
	s.lastSpec = spec
	return s.res, s.err
}

func queryHandler(q Query) http.Handler {
	return NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, q)
}

func TestQueryEndpoint_RunsAndForcesTemplateFromPath(t *testing.T) {
	q := &stubQuery{res: query.Result{
		Columns: []string{"Status"},
		Rows:    [][]query.Cell{{{Text: "open"}}},
		Count:   1,
		Total:   2,
	}}
	body := `{"template":"ignored.yaml","columns":[{"header":"Status","source":{"kind":"field","key":"status"}}]}`
	rec := doJSON(t, queryHandler(q), http.MethodPost, "/api/collections/recepten/query", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	// Path is authoritative: the body's template is overridden.
	if q.lastSpec.Template != "recepten.yaml" {
		t.Errorf("template = %q, want recepten.yaml (path wins over body)", q.lastSpec.Template)
	}
	out := decode[query.Result](t, rec)
	if out.Count != 1 || out.Total != 2 || len(out.Rows) != 1 || out.Rows[0][0].Text != "open" {
		t.Errorf("result = %+v", out)
	}
}

func TestQueryEndpoint_MethodNotAllowed(t *testing.T) {
	rec := do(t, queryHandler(&stubQuery{}), http.MethodGet, "/api/collections/recepten/query")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestQueryEndpoint_CollectionDisabled(t *testing.T) {
	// "basic" is not collection-enabled in newStub.
	rec := doJSON(t, queryHandler(&stubQuery{}), http.MethodPost, "/api/collections/basic/query", `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestQueryEndpoint_BadJSON(t *testing.T) {
	rec := doJSON(t, queryHandler(&stubQuery{}), http.MethodPost, "/api/collections/recepten/query", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestQueryEndpoint_RunErrorIsUnprocessable(t *testing.T) {
	q := &stubQuery{err: errors.New("order column 5 out of range")}
	rec := doJSON(t, queryHandler(q), http.MethodPost, "/api/collections/recepten/query",
		`{"columns":[{"header":"S","source":{"kind":"field","key":"s"}}],"orderBy":[{"column":5}]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body.String())
	}
}
