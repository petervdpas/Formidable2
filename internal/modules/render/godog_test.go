package render

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initRenderScenario,
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

type renderWorld struct {
	tmp        string
	tpl        *template.Template
	form       *storage.Form
	imageURL   ImageURLFunc
	m          *Manager
	markdown   string
	html       string
	htmlErr    error
	parsedFM   map[string]any
	parsedBody string
}

func initRenderScenario(ctx *godog.ScenarioContext) {
	w := &renderWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "render-godog-")
		if err != nil {
			return ctx, err
		}
		*w = renderWorld{
			tmp:  dir,
			tpl:  &template.Template{},
			form: &storage.Form{Data: map[string]any{}},
		}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.tmp != "" {
			_ = os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// Build a Manager backed by fakes — simpler than spinning up the
	// full system+sfr+template+storage stack and gives us fine control
	// over the (template, datafile) lookup result the scenario needs.
	rebuild := func() {
		w.m = NewManager(
			&fakeTemplateLoader{tpl: w.tpl},
			&fakeFormStore{form: w.form},
			w.imageURL,
			nil, // formidable link URL
			nil, // log
		)
	}

	// ── Background ────────────────────────────────────────────────────

	ctx.Step(`^a fresh render Manager with no image URL strategy$`, func() error {
		w.imageURL = nil
		rebuild()
		return nil
	})

	ctx.Step(`^an image URL strategy that returns "([^"]*)"$`, func(pat string) error {
		// pat is a literal string with `{template}` and `{name}` slots —
		// keeps the feature file readable without escaping.
		w.imageURL = func(tplName, name string) string {
			out := strings.ReplaceAll(pat, "{template}", tplName)
			out = strings.ReplaceAll(out, "{name}", name)
			return out
		}
		rebuild()
		return nil
	})

	// ── Template + form setup steps ───────────────────────────────────

	ctx.Step(`^a template with markdown "([^"]*)" and field "([^"]*)" of type "([^"]*)"$`,
		func(md, key, typ string) error {
			w.tpl = &template.Template{
				MarkdownTemplate: md,
				Fields:           []template.Field{{Key: key, Type: typ}},
			}
			rebuild()
			return nil
		})

	ctx.Step(`^a template with markdown "([^"]*)"$`, func(md string) error {
		w.tpl = &template.Template{MarkdownTemplate: md}
		rebuild()
		return nil
	})

	ctx.Step(`^a template with markdown:$`, func(body *godog.DocString) error {
		w.tpl = &template.Template{MarkdownTemplate: body.Content}
		rebuild()
		return nil
	})

	ctx.Step(`^the dropdown field "([^"]*)" has options "([^"]*)"$`,
		func(key, csv string) error {
			opts := []any{}
			for _, pair := range strings.Split(csv, ",") {
				kv := strings.SplitN(pair, ":", 2)
				if len(kv) != 2 {
					return fmt.Errorf("bad option pair %q", pair)
				}
				opts = append(opts, map[string]any{"value": kv[0], "label": kv[1]})
			}
			w.tpl.Fields = append(w.tpl.Fields, template.Field{
				Key: key, Type: "dropdown", Options: opts,
			})
			rebuild()
			return nil
		})

	ctx.Step(`^a loop "([^"]*)" with field "([^"]*)" of type "([^"]*)"$`,
		func(loopKey, innerKey, innerType string) error {
			w.tpl.Fields = []template.Field{
				{Key: loopKey, Type: "loopstart"},
				{Key: innerKey, Type: innerType},
				{Key: loopKey, Type: "loopstop"},
			}
			rebuild()
			return nil
		})

	ctx.Step(`^the form has values:$`, func(table *godog.Table) error {
		for _, r := range table.Rows[1:] {
			w.form.Data[r.Cells[0].Value] = r.Cells[1].Value
		}
		return nil
	})

	ctx.Step(`^the form loop "([^"]*)" has entries:$`, func(key string, table *godog.Table) error {
		// First row is header (column names = inner field keys).
		headers := []string{}
		for _, c := range table.Rows[0].Cells {
			headers = append(headers, c.Value)
		}
		entries := []any{}
		for _, r := range table.Rows[1:] {
			entry := map[string]any{}
			for i, c := range r.Cells {
				entry[headers[i]] = c.Value
			}
			entries = append(entries, entry)
		}
		w.form.Data[key] = entries
		return nil
	})

	// ── Whens ─────────────────────────────────────────────────────────

	ctx.Step(`^I render markdown for a template with no markdown_template$`, func() error {
		// w.tpl is already empty (no MarkdownTemplate); use it.
		md, err := RenderMarkdown(w.form.Data, w.tpl, &Options{})
		if err != nil {
			return err
		}
		w.markdown = md
		return nil
	})

	ctx.Step(`^I render markdown$`, func() error {
		md, err := w.m.RenderMarkdown("tpl.yaml", "df")
		if err != nil {
			return err
		}
		w.markdown = md
		return nil
	})

	ctx.Step(`^I render the form for template "([^"]*)" and datafile "([^"]*)"$`,
		func(tplName, df string) error {
			res, err := w.m.RenderForm(tplName, df)
			if err != nil {
				return err
			}
			w.markdown = res.Markdown
			w.html = res.HTML
			return nil
		})

	ctx.Step(`^I render html from "([^"]*)"$`, func(src string) error {
		decoded := strings.ReplaceAll(src, `\n`, "\n")
		out, err := RenderHTML(decoded)
		w.html = out
		w.htmlErr = err
		return nil
	})

	ctx.Step(`^I render html from a fenced code block containing "([^"]*)"$`, func(line string) error {
		src := "```\n" + line + "\n```"
		out, err := RenderHTML(src)
		w.html = out
		w.htmlErr = err
		return nil
	})

	ctx.Step(`^I render html from a fenced go code block "([^"]*)"$`, func(line string) error {
		src := "```go\n" + line + "\n```"
		out, err := RenderHTML(src)
		w.html = out
		w.htmlErr = err
		return nil
	})

	ctx.Step(`^I render html from a 2-row markdown table$`, func() error {
		src := "| a | b |\n| - | - |\n| 1 | 2 |\n"
		out, err := RenderHTML(src)
		w.html = out
		w.htmlErr = err
		return nil
	})

	ctx.Step(`^I parse frontmatter from "([^"]*)"$`, func(src string) error {
		decoded := strings.ReplaceAll(src, `\n`, "\n")
		fm, body, err := ParseFrontmatter(decoded)
		if err != nil {
			return err
		}
		w.parsedFM = fm
		w.parsedBody = body
		return nil
	})

	// ── Thens ─────────────────────────────────────────────────────────

	ctx.Step(`^the markdown is "([^"]*)"$`, func(want string) error {
		if w.markdown != want {
			return fmt.Errorf("markdown = %q, want %q", w.markdown, want)
		}
		return nil
	})

	ctx.Step(`^the markdown contains "([^"]*)"$`, func(want string) error {
		if !strings.Contains(w.markdown, want) {
			return fmt.Errorf("markdown missing %q; got: %q", want, w.markdown)
		}
		return nil
	})

	ctx.Step(`^the html contains "([^"]*)"$`, func(want string) error {
		if !strings.Contains(w.html, want) {
			return fmt.Errorf("html missing %q; got: %q", want, w.html)
		}
		return nil
	})

	ctx.Step(`^the html does not contain "([^"]*)"$`, func(unwanted string) error {
		if strings.Contains(w.html, unwanted) {
			return fmt.Errorf("html should not contain %q; got: %q", unwanted, w.html)
		}
		return nil
	})

	ctx.Step(`^the frontmatter title is "([^"]*)"$`, func(want string) error {
		if w.parsedFM == nil {
			return fmt.Errorf("no frontmatter parsed")
		}
		if got := w.parsedFM["title"]; got != want {
			return fmt.Errorf("title = %v, want %q", got, want)
		}
		return nil
	})

	ctx.Step(`^the frontmatter count is (\d+)$`, func(want int) error {
		if w.parsedFM == nil {
			return fmt.Errorf("no frontmatter parsed")
		}
		got, ok := w.parsedFM["count"].(int)
		if !ok {
			return fmt.Errorf("count not an int: %T %v", w.parsedFM["count"], w.parsedFM["count"])
		}
		if got != want {
			return fmt.Errorf("count = %d, want %d", got, want)
		}
		return nil
	})

	ctx.Step(`^the frontmatter body is "([^"]*)"$`, func(want string) error {
		decoded := strings.ReplaceAll(want, `\n`, "\n")
		if w.parsedBody != decoded {
			return fmt.Errorf("body = %q, want %q", w.parsedBody, decoded)
		}
		return nil
	})
}
