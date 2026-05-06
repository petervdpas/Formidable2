package wiki

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
)

// stubProvider is a hand-rolled dataprovider that lets each test
// shape the corpus without touching disk or SQLite. The wiki handler
// only needs ListTemplates, GetTemplate, ListForms, GetFormSummary,
// and RenderForm — keep the surface tight so the test stub stays
// small.
type stubProvider struct {
	templates []dataprovider.TemplateSummary
	forms     map[string][]dataprovider.FormSummary // keyed by template filename
	render    func(template, datafile string) (*dataprovider.RenderedPage, error)
}

func (s *stubProvider) ListTemplates(_ context.Context) ([]dataprovider.TemplateSummary, error) {
	return s.templates, nil
}

func (s *stubProvider) GetTemplate(_ context.Context, filename string) (*dataprovider.TemplateSummary, bool, error) {
	for i := range s.templates {
		if s.templates[i].Filename == filename || s.templates[i].Stem == filename {
			t := s.templates[i]
			return &t, true, nil
		}
	}
	return nil, false, nil
}

func (s *stubProvider) ListForms(_ context.Context, template string, _ dataprovider.ListOpts) ([]dataprovider.FormSummary, error) {
	return s.forms[template], nil
}

func (s *stubProvider) GetFormSummary(_ context.Context, template, datafile string) (*dataprovider.FormSummary, bool, error) {
	for _, f := range s.forms[template] {
		if f.Filename == datafile {
			ff := f
			return &ff, true, nil
		}
	}
	return nil, false, nil
}

func (s *stubProvider) RenderForm(_ context.Context, template, datafile string) (*dataprovider.RenderedPage, error) {
	if s.render != nil {
		return s.render(template, datafile)
	}
	return nil, errors.New("not configured")
}

func newStubProvider() *stubProvider {
	return &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "basic", Filename: "basic.yaml", Name: "Basic Form"},
			{Stem: "recepten", Filename: "recepten.yaml", Name: "Recepten", EnableCollection: true, GuidField: "id"},
		},
		forms: map[string][]dataprovider.FormSummary{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "x.meta.json", Title: "X"},
				{Template: "basic.yaml", Filename: "y.meta.json", Title: "Y"},
			},
		},
		render: func(_, datafile string) (*dataprovider.RenderedPage, error) {
			return &dataprovider.RenderedPage{
				Template: "basic.yaml",
				Filename: datafile,
				Title:    "Title-" + datafile,
				HTML:     "<p>rendered:" + datafile + "</p>",
			}, nil
		},
	}
}

func newTestHandler(t *testing.T) (http.Handler, *stubProvider) {
	t.Helper()
	sp := newStubProvider()
	h := NewHandler(sp)
	return h, sp
}

// ── index ──────────────────────────────────────────────────────────

func TestIndex_ListsTemplates(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `href="/template/basic"`) {
		t.Errorf("missing link to /template/basic; body=%q", body)
	}
	if !strings.Contains(body, `href="/template/recepten"`) {
		t.Errorf("missing link to /template/recepten; body=%q", body)
	}
	if !strings.Contains(body, "Basic Form") {
		t.Errorf("missing template name; body=%q", body)
	}
}

// ── /template/{tpl} ────────────────────────────────────────────────

func TestTemplate_ListsForms(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `href="/template/basic/form/x.meta.json"`) {
		t.Errorf("missing x form link; body=%q", body)
	}
	if !strings.Contains(body, `href="/template/basic/form/y.meta.json"`) {
		t.Errorf("missing y form link; body=%q", body)
	}
}

func TestTemplate_UnknownReturns404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/ghost", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestTemplate_PostReturns405(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/template/basic", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ── /template/{tpl}/form/{datafile} ────────────────────────────────

func TestForm_RendersBody(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/form/x.meta.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "rendered:x.meta.json") {
		t.Errorf("body missing rendered output; body=%q", body)
	}
	if !strings.Contains(body, "<title>") {
		t.Errorf("body missing <title>; body=%q", body)
	}
}

func TestForm_UnknownTemplate404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/ghost/form/x.meta.json", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestForm_UnknownFile404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/form/ghost.meta.json", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestForm_RenderError500(t *testing.T) {
	sp := newStubProvider()
	sp.render = func(_, _ string) (*dataprovider.RenderedPage, error) {
		return nil, errors.New("boom")
	}
	h := NewHandler(sp)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/form/x.meta.json", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ── 404 / negative cases ───────────────────────────────────────────

func TestUnknownPath404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/no/such/path", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// Live integration through Manager — exercises SetHandler + Serve.
func TestHandlerWiredToManager(t *testing.T) {
	sp := newStubProvider()
	h := NewHandler(sp)
	m := NewManager(nil)
	m.SetHandler(h)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	port := m.Status().Port
	resp, err := http.Get("http://127.0.0.1:" + intStr(port) + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Basic Form") {
		t.Errorf("body missing template name; body=%q", body)
	}
}

func intStr(n int) string {
	// avoid pulling strconv just for one site — this is test-only.
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
