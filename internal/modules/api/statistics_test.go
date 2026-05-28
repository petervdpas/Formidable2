package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/stat"
)

var errStub = errors.New("stat: bad dsl")

// stubStats is a hand-rolled Stats for the statistics endpoint tests; it
// returns canned objects/grids without standing up the engine + index.
type stubStats struct {
	objs      []stat.StatObject
	grid      *stat.Grid
	composite *stat.CompositeGrid
	evalErr   error
	dslErr    error
	lastDSL   string
}

func (s *stubStats) ListObjects(string) ([]stat.StatObject, error) { return s.objs, nil }
func (s *stubStats) EvaluateObject(_, _ string) (*stat.Grid, error) {
	return s.grid, s.evalErr
}
func (s *stubStats) EvaluateComposite(_, _ string) (*stat.CompositeGrid, error) {
	return s.composite, s.evalErr
}
func (s *stubStats) EvaluateDSL(_, dsl string) (*stat.Grid, error) {
	s.lastDSL = dsl
	return s.grid, s.dslErr
}

func sampleGrid() *stat.Grid {
	return &stat.Grid{
		Axes:     []stat.GridAxis{{Source: "status", Labels: []string{"a", "b"}}},
		Measures: []string{"count"},
		Cells: []stat.GridCell{
			{Coords: []int{0}, Values: []float64{3}, Pct: []float64{75}},
			{Coords: []int{1}, Values: []float64{1}, Pct: []float64{25}},
		},
		Total: 4,
	}
}

func statsHandler(s *stubStats) http.Handler {
	return NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), s, nil)
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestStatisticsList_ReturnsCatalog(t *testing.T) {
	s := &stubStats{objs: []stat.StatObject{
		{Name: "by-status", Label: "By status", DSL: `count() by F["status"]`},
		{Name: "combo", Label: "Combo", Composite: &stat.CompositeSpec{Parent: "by-status"}},
	}}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	out := decode[statisticsListResponse](t, rec)
	if out.Template != "recepten" || len(out.Statistics) != 2 {
		t.Fatalf("response = %+v", out)
	}
	if out.Statistics[0].Kind != "dsl" || out.Statistics[0].Href != "/api/statistics/recepten/by-status" {
		t.Errorf("dsl entry = %+v", out.Statistics[0])
	}
	if out.Statistics[1].Kind != "composite" || out.Statistics[1].DSL != "" {
		t.Errorf("composite entry = %+v", out.Statistics[1])
	}
}

func TestStatisticsList_IncludesScalingKind(t *testing.T) {
	s := &stubStats{objs: []stat.StatObject{
		{Name: "gas-apps", DSL: `records() by F["x"] scale "fcdm-urgency"`},
		{Name: "fcdm-urgency", Scaling: &stat.Scaling{Source: stat.SourceRef{Kind: stat.SourceFacet, Key: "fcdm"}}},
	}}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	out := decode[statisticsListResponse](t, rec)
	if out.Statistics[1].Kind != "scaling" {
		t.Errorf("scaling entry kind = %q, want scaling", out.Statistics[1].Kind)
	}
	if out.Statistics[1].Href != "" {
		t.Errorf("scaling entry should have no href, got %q", out.Statistics[1].Href)
	}
}

func TestStatisticEval_ScalingIs404(t *testing.T) {
	s := &stubStats{objs: []stat.StatObject{
		{Name: "fcdm-urgency", Scaling: &stat.Scaling{Source: stat.SourceRef{Kind: stat.SourceFacet, Key: "fcdm"}}},
	}}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten/fcdm-urgency")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (scaling has no grid)", rec.Code)
	}
}

func TestStatisticsList_CollectionDisabledIs403(t *testing.T) {
	// basic.yaml is not collection-enabled in newStub().
	rec := do(t, statsHandler(&stubStats{}), http.MethodGet, "/api/statistics/basic")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestStatisticEval_ReturnsGrid(t *testing.T) {
	s := &stubStats{
		objs: []stat.StatObject{{Name: "by-status", DSL: `count() by F["status"]`}},
		grid: sampleGrid(),
	}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten/by-status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	g := decode[stat.Grid](t, rec)
	if len(g.Cells) != 2 || g.Cells[0].Pct[0] != 75 {
		t.Errorf("grid = %+v", g)
	}
}

func TestStatisticEval_Composite(t *testing.T) {
	s := &stubStats{
		objs:      []stat.StatObject{{Name: "combo", Composite: &stat.CompositeSpec{Parent: "by-status"}}},
		composite: &stat.CompositeGrid{Parent: sampleGrid(), Branches: []stat.BranchGrid{{Branch: "a"}}},
	}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten/combo")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	cg := decode[stat.CompositeGrid](t, rec)
	if cg.Parent == nil || len(cg.Branches) != 1 || cg.Branches[0].Branch != "a" {
		t.Errorf("composite grid = %+v", cg)
	}
}

func TestStatisticEval_UnknownNameIs404(t *testing.T) {
	s := &stubStats{objs: []stat.StatObject{{Name: "by-status", DSL: `count()`}}}
	rec := do(t, statsHandler(s), http.MethodGet, "/api/statistics/recepten/ghost")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStatisticsAdhoc_EvaluatesDSL(t *testing.T) {
	s := &stubStats{grid: sampleGrid()}
	rec := doJSON(t, statsHandler(s), http.MethodPost, "/api/statistics/recepten", `{"dsl":"count() by F[\"status\"]"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if s.lastDSL != `count() by F["status"]` {
		t.Errorf("evaluated dsl = %q", s.lastDSL)
	}
}

func TestStatisticsAdhoc_EmptyDSLIs400(t *testing.T) {
	rec := doJSON(t, statsHandler(&stubStats{}), http.MethodPost, "/api/statistics/recepten", `{"dsl":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStatisticsAdhoc_BadDSLIs422(t *testing.T) {
	s := &stubStats{dslErr: errStub}
	rec := doJSON(t, statsHandler(s), http.MethodPost, "/api/statistics/recepten", `{"dsl":"bogus("}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestStatistics_MethodNotAllowed(t *testing.T) {
	rec := do(t, statsHandler(&stubStats{}), http.MethodPut, "/api/statistics/recepten")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
