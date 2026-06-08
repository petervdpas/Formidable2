package dataprovider

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/render"
)

// fakeIndex is a small in-memory stand-in for *index.Manager. Tests
// hand it canned rows; the dataprovider exercises its API like normal.
type fakeIndex struct {
	templates []index.TemplateRow
	forms     map[string][]index.FormRow // template → rows
	rev       int64
	listErr   error
}

func (f *fakeIndex) ListTemplates() ([]index.TemplateRow, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.templates, nil
}

func (f *fakeIndex) ListForms(t string, opts index.QueryOpts) ([]index.FormRow, error) {
	rows := f.forms[t]
	// Apply the AND-tag filter the way the real index does so tests
	// covering filter semantics still pass through this fake.
	if len(opts.Tags) > 0 {
		want := map[string]struct{}{}
		for _, x := range opts.Tags {
			want[x] = struct{}{}
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
	if opts.Offset > 0 && opts.Offset < len(rows) {
		rows = rows[opts.Offset:]
	} else if opts.Offset >= len(rows) {
		rows = nil
	}
	if opts.Limit > 0 && len(rows) > opts.Limit {
		rows = rows[:opts.Limit]
	}
	return rows, nil
}

func (f *fakeIndex) GetForm(t, file string) (*index.FormRow, bool, error) {
	for i := range f.forms[t] {
		if f.forms[t][i].Filename == file {
			r := f.forms[t][i]
			return &r, true, nil
		}
	}
	return nil, false, nil
}

func (f *fakeIndex) ListByTags(tags []string) ([]index.FormRow, error) {
	want := map[string]struct{}{}
	for _, x := range tags {
		want[x] = struct{}{}
	}
	var out []index.FormRow
	for _, rows := range f.forms {
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
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func (f *fakeIndex) FormsWithValueOp(template, fieldKey, op, value string) ([]string, error) {
	var out []string
	for _, r := range f.forms[template] {
		for _, v := range r.Values {
			if v.Col != nil || v.FieldKey != fieldKey {
				continue
			}
			match := false
			switch op {
			case "eq":
				match = v.Text == value
			case "ne":
				match = v.Text != value
			case "gt", "ge", "lt", "le":
				n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err != nil {
					return nil, err
				}
				if v.Num == nil {
					continue
				}
				switch op {
				case "gt":
					match = *v.Num > n
				case "ge":
					match = *v.Num >= n
				case "lt":
					match = *v.Num < n
				case "le":
					match = *v.Num <= n
				}
			default:
				return nil, errors.New("fakeIndex: invalid op " + op)
			}
			if match {
				out = append(out, r.Filename)
				break
			}
		}
	}
	return out, nil
}

func (f *fakeIndex) Rev() (int64, error) { return f.rev, nil }

// fakeRenderer returns canned markdown+html so tests don't need the
// real raymond+goldmark pipeline.
type fakeRenderer struct {
	markdown string
	html     string
	err      error
}

func (f *fakeRenderer) RenderForm(_, _ string) (*render.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &render.Result{Markdown: f.markdown, HTML: f.html}, nil
}

// ── helpers ───────────────────────────────────────────────────────

func newManagerWithFakes(idx *fakeIndex, ren *fakeRenderer) *Manager {
	if ren == nil {
		ren = &fakeRenderer{markdown: "# Hi", html: "<h1>Hi</h1>"}
	}
	return NewManager(idx, ren, nil)
}

func sortedNames(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// ── ListTemplates ────────────────────────────────────────────────

func TestListTemplates_ProjectsRowsToSummaries(t *testing.T) {
	idx := &fakeIndex{
		templates: []index.TemplateRow{
			{Filename: "basic.yaml", Name: "Basic", HasMarkdownTemplate: true},
			{Filename: "looper.yaml", Name: "Looper",
				GuidField: "id", TagsField: "labels", EnableCollection: true},
		},
	}
	m := newManagerWithFakes(idx, nil)

	out, err := m.ListTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d, want 2", len(out))
	}
	if out[0].Filename != "basic.yaml" || out[0].Stem != "basic" {
		t.Errorf("first summary wrong: %+v", out[0])
	}
	if !out[0].HasMarkdownTemplate {
		t.Errorf("HasMarkdownTemplate not propagated: %+v", out[0])
	}
	if out[1].Stem != "looper" || !out[1].EnableCollection {
		t.Errorf("looper summary wrong: %+v", out[1])
	}
}

func TestListTemplates_PropagatesError(t *testing.T) {
	idx := &fakeIndex{listErr: errors.New("boom")}
	m := newManagerWithFakes(idx, nil)
	if _, err := m.ListTemplates(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

// ── GetTemplate ──────────────────────────────────────────────────

func TestGetTemplate_FoundAndNotFound(t *testing.T) {
	idx := &fakeIndex{
		templates: []index.TemplateRow{{Filename: "basic.yaml", Name: "Basic"}},
	}
	m := newManagerWithFakes(idx, nil)

	got, ok, err := m.GetTemplate(context.Background(), "basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Stem != "basic" || got.Name != "Basic" {
		t.Errorf("found wrong: %+v ok=%v", got, ok)
	}

	_, ok, err = m.GetTemplate(context.Background(), "ghost.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("expected not-found, got ok")
	}
}

// ── ListForms ────────────────────────────────────────────────────

func TestListForms_ProjectsRows(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "one.meta.json",
					ID: "g1", Title: "One", Tags: []string{"a", "b"}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	out, err := m.ListForms(context.Background(), "basic.yaml", ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "g1" || out[0].Title != "One" {
		t.Fatalf("wrong: %+v", out)
	}
	if !equalSorted(out[0].Tags, []string{"a", "b"}) {
		t.Errorf("tags = %v", out[0].Tags)
	}
}

func TestListForms_TagFilterAND(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Filename: "a.meta.json", Tags: []string{"alpha", "beta"}},
				{Filename: "b.meta.json", Tags: []string{"alpha"}},
				{Filename: "c.meta.json", Tags: []string{"beta"}},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	out, err := m.ListForms(context.Background(), "basic.yaml",
		ListOpts{Tags: []string{"alpha", "beta"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Filename != "a.meta.json" {
		t.Errorf("got %+v, want only a.meta.json", out)
	}
}

// ── GetFormSummary ───────────────────────────────────────────────

func TestGetFormSummary_FoundAndNotFound(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "x.meta.json", ID: "g", Title: "X"},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)

	got, ok, err := m.GetFormSummary(context.Background(), "basic.yaml", "x.meta.json")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.ID != "g" || got.Title != "X" {
		t.Errorf("payload wrong: %+v", got)
	}

	_, ok, _ = m.GetFormSummary(context.Background(), "basic.yaml", "missing.meta.json")
	if ok {
		t.Errorf("expected not-found")
	}
}

// ── ListByTags ───────────────────────────────────────────────────

func TestListByTags_AcrossTemplates(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"a.yaml": {{Template: "a.yaml", Filename: "x.meta.json", Tags: []string{"alpha"}}},
			"b.yaml": {{Template: "b.yaml", Filename: "y.meta.json", Tags: []string{"alpha"}}},
		},
	}
	m := newManagerWithFakes(idx, nil)
	rows, err := m.ListByTags(context.Background(), []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Template+"/"+r.Filename)
	}
	want := []string{"a.yaml/x.meta.json", "b.yaml/y.meta.json"}
	if !equalSorted(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ── ResolveByID ──────────────────────────────────────────────────

func TestResolveByID_FindsByGuid(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "a.meta.json", ID: "g1", Title: "A"},
				{Template: "basic.yaml", Filename: "b.meta.json", ID: "g2", Title: "B"},
			},
		},
	}
	m := newManagerWithFakes(idx, nil)
	got, ok, err := m.ResolveByID(context.Background(), "basic.yaml", "g2")
	if err != nil || !ok || got.Filename != "b.meta.json" {
		t.Errorf("got=%+v ok=%v err=%v", got, ok, err)
	}

	_, ok, _ = m.ResolveByID(context.Background(), "basic.yaml", "no-such")
	if ok {
		t.Errorf("expected not-found")
	}
}

// ── RenderForm ───────────────────────────────────────────────────

func TestRenderForm_ReturnsRenderedPageWithFrontmatterTitle(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "x.meta.json", Title: "filename-title"},
			},
		},
	}
	ren := &fakeRenderer{
		markdown: "---\ntitle: From Frontmatter\n---\n# body\n",
		html:     "<h1>body</h1>",
	}
	m := newManagerWithFakes(idx, ren)

	got, err := m.RenderForm(context.Background(), "basic.yaml", "x.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "From Frontmatter" {
		t.Errorf("title = %q, want From Frontmatter", got.Title)
	}
	if !strings.Contains(got.HTML, "<h1>") {
		t.Errorf("html = %q", got.HTML)
	}
	if got.Markdown != ren.markdown {
		t.Errorf("markdown not passed through")
	}
}

func TestRenderForm_FallsBackToFormTitleThenFilename(t *testing.T) {
	// No frontmatter title → use the form summary's title; if that's
	// also empty → use the datafile name.
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "x.meta.json", Title: "From Index"},
			},
		},
	}
	ren := &fakeRenderer{markdown: "# body", html: "<h1>body</h1>"}
	m := newManagerWithFakes(idx, ren)

	got, _ := m.RenderForm(context.Background(), "basic.yaml", "x.meta.json")
	if got.Title != "From Index" {
		t.Errorf("title = %q, want From Index", got.Title)
	}

	idx.forms["basic.yaml"][0].Title = ""
	got, _ = m.RenderForm(context.Background(), "basic.yaml", "x.meta.json")
	if got.Title != "x.meta.json" {
		t.Errorf("title = %q, want filename fallback", got.Title)
	}
}

func TestRenderForm_PropagatesRenderError(t *testing.T) {
	idx := &fakeIndex{
		forms: map[string][]index.FormRow{
			"basic.yaml": {{Template: "basic.yaml", Filename: "x.meta.json"}},
		},
	}
	ren := &fakeRenderer{err: errors.New("boom")}
	m := newManagerWithFakes(idx, ren)

	if _, err := m.RenderForm(context.Background(), "basic.yaml", "x.meta.json"); err == nil {
		t.Errorf("expected render error to propagate")
	}
}

// ── Rev ──────────────────────────────────────────────────────────

func TestRev(t *testing.T) {
	idx := &fakeIndex{rev: 42}
	m := newManagerWithFakes(idx, nil)
	got, err := m.Rev(context.Background())
	if err != nil || got != 42 {
		t.Errorf("got %d err=%v, want 42", got, err)
	}
}

// helper local to this file
func equalSorted(a, b []string) bool {
	a = sortedNames(a)
	b = sortedNames(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
