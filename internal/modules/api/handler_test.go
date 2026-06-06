package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// stubProvider is a hand-rolled Provider for the api unit tests.
// Mirrors the wiki package's stubProvider style - narrow, manually
// shaped per scenario, no SQLite or filesystem.
type stubProvider struct {
	templates []dataprovider.TemplateSummary
	// forms keyed by template filename
	forms map[string][]dataprovider.FormSummary
	// rev is the value CollectionRev returns; tests bump it when they
	// want to invalidate a previously-captured ETag.
	rev int64
	// listCollectionErr forces an error from ListCollection so we can
	// exercise the 500 path without contorting the other returns.
	listCollectionErr error
	// collectionRevErr exercises the 500 branch in `list`.
	collectionRevErr error
	// storage, when set, lets ListCollection apply facet filtering
	// against the same backing store stubStorage uses. Mirrors the real
	// dataprovider's m.sto.LoadForm path without spinning up an index.
	storage *stubStorage
}

func (s *stubProvider) ListTemplates(_ context.Context) ([]dataprovider.TemplateSummary, error) {
	return s.templates, nil
}

func (s *stubProvider) IsCollectionEnabled(_ context.Context, template string) bool {
	for _, t := range s.templates {
		if t.Filename == template {
			return t.EnableCollection && t.GuidField != ""
		}
	}
	return false
}

func (s *stubProvider) ListCollection(_ context.Context, template string, opts dataprovider.CollectionListOpts) (*dataprovider.CollectionPage, error) {
	if s.listCollectionErr != nil {
		return nil, s.listCollectionErr
	}
	if !s.IsCollectionEnabled(context.Background(), template) {
		return &dataprovider.CollectionPage{Enabled: false}, nil
	}
	rows := s.forms[template]
	// Drop rows without an addressable GUID - mirrors real dataprovider.
	addressable := rows[:0:0]
	for _, r := range rows {
		if r.ID != "" {
			addressable = append(addressable, r)
		}
	}
	rows = addressable

	// Tags AND filter: every requested tag must be present.
	if len(opts.Tags) > 0 {
		want := map[string]struct{}{}
		for _, t := range opts.Tags {
			want[t] = struct{}{}
		}
		filtered := rows[:0:0]
		for _, r := range rows {
			has := map[string]struct{}{}
			for _, t := range r.Tags {
				has[t] = struct{}{}
			}
			ok := true
			for k := range want {
				if _, found := has[k]; !found {
					ok = false
					break
				}
			}
			if ok {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	// Facet AND filter - mirrors dataprovider/collection.go: for every
	// requested facet.<k>=L the record's meta.facets[k] must satisfy
	// set==true and selected==L. Without a wired-up storage stub this
	// branch is a no-op (the empty-Facets case in real life).
	if len(opts.Facets) > 0 && s.storage != nil {
		filtered := rows[:0:0]
		for _, r := range rows {
			form := s.storage.LoadForm(template, r.Filename)
			if form == nil {
				continue
			}
			match := true
			for key, want := range opts.Facets {
				st, ok := form.Meta.Facets[key]
				if !ok || !st.Set || st.Selected != want {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	// Substring filter (case-insensitive over title + tags) - matches
	// the real dataprovider implementation in `collection.go`.
	if opts.Q != "" {
		needle := strings.ToLower(opts.Q)
		filtered := rows[:0:0]
		var sb strings.Builder
		for _, r := range rows {
			sb.Reset()
			sb.WriteString(strings.ToLower(r.Title))
			for _, t := range r.Tags {
				sb.WriteByte(' ')
				sb.WriteString(strings.ToLower(t))
			}
			if strings.Contains(sb.String(), needle) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	total := len(rows)
	if opts.Offset > 0 {
		if opts.Offset >= len(rows) {
			rows = nil
		} else {
			rows = rows[opts.Offset:]
		}
	}
	if opts.Limit > 0 && len(rows) > opts.Limit {
		rows = rows[:opts.Limit]
	}

	stem := strings.TrimSuffix(template, ".yaml")
	items := make([]dataprovider.CollectionItem, len(rows))
	for i, r := range rows {
		items[i] = dataprovider.CollectionItem{
			Template: stem,
			ID:       r.ID,
			Filename: r.Filename,
			Title:    r.Title,
			Tags:     append([]string(nil), r.Tags...),
			HrefSelf: "/api/collections/" + stem + "/" + r.ID,
			HrefHTML: "/template/" + stem + "/form/" + r.Filename,
		}
	}
	return &dataprovider.CollectionPage{
		Enabled:  true,
		Template: stem,
		Total:    total,
		Limit:    opts.Limit,
		Offset:   opts.Offset,
		Items:    items,
	}, nil
}

func (s *stubProvider) CollectionRev(_ context.Context, _ string) (int64, error) {
	if s.collectionRevErr != nil {
		return 0, s.collectionRevErr
	}
	return s.rev, nil
}

func (s *stubProvider) ResolveCollectionByID(_ context.Context, template, id string) (*dataprovider.CollectionItem, bool, error) {
	if !s.IsCollectionEnabled(context.Background(), template) {
		return nil, false, nil
	}
	stem := strings.TrimSuffix(template, ".yaml")
	for _, r := range s.forms[template] {
		if r.ID == id {
			return &dataprovider.CollectionItem{
				Template: stem,
				ID:       r.ID,
				Filename: r.Filename,
				Title:    r.Title,
				Tags:     append([]string(nil), r.Tags...),
				HrefSelf: "/api/collections/" + stem + "/" + r.ID,
				HrefHTML: "/template/" + stem + "/form/" + r.Filename,
			}, true, nil
		}
	}
	return nil, false, nil
}

// stubStorage is the bytes-side counterpart to stubProvider. Loaded
// forms are keyed by "<templateFilename>/<datafile>" so the test can
// place arbitrary content per form. Images are keyed by
// "<templateFilename>/<filename>" so a single stub satisfies both the
// form-load surface and the image-bytes surface used by /api/images.
type stubStorage struct {
	forms  map[string]*storage.Form
	images map[string][]byte
}

func (s *stubStorage) LoadForm(templateFilename, datafile string) *storage.Form {
	return s.forms[templateFilename+"/"+datafile]
}

// OpenImageFile mirrors *storage.Manager.OpenImageFile semantics:
// missing file → nil bytes + nil error; traversal/empty rejected.
// MIME comes from the filename extension, matching the production
// imageMIMEFromName helper closely enough for the api unit tests.
func (s *stubStorage) OpenImageFile(templateFilename, name string) ([]byte, string, error) {
	if name == "" {
		return nil, "", errors.New("storage: empty image name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return nil, "", fmt.Errorf("storage: invalid image name %q", name)
	}
	bytes, ok := s.images[templateFilename+"/"+name]
	if !ok {
		return nil, "", nil
	}
	return bytes, stubImageMIME(name), nil
}

func (s *stubStorage) putImage(templateFilename, name string, bytes []byte) {
	s.images[templateFilename+"/"+name] = bytes
}

func stubImageMIME(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func newStubStorage() *stubStorage {
	return &stubStorage{forms: map[string]*storage.Form{}, images: map[string][]byte{}}
}

// stubWriter is a small in-memory write surface. Tests can peek
// `saves`/`deletes` to assert side-effects. When `st` is set, writes
// are mirrored into the read store so a subsequent GET sees the
// fresh state.
type stubWriter struct {
	st      *stubStorage
	saves   []stubWrite
	deletes []stubWrite
	saveErr string
	delErr  error
}

type stubWrite struct {
	Template string
	Datafile string
	Envelope map[string]any
}

func (w *stubWriter) SaveForm(_ context.Context, templateFilename, datafile string, data map[string]any) storage.SaveResult {
	w.saves = append(w.saves, stubWrite{Template: templateFilename, Datafile: datafile, Envelope: data})
	if w.saveErr != "" {
		return storage.SaveResult{Success: false, Error: w.saveErr}
	}
	if w.st != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return storage.SaveResult{Success: false, Error: err.Error()}
		}
		var f storage.Form
		if err := json.Unmarshal(raw, &f); err != nil {
			return storage.SaveResult{Success: false, Error: err.Error()}
		}
		w.st.forms[templateFilename+"/"+datafile] = &f
	}
	return storage.SaveResult{Success: true, Path: datafile}
}

func (w *stubWriter) DeleteForm(templateFilename, datafile string) error {
	w.deletes = append(w.deletes, stubWrite{Template: templateFilename, Datafile: datafile})
	if w.delErr != nil {
		return w.delErr
	}
	if w.st != nil {
		delete(w.st.forms, templateFilename+"/"+datafile)
	}
	return nil
}

func newStubWriter() *stubWriter { return &stubWriter{} }

// stubTemplates is a minimal Templates impl. Tests register full
// *template.Template values per filename - missing entries return a
// not-found error so the handler's 404 branch is reachable.
type stubTemplates struct {
	by map[string]*template.Template
}

func (s *stubTemplates) LoadTemplate(name string) (*template.Template, error) {
	t, ok := s.by[name]
	if !ok {
		return nil, errors.New("template: file not found")
	}
	return t, nil
}

func newStubTemplates() *stubTemplates {
	return &stubTemplates{by: map[string]*template.Template{}}
}

func newStub() *stubProvider {
	return &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "basic", Filename: "basic.yaml", Name: "Basic Form"},
			{Stem: "recepten", Filename: "recepten.yaml", Name: "Recepten", EnableCollection: true, GuidField: "guid"},
			{Stem: "leeg", Filename: "leeg.yaml", Name: "Leeg", EnableCollection: true, GuidField: "guid"},
		},
		forms: map[string][]dataprovider.FormSummary{
			"recepten.yaml": {
				{Template: "recepten.yaml", Filename: "brood.meta.json", ID: "g-1234", Title: "Brood"},
				{Template: "recepten.yaml", Filename: "pasta.meta.json", ID: "g-5678", Title: "Pasta"},
			},
			// leeg has no forms
		},
	}
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, rec.Body.String())
	}
	return out
}

func TestListCollections_OmitsNonCollectionTemplates(t *testing.T) {
	h := NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	rows := decode[[]TemplateRow](t, rec)
	ids := map[string]string{}
	for _, r := range rows {
		ids[r.ID] = r.Href
	}
	if _, ok := ids["basic"]; ok {
		t.Errorf("basic should be omitted")
	}
	if got := ids["recepten"]; got != "/api/collections/recepten" {
		t.Errorf("recepten href = %q", got)
	}
	if _, ok := ids["leeg"]; !ok {
		t.Errorf("leeg missing")
	}
}

func TestListCollections_NameFromYamlOrStem(t *testing.T) {
	sp := newStub()
	sp.templates[1].Name = "" // recepten without yaml name
	h := NewHandler(sp, newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rows := decode[[]TemplateRow](t, do(t, h, http.MethodGet, "/api/collections"))
	for _, r := range rows {
		if r.ID == "recepten" && r.Name != "recepten" {
			t.Errorf("fallback name = %q, want stem", r.Name)
		}
	}
}

func TestCount_OK(t *testing.T) {
	h := NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten/count")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	cr := decode[CountResponse](t, rec)
	if cr.Template != "recepten" || cr.Total != 2 {
		t.Errorf("got %+v", cr)
	}
}

func TestCount_DisabledTemplateReturns403(t *testing.T) {
	h := NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/basic/count")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := decode[errorBody](t, rec)
	if body.Error != "collection-disabled" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestCount_UnknownTemplateReturns403(t *testing.T) {
	// Unknown == disabled (don't leak existence via 404).
	h := NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/ghost/count")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCount_InternalErrorOnListFailure(t *testing.T) {
	sp := newStub()
	sp.listCollectionErr = errFake("boom")
	h := NewHandler(sp, newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten/count")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

// ── B1: POST + GUID endpoint ─────────────────────────────────────────

func TestGUID_ReturnsFreshUUID(t *testing.T) {
	h := NewHandler(newStub(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/guid")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	g, _ := body["guid"].(string)
	if len(g) < 30 {
		t.Errorf("guid looks short: %q", g)
	}
}

func TestPOST_AutoGeneratesGUID(t *testing.T) {
	sp := newStub()
	st := newStubStorage()
	wr := newStubWriter()
	wr.st = st
	tpl := newStubTemplates()
	tpl.by["recepten.yaml"] = &template.Template{
		Name: "Recepten", Filename: "recepten.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "guid", Type: "guid"}, {Key: "naam", Type: "text"}},
	}
	h := NewHandler(sp, st, wr, tpl, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/collections/recepten",
		strings.NewReader(`{"data":{"naam":"Pasta"}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if len(wr.saves) != 1 {
		t.Fatalf("saves = %d", len(wr.saves))
	}
	envelope := wr.saves[0].Envelope
	data, _ := envelope["data"].(map[string]any)
	guid, _ := data["guid"].(string)
	if len(guid) < 30 {
		t.Errorf("auto-minted guid looks short: %q", guid)
	}
}

func TestPOST_RejectsExistingByDefault(t *testing.T) {
	sp := newStub()
	sp.forms["recepten.yaml"] = []dataprovider.FormSummary{
		{Template: "recepten.yaml", Filename: "brood.meta.json", ID: "g-abc", Title: "Brood"},
	}
	tpl := newStubTemplates()
	tpl.by["recepten.yaml"] = &template.Template{
		Name: "Recepten", Filename: "recepten.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "guid", Type: "guid"}},
	}
	h := NewHandler(sp, newStubStorage(), newStubWriter(), tpl, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/collections/recepten",
		strings.NewReader(`{"data":{"guid":"g-abc"}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestPOST_UpsertOverwrites(t *testing.T) {
	sp := newStub()
	sp.forms["recepten.yaml"] = []dataprovider.FormSummary{
		{Template: "recepten.yaml", Filename: "brood.meta.json", ID: "g-abc", Title: "Brood"},
	}
	wr := newStubWriter()
	tpl := newStubTemplates()
	tpl.by["recepten.yaml"] = &template.Template{
		Name: "Recepten", Filename: "recepten.yaml", EnableCollection: true,
		Fields: []template.Field{{Key: "guid", Type: "guid"}, {Key: "naam", Type: "text"}},
	}
	h := NewHandler(sp, newStubStorage(), wr, tpl, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/collections/recepten?upsert=true",
		strings.NewReader(`{"data":{"guid":"g-abc","naam":"Brood 2"}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if len(wr.saves) != 1 {
		t.Fatalf("saves = %d", len(wr.saves))
	}
	if wr.saves[0].Datafile != "brood.meta.json" {
		t.Errorf("filename = %q, want brood.meta.json (existing)", wr.saves[0].Datafile)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Brood", "brood"},
		{"Brood Met Zaden", "brood-met-zaden"},
		{"Café Au Lait", "cafe-au-lait"},
		{"  multi   space  ", "multi-space"},
		{"!!!", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── A2: paged list ───────────────────────────────────────────────────

func newStubWithTags() *stubProvider {
	sp := newStub()
	sp.forms["recepten.yaml"] = []dataprovider.FormSummary{
		{Template: "recepten.yaml", Filename: "brood.meta.json", ID: "g-1234", Title: "Brood", Tags: []string{"bakery", "wb"}},
		{Template: "recepten.yaml", Filename: "pasta.meta.json", ID: "g-5678", Title: "Pasta", Tags: []string{"italian", "wb"}},
		{Template: "recepten.yaml", Filename: "pizza.meta.json", ID: "g-9999", Title: "Pizza", Tags: []string{"italian"}},
	}
	sp.rev = 42
	return sp
}

func TestList_Default(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("ETag"); got == "" {
		t.Errorf("missing ETag header")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q", got)
	}
	page := decode[dataprovider.CollectionPage](t, rec)
	if !page.Enabled || page.Total != 3 || page.Limit != 100 || page.Offset != 0 {
		t.Errorf("page = %+v", page)
	}
	if len(page.Items) != 3 {
		t.Errorf("items len = %d", len(page.Items))
	}
}

func TestList_Pagination(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten?limit=1&offset=1")
	page := decode[dataprovider.CollectionPage](t, rec)
	if page.Total != 3 || page.Limit != 1 || page.Offset != 1 || len(page.Items) != 1 {
		t.Errorf("page = %+v", page)
	}
}

func TestList_QFilter(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten?q=BROOD")
	page := decode[dataprovider.CollectionPage](t, rec)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Errorf("expected 1 row, got %+v", page)
	}
}

func TestList_TagsAND(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten?tags=italian,wb")
	page := decode[dataprovider.CollectionPage](t, rec)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "g-5678" {
		t.Errorf("page = %+v", page)
	}
}

func TestList_IfNoneMatchReturns304(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	first := do(t, h, http.MethodGet, "/api/collections/recepten")
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("no ETag on first response")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/collections/recepten", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("304 body should be empty, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != etag {
		t.Errorf("304 ETag = %q, want %q", got, etag)
	}
}

func TestList_StaleIfNoneMatchReturns200(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/collections/recepten", nil)
	req.Header.Set("If-None-Match", `W/"stale"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestList_DisabledTemplate(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/basic")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestList_UnknownTemplate(t *testing.T) {
	h := NewHandler(newStubWithTags(), newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/ghost")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestList_RevErrorReturns500(t *testing.T) {
	sp := newStubWithTags()
	sp.collectionRevErr = errFake("rev boom")
	h := NewHandler(sp, newStubStorage(), newStubWriter(), newStubTemplates(), nil, nil, nil)
	rec := do(t, h, http.MethodGet, "/api/collections/recepten")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
