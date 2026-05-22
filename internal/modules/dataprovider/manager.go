package dataprovider

import (
	"context"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/render"
)

// ── core read API ────────────────────────────────────────────────────

// ListTemplates returns every indexed template projected into the
// HTTP-friendly TemplateSummary shape. Sorted by filename ASC (the
// index already does that).
func (m *Manager) ListTemplates(_ context.Context) ([]TemplateSummary, error) {
	rows, err := m.idx.ListTemplates()
	if err != nil {
		return nil, err
	}
	out := make([]TemplateSummary, len(rows))
	for i, r := range rows {
		out[i] = templateRowToSummary(r)
	}
	return out, nil
}

// GetTemplate returns one template by filename. Linear scan is fine -
// templates are tens of items at most, and this avoids adding a new
// per-key query method to the index API just for the wiki.
func (m *Manager) GetTemplate(_ context.Context, filename string) (*TemplateSummary, bool, error) {
	rows, err := m.idx.ListTemplates()
	if err != nil {
		return nil, false, err
	}
	for _, r := range rows {
		if r.Filename == filename {
			s := templateRowToSummary(r)
			return &s, true, nil
		}
	}
	return nil, false, nil
}

// ListForms returns the forms for one template, mapped through ListOpts.
func (m *Manager) ListForms(_ context.Context, template string, opts ListOpts) ([]FormSummary, error) {
	rows, err := m.idx.ListForms(template, listOptsToQuery(opts))
	if err != nil {
		return nil, err
	}
	return formRowsToSummaries(rows), nil
}

// GetFormSummary fetches the metadata-only view of a form by composite
// key. The full rendered body lives behind RenderForm.
func (m *Manager) GetFormSummary(_ context.Context, template, datafile string) (*FormSummary, bool, error) {
	row, ok, err := m.idx.GetForm(template, datafile)
	if err != nil || !ok {
		return nil, false, err
	}
	s := formRowToSummary(*row)
	return &s, true, nil
}

// ListByTags returns forms across ALL templates that own every listed
// tag (AND).
func (m *Manager) ListByTags(_ context.Context, tags []string) ([]FormSummary, error) {
	rows, err := m.idx.ListByTags(tags)
	if err != nil {
		return nil, err
	}
	return formRowsToSummaries(rows), nil
}

// ResolveByID finds the form whose template's guid_field value
// matches `id`. Linear over the template's forms - which is the
// same complexity the old wiki used and is bounded by the per-
// template form count. Cheap at typical sizes.
func (m *Manager) ResolveByID(_ context.Context, template, id string) (*FormSummary, bool, error) {
	rows, err := m.idx.ListForms(template, index.QueryOpts{})
	if err != nil {
		return nil, false, err
	}
	for _, r := range rows {
		if r.ID == id {
			s := formRowToSummary(r)
			return &s, true, nil
		}
	}
	return nil, false, nil
}

// RenderForm wraps render.Manager.RenderForm with the small bit of
// post-processing the wiki cares about: lifting the frontmatter title
// out so the HTTP layer can stamp it into <title>, and falling back
// to the FormSummary's title (then the datafile name) when missing.
func (m *Manager) RenderForm(ctx context.Context, template, datafile string) (*RenderedPage, error) {
	res, err := m.ren.RenderForm(template, datafile)
	if err != nil {
		return nil, err
	}
	title := titleFromFrontmatter(res.Markdown)
	if title == "" {
		// Fall back to the form summary's title, which is itself a
		// chain (item_field → filename). This keeps the HTML page
		// title aligned with the sidebar title.
		if sum, ok, _ := m.GetFormSummary(ctx, template, datafile); ok && sum != nil {
			title = sum.Title
		}
	}
	if title == "" {
		title = datafile
	}
	return &RenderedPage{
		Template: template,
		Filename: datafile,
		Title:    title,
		Markdown: res.Markdown,
		HTML:     res.HTML,
	}, nil
}

// Rev returns the index's monotonic revision counter - the wiki uses
// it as a coarse ETag for "did anything change?" checks.
func (m *Manager) Rev(_ context.Context) (int64, error) { return m.idx.Rev() }

// ── projections ──────────────────────────────────────────────────────

func templateRowToSummary(r index.TemplateRow) TemplateSummary {
	return TemplateSummary{
		Stem:                strings.TrimSuffix(r.Filename, ".yaml"),
		Filename:            r.Filename,
		Name:                r.Name,
		ItemField:           r.ItemField,
		GuidField:           r.GuidField,
		TagsField:           r.TagsField,
		HasMarkdownTemplate: r.HasMarkdownTemplate,
		EnableCollection:    r.EnableCollection,
	}
}

func formRowToSummary(r index.FormRow) FormSummary {
	return FormSummary{
		Template:        r.Template,
		Filename:        r.Filename,
		ID:              r.ID,
		Title:           r.Title,
		FmTitle:         r.FmTitle,
		Author:          r.UpdatedName,
		Created:         r.Created,
		Updated:         r.Updated,
		Tags:            append([]string(nil), r.Tags...),
		ExpressionItems: r.ExpressionItems,
	}
}

func formRowsToSummaries(rows []index.FormRow) []FormSummary {
	out := make([]FormSummary, len(rows))
	for i, r := range rows {
		out[i] = formRowToSummary(r)
	}
	return out
}

func listOptsToQuery(o ListOpts) index.QueryOpts {
	return index.QueryOpts{
		Limit:   o.Limit,
		Offset:  o.Offset,
		OrderBy: o.OrderBy,
		Tags:    o.Tags,
	}
}

// titleFromFrontmatter pulls a `title:` value out of a markdown's
// YAML frontmatter via render.ParseFrontmatter. Returns "" when
// there's no frontmatter, no title key, or the title isn't a string.
func titleFromFrontmatter(md string) string {
	fm, _, err := render.ParseFrontmatter(md)
	if err != nil || fm == nil {
		return ""
	}
	if t, ok := fm["title"].(string); ok {
		return strings.TrimSpace(t)
	}
	return ""
}
