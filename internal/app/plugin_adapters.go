package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
)

// pluginLocaleAdapter wires plugin.LocaleProvider to the config
// manager so Manager.Run sources `formidable.i18n.t()` lookups from
// the active profile's `language`. Read goes through the manager's
// in-memory cache so per-Run cost is a map read, not disk I/O.
type pluginLocaleAdapter struct {
	cfg *config.Manager
}

func (a pluginLocaleAdapter) ActiveLocale() string {
	if a.cfg == nil {
		return ""
	}
	cfg, err := a.cfg.LoadUserConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.Language
}

// Adapters between the plugin module's access interfaces and the
// existing manager surface. Each adapter is a thin shim - no
// caching or transformation beyond marshalling typed values into
// the JSON-shaped maps the Lua bridge expects.

// toJSONMap converts any JSON-marshalable Go value into a
// map[string]any, the shape plugin's lvalue.go round-trips
// through. Used so plugin-side code sees the same JSON shape Vue
// receives - no parallel type vocabulary to maintain.
func toJSONMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// pluginTemplateAdapter implements plugin.TemplateAccess by
// composing dataprovider (for fast index-backed listing) with
// the template manager (for full-fat reads, including fields and
// markdown_template that the plugin may need for code-gen).
type pluginTemplateAdapter struct {
	dp  *dataprovider.Manager
	tpl *template.Manager
}

func (a pluginTemplateAdapter) ListTemplates() []map[string]any {
	rows, err := a.dp.ListTemplates(context.Background())
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, err := toJSONMap(r); err == nil {
			out = append(out, m)
		}
	}
	return out
}

func (a pluginTemplateAdapter) GetTemplate(filename string) (map[string]any, error) {
	t, err := a.tpl.LoadTemplate(filename)
	if err != nil {
		return nil, err
	}
	return toJSONMap(t)
}

// pluginCollectionAdapter wraps dataprovider.ListForms to give
// plugins "all forms of a template" without paginating. Plugin
// scripts that iterate every form (the wiki-export use case) want
// the full set in one call.
type pluginCollectionAdapter struct {
	dp *dataprovider.Manager
}

func (a pluginCollectionAdapter) ListCollection(templateFilename string) ([]map[string]any, error) {
	rows, err := a.dp.ListForms(context.Background(), templateFilename, dataprovider.ListOpts{})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, err := toJSONMap(r); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// pluginFormAdapter routes load/save through the storage manager.
// SaveForm goes through the same atomic-write path the Wails
// Storage service uses, so plugin writes get the same durability
// guarantees as user writes.
type pluginFormAdapter struct {
	sto *storage.Manager
}

func (a pluginFormAdapter) LoadForm(templateFilename, datafile string) (map[string]any, error) {
	f := a.sto.LoadForm(templateFilename, datafile)
	if f == nil {
		return nil, fmt.Errorf("form not found: %s/%s", templateFilename, datafile)
	}
	// Plugins receive the inner data only - meta is reserved (the
	// storage manager owns identity and timestamps and rewrites
	// them on every save).
	if f.Data == nil {
		return map[string]any{}, nil
	}
	return f.Data, nil
}

func (a pluginFormAdapter) SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) error {
	res := a.sto.SaveForm(ctx, templateFilename, datafile, data)
	if !res.Success {
		return fmt.Errorf("storage: save %s/%s: %s", templateFilename, datafile, res.Error)
	}
	return nil
}

// pluginRenderAdapter exposes the slideout render manager (the
// same one feeding the Storage workspace preview). Plugins
// rendering markdown for export-to-wiki get the same output the
// preview shows.
type pluginRenderAdapter struct {
	rdr *render.Manager
}

func (a pluginRenderAdapter) RenderMarkdown(templateFilename, datafile string) (string, error) {
	return a.rdr.RenderMarkdown(templateFilename, datafile)
}

func (a pluginRenderAdapter) RenderHTML(templateFilename, datafile string) (string, error) {
	md, err := a.rdr.RenderMarkdown(templateFilename, datafile)
	if err != nil {
		return "", err
	}
	return a.rdr.RenderHTMLOnly(md)
}

// pluginStorageAdapter exposes storage image bytes to Lua plugins
// that export to disk (wikiwonder copies referenced images alongside
// the generated markdown). Missing files come back as (nil, nil) so
// the plugin can skip silently rather than branch on errors.
type pluginStorageAdapter struct {
	sto *storage.Manager
}

func (a pluginStorageAdapter) ImageBytes(templateFilename, name string) ([]byte, error) {
	b, _, err := a.sto.OpenImageFile(templateFilename, name)
	return b, err
}

// pluginFMAdapter exposes the render module's frontmatter helpers
// to Lua. Stateless - the underlying functions are pure - so the
// adapter holds no fields. Lives here (not in render's service) so
// the plugin module stays the sole front door for "what Lua can do."
type pluginFMAdapter struct{}

func (pluginFMAdapter) Parse(markdown string) (map[string]any, string, error) {
	return render.ParseFrontmatter(markdown)
}

func (pluginFMAdapter) Build(data map[string]any, body string) string {
	return render.BuildFrontmatter(data, body)
}

// pluginStatsAdapter bridges the stat manager into Lua's
// formidable.stats.* and formidable.facets.* namespaces. Each chart-
// neutral Result is marshalled to the same JSON-shaped map Vue
// receives, so plugin authors and the frontend share one vocabulary.
// One adapter satisfies both plugin.StatsAccess and
// plugin.FacetStatsAccess since they read the same manager.
type pluginStatsAdapter struct {
	st *stat.Manager
}

func (a pluginStatsAdapter) Distribution(template, fieldKey string, col *int) (map[string]any, error) {
	return statResultMap(a.st.Distribution(template, fieldKey, col))
}

func (a pluginStatsAdapter) NumericStats(template, fieldKey string, col *int, percentile *float64) (map[string]any, error) {
	return statResultMap(a.st.NumericStats(template, fieldKey, col, percentile))
}

func (a pluginStatsAdapter) TimeSeries(template, fieldKey string, col *int, period string) (map[string]any, error) {
	return statResultMap(a.st.TimeSeries(template, fieldKey, col, period))
}

func (a pluginStatsAdapter) FacetDistribution(template, facetKey string) (map[string]any, error) {
	return statResultMap(a.st.FacetDistribution(template, facetKey))
}

func (a pluginStatsAdapter) FacetCross(template, keyA, keyB string) (map[string]any, error) {
	return statResultMap(a.st.CrossTab(template, keyA, keyB))
}

func (a pluginStatsAdapter) TotalForms(template string) (int, error) {
	return a.st.TotalForms(template)
}

// statTemplateSource resolves a template's named statistical object to
// its stored DSL string, implementing stat.StatisticSource over the
// template manager. Used by Stat.EvaluateObject (Wails + Lua).
type statTemplateSource struct {
	tpl *template.Manager
}

func (s statTemplateSource) StatisticDSL(tplFile, name string) (string, bool, error) {
	t, err := s.tpl.LoadTemplate(tplFile)
	if err != nil {
		return "", false, err
	}
	for _, st := range t.Statistics {
		if st.Name == name {
			return st.DSL, true, nil
		}
	}
	return "", false, nil
}

// statSourceOptions gives the stat engine a facet dimension's full,
// ordered option labels, so a statistic shows every defined option
// (including zero-count ones) instead of only the values present in the
// data. Open-ended / non-facet sources return ok=false (present-values).
type statSourceOptions struct {
	tpl *template.Manager
}

func (s statSourceOptions) DimensionLabels(tplFile string, src stat.SourceRef) ([]string, bool) {
	t, err := s.tpl.LoadTemplate(tplFile)
	if err != nil {
		return nil, false
	}
	return dimensionOptionLabels(t, src)
}

// dimensionOptionLabels returns the full ordered category set for a
// dimension source whose categories are fixed by definition: a facet's
// option labels (facets store the label as the selected value), or a
// choice field's option values (dropdown/radio/multioption store the
// value; boolean indexes as "true"/"false"). Open-ended sources - dates,
// numbers, free text, and (for now) table columns - return ok=false so the
// engine falls back to the values present in the data.
func dimensionOptionLabels(t *template.Template, src stat.SourceRef) ([]string, bool) {
	if src.Kind == stat.SourceFacet {
		for _, f := range t.Facets {
			if f.Key == src.Key {
				out := make([]string, 0, len(f.Options))
				for _, o := range f.Options {
					out = append(out, o.Label)
				}
				return out, true
			}
		}
		return nil, false
	}
	if src.Column != "" {
		return nil, false // table columns: deferred
	}
	for _, fld := range t.Fields {
		if fld.Key != src.Key {
			continue
		}
		switch fld.Type {
		case "dropdown", "radio", "multioption":
			return optionValues(fld.Options), true
		case "boolean":
			return []string{"true", "false"}, true
		}
		return nil, false
	}
	return nil, false
}

// optionValues pulls the `value` of each option (string options are their
// own value), skipping blanks. Mirrors what pickValues stores for choice
// fields, so the axis labels line up with the indexed values.
func optionValues(options []any) []string {
	out := make([]string, 0, len(options))
	for _, o := range options {
		switch v := o.(type) {
		case string:
			if v != "" {
				out = append(out, v)
			}
		case map[string]any:
			if s, _ := v["value"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// pluginStatObjectAdapter bridges Stat.EvaluateObject into the Lua
// formidable.statistical(tpl, name) surface, flattening the Grid to the
// JSON-shaped map the Lua bridge round-trips.
type pluginStatObjectAdapter struct {
	svc *stat.Service
}

func (a pluginStatObjectAdapter) EvaluateObject(template, name string) (map[string]any, error) {
	g, err := a.svc.EvaluateObject(template, name)
	if err != nil {
		return nil, err
	}
	return toJSONMap(g)
}

// statResultMap collapses a (Result, error) pair into the JSON map the
// Lua bridge expects, short-circuiting on error.
func statResultMap(res *stat.Result, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	return toJSONMap(res)
}

// pluginHTTPAdapter wires plugin.HTTPClient to the running wiki
// HTTP server. IsAvailable mirrors wiki.Status().Running; Fetch
// proxies via system.ProxyFetchRemote against the loopback URL on
// the wiki server's actual port. The plugin module stays unaware
// of either dependency - both are composed here.
type pluginHTTPAdapter struct {
	wiki *wiki.Manager
	sys  *system.Manager
}

func (a pluginHTTPAdapter) IsAvailable() bool {
	if a.wiki == nil {
		return false
	}
	return a.wiki.Status().Running
}

func (a pluginHTTPAdapter) Fetch(method, path, body string, headers map[string]string) (plugin.HTTPResponse, error) {
	st := a.wiki.Status()
	if !st.Running || st.Port == 0 {
		return plugin.HTTPResponse{}, fmt.Errorf("internal server not running")
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", st.Port, path)
	res, err := a.sys.ProxyFetchRemote(url, system.FetchOptions{
		Method:       method,
		Headers:      headers,
		Body:         body,
		TimeoutSecs:  30,
		FollowRedirs: true,
	})
	if err != nil {
		return plugin.HTTPResponse{}, err
	}
	return plugin.HTTPResponse{
		Status:  res.StatusCode,
		Body:    res.Body,
		Headers: res.Headers,
	}, nil
}
