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
	"github.com/petervdpas/formidable2/internal/modules/expression"
)

// stubExpressioner returns canned sidebar items keyed by template
// filename. A nil items slice for a template makes EvaluateList
// return ErrNoExpression (the same signal `*expression.Manager` emits
// when sidebar_expression isn't set), so handler tests can exercise
// both "expression configured" and "no expression — fall back to
// filename" paths without spinning up the engine.
type stubExpressioner struct {
	items map[string][]expression.Result
	err   error
}

func (s *stubExpressioner) EvaluateList(templateName string) ([]expression.Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	v, ok := s.items[templateName]
	if !ok {
		return nil, expression.ErrNoExpression
	}
	return v, nil
}

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

// stubStorage is the bytes-side counterpart to stubProvider — keeps
// the handler tests free of disk and the storage manager.
type stubStorage struct {
	// images keyed by "<templateFilename>/<name>"
	images map[string][]byte
	// returnError simulates an unexpected error path.
	returnError error
}

func (s *stubStorage) OpenImageFile(templateFilename, name string) ([]byte, string, error) {
	if s.returnError != nil {
		return nil, "", s.returnError
	}
	key := templateFilename + "/" + name
	raw, ok := s.images[key]
	if !ok {
		return nil, "", nil
	}
	mime := "image/png"
	if strings.HasSuffix(name, ".svg") {
		mime = "image/svg+xml"
	}
	return raw, mime, nil
}

func newStubStorage() *stubStorage {
	return &stubStorage{
		images: map[string][]byte{
			"basic.yaml/logo.png": []byte("PNGBYTES"),
		},
	}
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
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	return h, sp
}

func newTestHandlerWithExpr(t *testing.T, expr Expressioner) (http.Handler, *stubProvider) {
	t.Helper()
	sp := newStubProvider()
	h := NewHandler(sp, newStubStorage(), expr)
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

func TestTemplate_FormList_UsesExpressionSubtitles(t *testing.T) {
	expr := &stubExpressioner{
		items: map[string][]expression.Result{
			"basic.yaml": {
				{Filename: "x.meta.json", Text: "Direct + Indirect", Classes: []string{"expr-text-green"}, Color: "#0a0"},
				{Filename: "y.meta.json", Text: "NIET IN GEBRUIK", Classes: []string{"expr-text-gray"}},
			},
		},
	}
	h, _ := newTestHandlerWithExpr(t, expr)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	// html/template escapes "+" as "&#43;" — match either form.
	if !strings.Contains(body, "Direct + Indirect") && !strings.Contains(body, "Direct &#43; Indirect") {
		t.Errorf("expression subtitle missing; body=%q", body)
	}
	if !strings.Contains(body, "NIET IN GEBRUIK") {
		t.Errorf("second expression subtitle missing; body=%q", body)
	}
	if !strings.Contains(body, "expr-text-green") {
		t.Errorf("expression classes not applied; body=%q", body)
	}
	if !strings.Contains(body, "color: #0a0") {
		t.Errorf("expression inline color not applied; body=%q", body)
	}
	// Raw filename must NOT appear as the visible subtitle when the
	// expression supplied a real text (it's still in the href).
	if strings.Contains(body, ">x.meta.json<") {
		t.Errorf("raw filename leaked into subtitle; body=%q", body)
	}
}

func TestTemplate_FormList_FallsBackToFilenameWhenNoExpression(t *testing.T) {
	// Empty expression stub → EvaluateList returns ErrNoExpression
	// → handler must fall back to filename for every row's subtitle.
	h, _ := newTestHandlerWithExpr(t, &stubExpressioner{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, ">x.meta.json<") {
		t.Errorf("filename fallback missing for x; body=%q", body)
	}
	if !strings.Contains(body, ">y.meta.json<") {
		t.Errorf("filename fallback missing for y; body=%q", body)
	}
}

func TestTemplate_FormList_NilExpressionerFallsBackToFilename(t *testing.T) {
	// Defensive: explicit nil Expressioner → handler must not panic
	// and every row falls back to filename.
	h, _ := newTestHandlerWithExpr(t, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, ">x.meta.json<") {
		t.Errorf("filename fallback missing under nil expressioner; body=%q", body)
	}
}

func TestIndex_DoesNotShowStemBesideName(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	// Display name must appear; bare stem next to it must NOT.
	if !strings.Contains(body, "Basic Form") {
		t.Errorf("display name missing; body=%q", body)
	}
	if strings.Contains(body, `<span class="muted">basic</span>`) {
		t.Errorf("redundant stem leaked into index page; body=%q", body)
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
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/form/x.meta.json", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ── /storage/{tpl}/images/{name} ───────────────────────────────────

func TestStorage_ServesExistingImage(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/storage/basic/images/logo.png", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	if w.Body.String() != "PNGBYTES" {
		t.Errorf("body = %q, want PNGBYTES", w.Body.String())
	}
}

func TestStorage_MissingImage404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/storage/basic/images/ghost.png", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStorage_PostReturns405(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/storage/basic/images/logo.png", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestStorage_RejectsTraversalInImageName(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/storage/basic/images/..secret", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStorage_StorageErrorReturns500(t *testing.T) {
	sp := newStubProvider()
	st := newStubStorage()
	st.returnError = errors.New("boom")
	h := NewHandler(sp, st, &stubExpressioner{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/storage/basic/images/logo.png", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ── /_/<path> static asset route ───────────────────────────────────

func TestStatic_ServesEmbeddedCSS(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/css/base.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content-type = %q, want text/css", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("empty body")
	}
}

func TestStatic_ServesEmbeddedJS(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/js/crumbs.js", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("content-type = %q, want text/javascript", ct)
	}
}

func TestStatic_ServesLogoPNG(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/img/logo.png", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q, want image/png", ct)
	}
}

func TestStatic_FormidableProse_StreamsRenderModuleCSS(t *testing.T) {
	// /_/css/formidable-prose.css must come from render.ProseCSS so
	// the wiki view stays byte-identical to the in-app slideout's
	// rendered preview. Asserts we hit the special-case branch.
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/css/formidable-prose.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, ".formidable-prose") {
		t.Errorf("body missing .formidable-prose selector; got %d bytes", len(body))
	}
}

func TestStatic_TraversalReturns404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/css/..%2Fsecrets.txt", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStatic_MissingFile404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_/css/ghost.css", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ── topbar + meta in HTML pages ───────────────────────────────────

func TestPages_LinkTopbarAssets(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	body := w.Body.String()
	for _, want := range []string{
		`/_/css/base.css`,
		`/_/css/header.css`,
		`/_/css/content.css`,
		`/_/css/formidable-prose.css`,
		`/_/js/crumbs.js`,
		`/_/js/filter.js`,
		`id="topbar"`,
		`id="crumbs"`,
		`id="q"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestTemplatePage_EmitsDataTagsForFilter(t *testing.T) {
	sp := newStubProvider()
	sp.forms["basic.yaml"] = []dataprovider.FormSummary{
		{Template: "basic.yaml", Filename: "x.meta.json", Title: "X", Tags: []string{"alpha", "beta"}},
		{Template: "basic.yaml", Filename: "y.meta.json", Title: "Y", Tags: []string{}},
	}
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic", nil))
	body := w.Body.String()
	if !strings.Contains(body, `data-tags="alpha,beta"`) {
		t.Errorf("missing data-tags=alpha,beta; body=%q", body)
	}
	if !strings.Contains(body, `data-tags=""`) {
		t.Errorf("expected empty data-tags for tagless form")
	}
}

func TestFormPage_EmitsMetaForCrumbs(t *testing.T) {
	h, _ := newTestHandler(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/template/basic/form/x.meta.json", nil))
	body := w.Body.String()
	if !strings.Contains(body, `window.__FORMIDABLE__`) {
		t.Error("missing window.__FORMIDABLE__ assignment")
	}
	if !strings.Contains(body, `"templateId": "basic"`) {
		t.Errorf("meta missing templateId; body=%q", body)
	}
	if !strings.Contains(body, `"formFile": "x.meta.json"`) {
		t.Errorf("meta missing formFile; body=%q", body)
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
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
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
