package integrity

import (
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ─── stubs ────────────────────────────────────────────────────────────

type stubTemplates struct {
	ts map[string]*template.Template
}

func (s *stubTemplates) LoadTemplate(name string) (*template.Template, error) {
	t, ok := s.ts[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return t, nil
}

type stubStorage struct {
	forms     map[string]map[string]*storage.Form
	listErr   error
	listCalls int
}

func (s *stubStorage) ListForms(tpl string) ([]string, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := []string{}
	for fn := range s.forms[tpl] {
		out = append(out, fn)
	}
	sort.Strings(out)
	return out, nil
}

func (s *stubStorage) LoadForm(tpl, fn string) *storage.Form {
	return s.forms[tpl][fn]
}

// ─── fixtures ─────────────────────────────────────────────────────────

func newM(t *testing.T, tpl *template.Template, forms map[string]*storage.Form) *Manager {
	t.Helper()
	st := &stubTemplates{ts: map[string]*template.Template{tpl.Filename: tpl}}
	so := &stubStorage{forms: map[string]map[string]*storage.Form{tpl.Filename: forms}}
	return NewManager(st, so)
}

func tplBasic() *template.Template {
	return &template.Template{
		Name: "Basic", Filename: "basic.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "due", Type: "date"},
			{Key: "active", Type: "boolean"},
			{Key: "count", Type: "number"},
			{Key: "tags", Type: "tags"},
		},
	}
}

func tplWithGuid() *template.Template {
	return &template.Template{
		Name: "WithGuid", Filename: "g.yaml",
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "title", Type: "text"},
		},
	}
}

func tplWithLoop() *template.Template {
	return &template.Template{
		Name: "Looped", Filename: "loop.yaml",
		Fields: []template.Field{
			{Key: "title", Type: "text"},
			{Key: "items", Type: "loopstart"},
			{Key: "name", Type: "text"},
			{Key: "qty", Type: "number"},
			{Key: "items", Type: "loopstop"},
		},
	}
}

func tplWithFlag() *template.Template {
	return &template.Template{
		Name: "Flagged", Filename: "f.yaml",
		Facets: []template.Facet{{
			Key:  "flag",
			Icon: "fa-flag",
			Options: []template.FacetOption{
				{Label: "FLASH", Color: "red"},
				{Label: "WARN", Color: "orange"},
			},
		}},
		Fields: []template.Field{
			{Key: "title", Type: "text"},
		},
	}
}

func cleanForm() *storage.Form {
	return &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
		},
		Data: map[string]any{
			"title":  "hello",
			"due":    "2026-06-01",
			"active": true,
			"count":  float64(7),
			"tags":   []any{"alpha"},
		},
	}
}

// ─── helpers ──────────────────────────────────────────────────────────

func findIssue(t *testing.T, r Report, fn string, kind IssueKind, path string) Issue {
	t.Helper()
	for _, fr := range r.Forms {
		if fr.Filename != fn {
			continue
		}
		for _, iss := range fr.Issues {
			if iss.Kind == kind && (path == "" || iss.Path == path) {
				return iss
			}
		}
	}
	t.Fatalf("no %s issue at %q on %s; got %+v", kind, path, fn, r.Forms)
	return Issue{}
}

func mustZeroIssues(t *testing.T, r Report) {
	t.Helper()
	if r.IssueCount != 0 {
		t.Fatalf("expected 0 issues, got %d: %+v", r.IssueCount, r.Forms)
	}
}

// ─── tests ────────────────────────────────────────────────────────────

func TestAnalyze_NoIssues_OnCleanForm(t *testing.T) {
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})
	r, err := m.AnalyzeTemplate("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if r.FormCount != 1 {
		t.Errorf("want FormCount=1, got %d", r.FormCount)
	}
	mustZeroIssues(t, r)
}

func TestAnalyze_NoForms_ReturnsEmptyReport(t *testing.T) {
	m := newM(t, tplBasic(), map[string]*storage.Form{})
	r, err := m.AnalyzeTemplate("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if r.FormCount != 0 || r.IssueCount != 0 {
		t.Errorf("want empty report, got %+v", r)
	}
	if r.Template != "basic.yaml" {
		t.Errorf("want Template=basic.yaml, got %q", r.Template)
	}
}

func TestAnalyze_DetectsMissingField(t *testing.T) {
	f := cleanForm()
	delete(f.Data, "count")
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueMissingField, "count")
}

func TestAnalyze_DetectsExtraField(t *testing.T) {
	f := cleanForm()
	f.Data["zombie"] = "ghost"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueExtraField, "zombie")
}

func TestAnalyze_DetectsTypeMismatch_BooleanAsString(t *testing.T) {
	f := cleanForm()
	f.Data["active"] = "yes"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "active")
}

func TestAnalyze_DetectsTypeMismatch_NumberAsString(t *testing.T) {
	f := cleanForm()
	f.Data["count"] = "seven"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "count")
}

func TestAnalyze_AcceptsNumericIntAndFloat(t *testing.T) {
	for _, v := range []any{1, int64(2), float64(3), float32(4)} {
		f := cleanForm()
		f.Data["count"] = v
		m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
		r, _ := m.AnalyzeTemplate("basic.yaml")
		mustZeroIssues(t, r)
	}
}

func TestAnalyze_DetectsTypeMismatch_TagsAsString(t *testing.T) {
	f := cleanForm()
	f.Data["tags"] = "alpha,beta"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "tags")
}

func TestAnalyze_DetectsBadDateFormat(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = "21/07/2025"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueBadDateFormat, "due")
}

func TestAnalyze_AcceptsEmptyDate(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = ""
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_DateAsNumber_IsTypeMismatchNotBadDate(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = float64(20260601)
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "due")
}

func TestAnalyze_DetectsBadMetaCreated(t *testing.T) {
	f := cleanForm()
	f.Meta.Created.At = "yesterday"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueMetaBadFormat, "meta.created")
}

func TestAnalyze_DetectsBadMetaUpdated(t *testing.T) {
	f := cleanForm()
	f.Meta.Updated.At = "tomorrow"
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	findIssue(t, r, "a.meta.json", IssueMetaBadFormat, "meta.updated")
}

func TestAnalyze_AcceptsEmptyCreatedUpdated(t *testing.T) {
	f := cleanForm()
	f.Meta.Created = storage.AuditEntry{}
	f.Meta.Updated = storage.AuditEntry{}
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_DetectsMissingMetaId_WhenGuidFieldDeclared(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{"title": "x", "id": "abc-123"},
	}
	m := newM(t, tplWithGuid(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("g.yaml")
	findIssue(t, r, "a.meta.json", IssueMetaMissing, "meta.id")
}

func TestAnalyze_AcceptsMissingMetaId_WithoutGuidField(t *testing.T) {
	f := cleanForm()
	f.Meta.ID = ""
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_DetectsUnknownFacetSelected(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"flag": {Set: true, Selected: "GHOST"},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	m := newM(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("f.yaml")
	findIssue(t, r, "a.meta.json", IssueMetaBadFormat, "meta.facets.flag.selected")
}

func TestAnalyze_DetectsUnknownFacetKey(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"phantom": {Set: true},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	m := newM(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("f.yaml")
	findIssue(t, r, "a.meta.json", IssueMetaBadFormat, "meta.facets.phantom")
}

func TestAnalyze_AcceptsKnownFacetSelected(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"flag": {Set: true, Selected: "FLASH"},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	m := newM(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("f.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_AcceptsFacetSetWithoutSelected(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"flag": {Set: true},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	m := newM(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("f.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_LoopHappy(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{
			"title": "hi",
			"items": []any{
				map[string]any{"name": "a", "qty": float64(1)},
				map[string]any{"name": "b", "qty": float64(2)},
			},
		},
	}
	m := newM(t, tplWithLoop(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("loop.yaml")
	mustZeroIssues(t, r)
}

func TestAnalyze_DetectsLoopValueNotArray(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{"title": "hi", "items": "nope"},
	}
	m := newM(t, tplWithLoop(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("loop.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "items")
}

func TestAnalyze_DetectsMissingFieldInLoopItem(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{
			"title": "hi",
			"items": []any{
				map[string]any{"name": "a"},
			},
		},
	}
	m := newM(t, tplWithLoop(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("loop.yaml")
	findIssue(t, r, "a.meta.json", IssueMissingField, "items[0].qty")
}

func TestAnalyze_DetectsExtraFieldInLoopItem(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{
			"title": "hi",
			"items": []any{
				map[string]any{"name": "a", "qty": float64(1), "ghost": "x"},
			},
		},
	}
	m := newM(t, tplWithLoop(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("loop.yaml")
	findIssue(t, r, "a.meta.json", IssueExtraField, "items[0].ghost")
}

func TestAnalyze_DetectsLoopItemNotMap(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{
			"title": "hi",
			"items": []any{"not-a-map"},
		},
	}
	m := newM(t, tplWithLoop(), map[string]*storage.Form{"a.meta.json": f})
	r, _ := m.AnalyzeTemplate("loop.yaml")
	findIssue(t, r, "a.meta.json", IssueTypeMismatch, "items[0]")
}

func TestAnalyze_UnknownTemplate_Errors(t *testing.T) {
	m := newM(t, tplBasic(), map[string]*storage.Form{})
	_, err := m.AnalyzeTemplate("ghost.yaml")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestAnalyze_UnreadableForm_IssueRecorded(t *testing.T) {
	// LoadForm returning nil mimics what storage.Manager does on
	// parse failure — see the rescan/index "malformed JSON" path.
	m := newM(t, tplBasic(), map[string]*storage.Form{"broken.meta.json": nil})
	r, err := m.AnalyzeTemplate("basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	iss := findIssue(t, r, "broken.meta.json", IssueUnreadable, "")
	if iss.Kind != IssueUnreadable {
		t.Errorf("want IssueUnreadable, got %s", iss.Kind)
	}
	// Unreadable is a terminal verdict — no other issues should be stacked.
	for _, fr := range r.Forms {
		if fr.Filename == "broken.meta.json" && len(fr.Issues) != 1 {
			t.Errorf("want exactly 1 issue on broken form, got %d", len(fr.Issues))
		}
	}
}

func TestAnalyze_ScannedAtSet(t *testing.T) {
	m := newM(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})
	before := time.Now()
	r, _ := m.AnalyzeTemplate("basic.yaml")
	if r.ScannedAt.Before(before) {
		t.Errorf("ScannedAt %v should not predate test start %v", r.ScannedAt, before)
	}
}

func TestAnalyze_ListErrorPropagates(t *testing.T) {
	st := &stubTemplates{ts: map[string]*template.Template{"basic.yaml": tplBasic()}}
	so := &stubStorage{listErr: errors.New("io fail")}
	m := NewManager(st, so)
	_, err := m.AnalyzeTemplate("basic.yaml")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestAnalyze_OrderedFormsInReport(t *testing.T) {
	a := cleanForm()
	a.Data["zombie1"] = "x"
	b := cleanForm()
	b.Data["zombie2"] = "y"
	m := newM(t, tplBasic(), map[string]*storage.Form{
		"b.meta.json": b,
		"a.meta.json": a,
	})
	r, _ := m.AnalyzeTemplate("basic.yaml")
	if len(r.Forms) != 2 {
		t.Fatalf("want 2 affected forms, got %d", len(r.Forms))
	}
	if r.Forms[0].Filename != "a.meta.json" || r.Forms[1].Filename != "b.meta.json" {
		t.Errorf("forms should be sorted by filename; got %s,%s",
			r.Forms[0].Filename, r.Forms[1].Filename)
	}
}
