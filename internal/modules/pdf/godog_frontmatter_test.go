package pdf

import (
	"context"
	"errors"
	"fmt"

	picoloom "github.com/alnah/picoloom/v2"
	"github.com/cucumber/godog"
)

type frontmatterWorld struct {
	source     string
	parsedFM   Frontmatter
	parsedBody string
	parseErr   error

	layers map[string]Frontmatter
	merged Frontmatter

	input picoloom.Input
}

func (w *frontmatterWorld) reset() {
	*w = frontmatterWorld{layers: map[string]Frontmatter{}}
}

func initFrontmatterScenario(ctx *godog.ScenarioContext) {
	w := &frontmatterWorld{layers: map[string]Frontmatter{}}

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.reset()
		return ctx, nil
	})

	ctx.Step(`^a fresh frontmatter test world$`, func() error {
		w.reset()
		return nil
	})

	// ---- Parse ----

	ctx.Step(`^I parse the markdown "([^"]*)"$`, func(md string) error {
		w.source = md
		w.parsedFM, w.parsedBody, w.parseErr = ParseFrontmatter(md)
		return nil
	})

	ctx.Step(`^I parse the markdown:$`, func(doc *godog.DocString) error {
		md := doc.Content
		w.source = md
		w.parsedFM, w.parsedBody, w.parseErr = ParseFrontmatter(md)
		return nil
	})

	ctx.Step(`^the parse returned no error$`, func() error {
		if w.parseErr != nil {
			return fmt.Errorf("parse err = %v, want nil", w.parseErr)
		}
		return nil
	})

	ctx.Step(`^the parse returned a malformed-frontmatter error$`, func() error {
		if !errors.Is(w.parseErr, ErrFrontmatterMalformed) {
			return fmt.Errorf("parse err = %v, want ErrFrontmatterMalformed", w.parseErr)
		}
		return nil
	})

	ctx.Step(`^the parsed body is empty$`, func() error {
		if w.parsedBody != "" {
			return fmt.Errorf("body = %q, want empty", w.parsedBody)
		}
		return nil
	})

	ctx.Step(`^the parsed body equals "([^"]*)"$`, func(want string) error {
		if w.parsedBody != want {
			return fmt.Errorf("body = %q, want %q", w.parsedBody, want)
		}
		return nil
	})

	ctx.Step(`^the parsed body equals the original input$`, func() error {
		if w.parsedBody != w.source {
			return fmt.Errorf("body = %q, want input verbatim %q", w.parsedBody, w.source)
		}
		return nil
	})

	ctx.Step(`^the parsed style is empty$`, func() error {
		if w.parsedFM.Style != "" {
			return fmt.Errorf("style = %q, want empty", w.parsedFM.Style)
		}
		return nil
	})

	ctx.Step(`^the parsed style is "([^"]*)"$`, func(want string) error {
		if w.parsedFM.Style != want {
			return fmt.Errorf("style = %q, want %q", w.parsedFM.Style, want)
		}
		return nil
	})

	// ---- Merge ----

	ctx.Step(`^a frontmatter layer "([^"]*)" with style "([^"]*)"$`, func(name, style string) error {
		fm := w.layers[name]
		fm.Style = style
		w.layers[name] = fm
		return nil
	})

	ctx.Step(`^a frontmatter layer "([^"]*)" with no opinions$`, func(name string) error {
		w.layers[name] = Frontmatter{}
		return nil
	})

	ctx.Step(`^a frontmatter layer "([^"]*)" with cover title "([^"]*)"$`, func(name, title string) error {
		fm := w.layers[name]
		if fm.Cover == nil {
			fm.Cover = &CoverFM{}
		}
		fm.Cover.Title = title
		w.layers[name] = fm
		return nil
	})

	ctx.Step(`^a frontmatter layer "([^"]*)" with cover author "([^"]*)"$`, func(name, author string) error {
		fm := w.layers[name]
		if fm.Cover == nil {
			fm.Cover = &CoverFM{}
		}
		fm.Cover.Author = author
		w.layers[name] = fm
		return nil
	})

	ctx.Step(`^a frontmatter layer "([^"]*)" with cover enabled (true|false)$`, func(name, raw string) error {
		fm := w.layers[name]
		if fm.Cover == nil {
			fm.Cover = &CoverFM{}
		}
		v := raw == "true"
		fm.Cover.Enabled = &v
		w.layers[name] = fm
		return nil
	})

	ctx.Step(`^I merge layers in order ([\w, ]+)$`, func(order string) error {
		names := splitCSV(order)
		layers := make([]Frontmatter, 0, len(names))
		for _, n := range names {
			fm, ok := w.layers[n]
			if !ok {
				return fmt.Errorf("merge ordering references missing layer %q", n)
			}
			layers = append(layers, fm)
		}
		w.merged = Merge(layers...)
		return nil
	})

	ctx.Step(`^the merged style is "([^"]*)"$`, func(want string) error {
		if w.merged.Style != want {
			return fmt.Errorf("merged style = %q, want %q", w.merged.Style, want)
		}
		return nil
	})

	ctx.Step(`^the merged cover title is "([^"]*)"$`, func(want string) error {
		if w.merged.Cover == nil || w.merged.Cover.Title != want {
			return fmt.Errorf("merged cover = %+v, want title %q", w.merged.Cover, want)
		}
		return nil
	})

	ctx.Step(`^the merged cover author is "([^"]*)"$`, func(want string) error {
		if w.merged.Cover == nil || w.merged.Cover.Author != want {
			return fmt.Errorf("merged cover = %+v, want author %q", w.merged.Cover, want)
		}
		return nil
	})

	ctx.Step(`^the merged cover enabled is (true|false)$`, func(raw string) error {
		if w.merged.Cover == nil || w.merged.Cover.Enabled == nil {
			return fmt.Errorf("merged cover Enabled = nil, want %s", raw)
		}
		want := raw == "true"
		if *w.merged.Cover.Enabled != want {
			return fmt.Errorf("merged cover Enabled = %v, want %v", *w.merged.Cover.Enabled, want)
		}
		return nil
	})

	// ---- BuildInput ----

	ctx.Step(`^an empty merged frontmatter$`, func() error {
		w.merged = Frontmatter{}
		return nil
	})

	ctx.Step(`^a merged frontmatter with cover enabled (true|false) and title "([^"]*)"$`, func(raw, title string) error {
		v := raw == "true"
		w.merged = Frontmatter{Cover: &CoverFM{Enabled: &v, Title: title}}
		return nil
	})

	ctx.Step(`^a merged frontmatter with cover title "([^"]*)" and no explicit enabled$`, func(title string) error {
		w.merged = Frontmatter{Cover: &CoverFM{Title: title}}
		return nil
	})

	ctx.Step(`^I build the picoloom\.Input with body "([^"]*)"$`, func(body string) error {
		w.input = BuildInput(w.merged, body)
		return nil
	})

	ctx.Step(`^the Input markdown equals "([^"]*)"$`, func(want string) error {
		if w.input.Markdown != want {
			return fmt.Errorf("Input.Markdown = %q, want %q", w.input.Markdown, want)
		}
		return nil
	})

	ctx.Step(`^the Input has no Cover block$`, func() error {
		if w.input.Cover != nil {
			return fmt.Errorf("Input.Cover = %+v, want nil", w.input.Cover)
		}
		return nil
	})

	ctx.Step(`^the Input has no TOC block$`, func() error {
		if w.input.TOC != nil {
			return fmt.Errorf("Input.TOC = %+v, want nil", w.input.TOC)
		}
		return nil
	})

	ctx.Step(`^the Input has no Watermark block$`, func() error {
		if w.input.Watermark != nil {
			return fmt.Errorf("Input.Watermark = %+v, want nil", w.input.Watermark)
		}
		return nil
	})

	ctx.Step(`^the Input has a Cover block with title "([^"]*)"$`, func(want string) error {
		if w.input.Cover == nil {
			return fmt.Errorf("Input.Cover = nil, want non-nil")
		}
		if w.input.Cover.Title != want {
			return fmt.Errorf("Input.Cover.Title = %q, want %q", w.input.Cover.Title, want)
		}
		return nil
	})
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		switch r {
		case ',':
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		case ' ':
			// skip
		default:
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
