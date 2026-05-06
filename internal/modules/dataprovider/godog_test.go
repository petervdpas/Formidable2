package dataprovider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/index"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// world holds per-scenario state so each test case starts clean.
type world struct {
	idx *fakeIndex
	ren *fakeRenderer
	m   *Manager

	// Latest results — assertion steps inspect these.
	tplList   []TemplateSummary
	tpl       *TemplateSummary
	tplFound  bool
	formList  []FormSummary
	formFound bool

	resolved      *FormSummary
	resolvedFound bool

	rendered *RenderedPage

	collPage     *CollectionPage
	collItem     *CollectionItem
	collItemHit  bool
}

func initScenario(ctx *godog.ScenarioContext) {
	w := &world{}

	ctx.Before(func(c context.Context, _ *godog.Scenario) (context.Context, error) {
		w.idx = &fakeIndex{forms: map[string][]index.FormRow{}}
		w.ren = &fakeRenderer{markdown: "# default", html: "<h1>default</h1>"}
		w.m = NewManager(w.idx, w.ren)
		// reset captured results
		*w = world{idx: w.idx, ren: w.ren, m: w.m}
		return c, nil
	})

	// ── Background: seed templates + forms ────────────────────────

	ctx.Step(`^a fresh dataprovider with these templates:$`, func(table *godog.Table) error {
		for _, row := range tableRows(table) {
			w.idx.templates = append(w.idx.templates, index.TemplateRow{
				Filename:         row["filename"],
				Name:             row["name"],
				GuidField:        row["guid_field"],
				TagsField:        row["tags_field"],
				ItemField:        row["item_field"],
				EnableCollection: row["enable_collection"] == "true",
			})
		}
		return nil
	})

	ctx.Step(`^these forms under "([^"]*)":$`, func(tpl string, table *godog.Table) error {
		for _, row := range tableRows(table) {
			tags := splitCommaList(row["tags"])
			w.idx.forms[tpl] = append(w.idx.forms[tpl], index.FormRow{
				Template: tpl,
				Filename: row["filename"],
				ID:       row["id"],
				Title:    row["title"],
				Tags:     tags,
			})
		}
		return nil
	})

	// ── Renderer setup (only some scenarios use this) ─────────────

	ctx.Step(`^the renderer returns markdown:$`, func(doc *godog.DocString) error {
		w.ren.markdown = doc.Content
		w.ren.html = "<rendered/>"
		return nil
	})

	// ── Templates / forms ─────────────────────────────────────────

	ctx.Step(`^I list templates$`, func() error {
		out, err := w.m.ListTemplates(context.Background())
		w.tplList = out
		return err
	})

	ctx.Step(`^the template list has (\d+) templates$`, func(n int) error {
		if len(w.tplList) != n {
			return fmt.Errorf("got %d, want %d", len(w.tplList), n)
		}
		return nil
	})

	ctx.Step(`^template "([^"]*)" has stem "([^"]*)" and collection enabled$`,
		func(filename, stem string) error {
			for _, t := range w.tplList {
				if t.Filename == filename {
					if t.Stem != stem {
						return fmt.Errorf("stem = %q, want %q", t.Stem, stem)
					}
					if !t.EnableCollection {
						return fmt.Errorf("expected collection enabled")
					}
					return nil
				}
			}
			return fmt.Errorf("template %q not in list", filename)
		})

	ctx.Step(`^I get template "([^"]*)"$`, func(filename string) error {
		t, ok, err := w.m.GetTemplate(context.Background(), filename)
		w.tpl, w.tplFound = t, ok
		return err
	})

	ctx.Step(`^the template stem is "([^"]*)"$`, func(want string) error {
		if !w.tplFound {
			return fmt.Errorf("template not found")
		}
		if w.tpl.Stem != want {
			return fmt.Errorf("stem = %q, want %q", w.tpl.Stem, want)
		}
		return nil
	})

	ctx.Step(`^the template name is "([^"]*)"$`, func(want string) error {
		if w.tpl.Name != want {
			return fmt.Errorf("name = %q, want %q", w.tpl.Name, want)
		}
		return nil
	})

	ctx.Step(`^the template lookup misses$`, func() error {
		if w.tplFound {
			return fmt.Errorf("expected miss, got %+v", w.tpl)
		}
		return nil
	})

	ctx.Step(`^I list forms under "([^"]*)"$`, func(tpl string) error {
		out, err := w.m.ListForms(context.Background(), tpl, ListOpts{})
		w.formList = out
		return err
	})

	ctx.Step(`^the form list has (\d+) forms$`, func(n int) error {
		if len(w.formList) != n {
			return fmt.Errorf("got %d, want %d", len(w.formList), n)
		}
		return nil
	})

	ctx.Step(`^form "([^"]*)" has tags "([^"]*)"$`, func(filename, csv string) error {
		want := splitCommaList(csv)
		sort.Strings(want)
		for _, f := range w.formList {
			if f.Filename == filename {
				got := append([]string(nil), f.Tags...)
				sort.Strings(got)
				if !equalSorted(got, want) {
					return fmt.Errorf("got %v, want %v", got, want)
				}
				return nil
			}
		}
		return fmt.Errorf("form %q not in list", filename)
	})

	ctx.Step(`^I resolve id "([^"]*)" under "([^"]*)"$`, func(id, tpl string) error {
		f, ok, err := w.m.ResolveByID(context.Background(), tpl, id)
		w.resolved, w.resolvedFound = f, ok
		return err
	})

	ctx.Step(`^the resolution returns "([^"]*)"$`, func(filename string) error {
		if !w.resolvedFound {
			return fmt.Errorf("expected hit, got miss")
		}
		if w.resolved.Filename != filename {
			return fmt.Errorf("filename = %q, want %q", w.resolved.Filename, filename)
		}
		return nil
	})

	ctx.Step(`^the resolution misses$`, func() error {
		if w.resolvedFound {
			return fmt.Errorf("expected miss, got %+v", w.resolved)
		}
		return nil
	})

	// ── Render ────────────────────────────────────────────────────

	ctx.Step(`^I render "([^"]*)" under "([^"]*)"$`, func(filename, tpl string) error {
		out, err := w.m.RenderForm(context.Background(), tpl, filename)
		w.rendered = out
		return err
	})

	ctx.Step(`^the rendered title is "([^"]*)"$`, func(want string) error {
		if w.rendered == nil {
			return fmt.Errorf("nothing rendered")
		}
		if w.rendered.Title != want {
			return fmt.Errorf("title = %q, want %q", w.rendered.Title, want)
		}
		return nil
	})

	// ── Collections ───────────────────────────────────────────────

	ctx.Step(`^I list the collection for "([^"]*)"$`, func(tpl string) error {
		out, err := w.m.ListCollection(context.Background(), tpl, CollectionListOpts{})
		w.collPage = out
		return err
	})

	ctx.Step(`^I list the collection for "([^"]*)" with q "([^"]*)"$`, func(tpl, q string) error {
		out, err := w.m.ListCollection(context.Background(), tpl, CollectionListOpts{Q: q})
		w.collPage = out
		return err
	})

	ctx.Step(`^I list the collection for "([^"]*)" with tags "([^"]*)"$`, func(tpl, csv string) error {
		out, err := w.m.ListCollection(context.Background(), tpl,
			CollectionListOpts{Tags: splitCommaList(csv)})
		w.collPage = out
		return err
	})

	ctx.Step(`^I list the collection for "([^"]*)" with limit (\d+) and offset (\d+)$`,
		func(tpl string, limit, offset int) error {
			out, err := w.m.ListCollection(context.Background(), tpl,
				CollectionListOpts{Limit: limit, Offset: offset})
			w.collPage = out
			return err
		})

	ctx.Step(`^the collection is disabled$`, func() error {
		if w.collPage == nil {
			return fmt.Errorf("nil collection page")
		}
		if w.collPage.Enabled {
			return fmt.Errorf("expected Enabled=false")
		}
		return nil
	})

	ctx.Step(`^the collection is enabled$`, func() error {
		if w.collPage == nil || !w.collPage.Enabled {
			return fmt.Errorf("expected enabled, got %+v", w.collPage)
		}
		return nil
	})

	ctx.Step(`^the collection total is (\d+)$`, func(n int) error {
		if w.collPage.Total != n {
			return fmt.Errorf("total = %d, want %d", w.collPage.Total, n)
		}
		return nil
	})

	ctx.Step(`^the collection page has (\d+) items$`, func(n int) error {
		if len(w.collPage.Items) != n {
			return fmt.Errorf("items = %d, want %d", len(w.collPage.Items), n)
		}
		return nil
	})

	ctx.Step(`^item "([^"]*)" has self-href "([^"]*)"$`, func(id, want string) error {
		for _, it := range w.collPage.Items {
			if it.ID == id {
				if it.HrefSelf != want {
					return fmt.Errorf("hrefSelf = %q, want %q", it.HrefSelf, want)
				}
				return nil
			}
		}
		return fmt.Errorf("no item with id %q", id)
	})

	ctx.Step(`^item "([^"]*)" has html-href "([^"]*)"$`, func(id, want string) error {
		for _, it := range w.collPage.Items {
			if it.ID == id {
				if it.HrefHTML != want {
					return fmt.Errorf("hrefHTML = %q, want %q", it.HrefHTML, want)
				}
				return nil
			}
		}
		return fmt.Errorf("no item with id %q", id)
	})

	ctx.Step(`^the collection contains items "([^"]*)"$`, func(csv string) error {
		want := splitCommaList(csv)
		sort.Strings(want)
		got := []string{}
		for _, it := range w.collPage.Items {
			got = append(got, it.Filename)
		}
		sort.Strings(got)
		if !equalSorted(got, want) {
			return fmt.Errorf("got %v, want %v", got, want)
		}
		return nil
	})

	ctx.Step(`^I resolve collection id "([^"]*)" under "([^"]*)"$`, func(id, tpl string) error {
		it, ok, err := w.m.ResolveCollectionByID(context.Background(), tpl, id)
		w.collItem, w.collItemHit = it, ok
		return err
	})

	ctx.Step(`^the collection item filename is "([^"]*)"$`, func(want string) error {
		if !w.collItemHit {
			return fmt.Errorf("expected hit")
		}
		if w.collItem.Filename != want {
			return fmt.Errorf("filename = %q, want %q", w.collItem.Filename, want)
		}
		return nil
	})

	ctx.Step(`^the collection item self-href is "([^"]*)"$`, func(want string) error {
		if w.collItem.HrefSelf != want {
			return fmt.Errorf("hrefSelf = %q, want %q", w.collItem.HrefSelf, want)
		}
		return nil
	})
}

// ── small helpers (godog only) ────────────────────────────────────────

// tableRows turns a godog.Table with a header row into []map[col]value
// so step bindings can read by column name without column-index math.
func tableRows(table *godog.Table) []map[string]string {
	if table == nil || len(table.Rows) < 2 {
		return nil
	}
	headers := []string{}
	for _, c := range table.Rows[0].Cells {
		headers = append(headers, c.Value)
	}
	out := make([]map[string]string, 0, len(table.Rows)-1)
	for _, r := range table.Rows[1:] {
		row := map[string]string{}
		for i, c := range r.Cells {
			if i < len(headers) {
				row[headers[i]] = c.Value
			}
		}
		out = append(out, row)
	}
	return out
}

func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
