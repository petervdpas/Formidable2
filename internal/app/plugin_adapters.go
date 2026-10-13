package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/connection"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/pdf"
	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
)

// pluginLocaleAdapter sources `formidable.i18n.t()` lookups from the active
// profile's `language`. Reads hit the config manager's in-memory cache.
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

// Adapters between the plugin module's access interfaces and the existing
// manager surface. Each is a thin shim that marshals typed values into the
// JSON-shaped maps the Lua bridge expects.

// toJSONMap renders any JSON-marshalable value as the map shape plugin's
// lvalue.go round-trips, so plugin code sees the same JSON shape Vue receives.
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

// pluginTemplateAdapter composes dataprovider (fast index-backed listing) with
// the template manager (full reads, including fields and markdown_template).
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

// pluginCollectionAdapter returns all forms of a template in one call (no
// pagination), for plugin scripts that iterate every form (wiki-export).
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

// pluginFormAdapter routes load/save through the storage manager, so plugin
// writes use the same atomic-write path (and durability) as user writes.
type pluginFormAdapter struct {
	sto *storage.Manager
}

func (a pluginFormAdapter) LoadForm(templateFilename, datafile string) (map[string]any, error) {
	f := a.sto.LoadForm(templateFilename, datafile)
	if f == nil {
		return nil, fmt.Errorf("form not found: %s/%s", templateFilename, datafile)
	}
	// Inner data only: storage owns meta (identity, timestamps) and rewrites it on every save.
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

// pluginRenderAdapter exposes the slideout render manager, so export-to-wiki
// plugins render the same markdown the Storage workspace preview shows.
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

// pluginStorageAdapter exposes storage image bytes to disk-exporting plugins
// (wikiwonder copies referenced images alongside the generated markdown).
// Missing files return (nil, nil) so the plugin can skip silently.
type pluginStorageAdapter struct {
	sto *storage.Manager
}

func (a pluginStorageAdapter) ImageBytes(templateFilename, name string) ([]byte, error) {
	b, _, err := a.sto.OpenImageFile(templateFilename, name)
	return b, err
}

// pluginFMAdapter exposes render's pure frontmatter helpers to Lua. Lives here
// (not in render's service) so plugin stays the sole front door for Lua.
type pluginFMAdapter struct{}

func (pluginFMAdapter) Parse(markdown string) (map[string]any, string, error) {
	return render.ParseFrontmatter(markdown)
}

func (pluginFMAdapter) Build(data map[string]any, body string) string {
	return render.BuildFrontmatter(data, body)
}

// pluginStatsAdapter bridges the stat manager into Lua's formidable.stats.* and
// formidable.facets.* namespaces. One adapter satisfies both plugin.StatsAccess
// and plugin.FacetStatsAccess since they read the same manager.
type pluginStatsAdapter struct {
	st *stat.Manager
}

func (a pluginStatsAdapter) Distribution(template, fieldKey string, col *int) (map[string]any, error) {
	return statGridMap(a.st.Distribution(template, fieldKey, col))
}

func (a pluginStatsAdapter) NumericStats(template, fieldKey string, col *int, percentile *float64) (map[string]any, error) {
	return statGridMap(a.st.NumericStats(template, fieldKey, col, percentile))
}

func (a pluginStatsAdapter) TimeSeries(template, fieldKey string, col *int, period string) (map[string]any, error) {
	return statGridMap(a.st.TimeSeries(template, fieldKey, col, period))
}

func (a pluginStatsAdapter) FacetDistribution(template, facetKey string) (map[string]any, error) {
	return statGridMap(a.st.FacetDistribution(template, facetKey))
}

func (a pluginStatsAdapter) FacetCross(template, keyA, keyB string) (map[string]any, error) {
	return statGridMap(a.st.CrossTab(template, keyA, keyB))
}

func (a pluginStatsAdapter) TotalForms(template string) (int, error) {
	return a.st.TotalForms(template)
}

// statTemplateSource resolves a template's named statistical object to its
// stored DSL, implementing stat.StatisticSource. Used by Stat.EvaluateObject.
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

func (s statTemplateSource) ListStatistics(tplFile string) ([]stat.StatObject, error) {
	t, err := s.tpl.LoadTemplate(tplFile)
	if err != nil {
		return nil, err
	}
	out := make([]stat.StatObject, 0, len(t.Statistics)+len(t.Scalings))
	for _, st := range t.Statistics {
		out = append(out, stat.StatObject{
			Name:      st.Name,
			Label:     st.Label,
			DSL:       st.DSL,
			Composite: toStatComposite(st.Composite),
			Scaling:   toStatScaling(st.Scaling),
		})
	}
	// Scalings now live at the template level (a facet weighting subsystem), but
	// the Statistical Engine still resolves the DSL scale "<name>" clause through
	// this catalog, so surface each one as a StatObject of kind scaling.
	for _, sc := range t.Scalings {
		out = append(out, stat.StatObject{
			Name:    sc.Name,
			Label:   sc.Label,
			Scaling: toStatScalingFromTop(sc),
		})
	}
	return out, nil
}

// toStatComposite maps a template's stored composite spec onto stat.CompositeSpec,
// keeping the template package free of a stat dependency. nil maps to nil.
func toStatComposite(c *template.StatComposite) *stat.CompositeSpec {
	if c == nil {
		return nil
	}
	edges := make([]stat.CompositeEdgeSpec, 0, len(c.Edges))
	for _, e := range c.Edges {
		edges = append(edges, stat.CompositeEdgeSpec{Branch: e.Branch, Child: e.Child})
	}
	return &stat.CompositeSpec{Parent: c.Parent, Edges: edges}
}

// toStatScaling maps a template's stored scaling spec onto stat.Scaling,
// keeping the template package free of a stat dependency. nil maps to nil.
func toStatScaling(sc *template.StatScaling) *stat.Scaling {
	if sc == nil {
		return nil
	}
	kind := stat.SourceField
	if sc.Source.Kind == "facet" {
		kind = stat.SourceFacet
	}
	weights := make([]stat.WeightEntry, 0, len(sc.Weights))
	for _, w := range sc.Weights {
		weights = append(weights, stat.WeightEntry{Label: w.Label, Factor: w.Factor})
	}
	return &stat.Scaling{
		Source:  stat.SourceRef{Kind: kind, Key: sc.Source.Key, Column: sc.Source.Column},
		Weights: weights,
		Default: sc.Default,
	}
}

// toStatScalingFromTop maps a top-level template.Scaling onto stat.Scaling
// (the same shape toStatScaling produces from the legacy nested form).
func toStatScalingFromTop(sc template.Scaling) *stat.Scaling {
	kind := stat.SourceField
	if sc.Source.Kind == "facet" {
		kind = stat.SourceFacet
	}
	weights := make([]stat.WeightEntry, 0, len(sc.Weights))
	for _, w := range sc.Weights {
		weights = append(weights, stat.WeightEntry{Label: w.Label, Factor: w.Factor})
	}
	return &stat.Scaling{
		Source:  stat.SourceRef{Kind: kind, Key: sc.Source.Key, Column: sc.Source.Column},
		Weights: weights,
		Default: sc.Default,
	}
}

// statSourceOptions gives the stat engine a dimension's full ordered option
// labels, so a statistic shows every defined option (including zero-count ones)
// instead of only values present in the data. Open-ended sources return ok=false.
type statSourceOptions struct {
	tpl *template.Manager
}

func (s statSourceOptions) DimensionLabels(tplFile string, src stat.SourceRef) ([]stat.CategoryOption, bool) {
	t, err := s.tpl.LoadTemplate(tplFile)
	if err != nil {
		return nil, false
	}
	return dimensionOptionLabels(t, src)
}

// dimensionOptionLabels returns the full ordered category set for a dimension
// whose categories are fixed by definition. Each option carries the stored
// Value (group-by key) and display Label: a facet has Value==Label; a choice
// field stores the option value and displays its caption. Open-ended sources
// (dates, numbers, free text, table columns) return ok=false so the engine
// falls back to the values present in the data.
func dimensionOptionLabels(t *template.Template, src stat.SourceRef) ([]stat.CategoryOption, bool) {
	if src.Kind == stat.SourceFacet {
		for _, f := range t.Facets {
			if f.Key == src.Key {
				out := make([]stat.CategoryOption, 0, len(f.Options))
				for _, o := range f.Options {
					out = append(out, stat.CategoryOption{Value: o.Label, Label: o.Label})
				}
				return out, true
			}
		}
		return nil, false
	}
	if src.Column != "" {
		// Dropdown/radio columns carry categories in the option's pipe-delimited
		// `choices` string; other column types have no fixed set.
		for _, fld := range t.Fields {
			if fld.Key != src.Key {
				continue
			}
			col := tableColumnOption(fld.Options, src.Column)
			if col == nil {
				return nil, false
			}
			ctype, _ := col["type"].(string)
			if ctype == "dropdown" || ctype == "radio" {
				opts := parseColumnChoices(asString(col["choices"]))
				if len(opts) > 0 {
					return opts, true
				}
			}
			return nil, false
		}
		return nil, false
	}
	for _, fld := range t.Fields {
		if fld.Key != src.Key {
			continue
		}
		switch fld.Type {
		case "dropdown", "radio", "multioption":
			opts := choiceOptions(fld.Options)
			return opts, len(opts) > 0
		case "boolean":
			opts := choiceOptions(fld.Options)
			if len(opts) == 0 {
				// Booleans index as "true"/"false"; show both even without custom labels.
				opts = []stat.CategoryOption{{Value: "true", Label: "true"}, {Value: "false", Label: "false"}}
			}
			return opts, true
		}
		return nil, false
	}
	return nil, false
}

// tableColumnOption finds a table field's column definition by value key, or
// nil. Each column is a {value, type, label, choices} map.
func tableColumnOption(options []any, columnKey string) map[string]any {
	for _, o := range options {
		if m, ok := o.(map[string]any); ok {
			if v, _ := m["value"].(string); v == columnKey {
				return m
			}
		}
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// parseColumnChoices parses a dropdown column's `choices` string: pipe-delimited
// "value:label" pairs (whitespace tolerant). A pair with no colon is its own label.
func parseColumnChoices(s string) []stat.CategoryOption {
	out := make([]stat.CategoryOption, 0)
	for _, part := range strings.Split(s, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, l, found := strings.Cut(part, ":")
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		l = strings.TrimSpace(l)
		if !found || l == "" {
			l = v
		}
		out = append(out, stat.CategoryOption{Value: v, Label: l})
	}
	return out
}

// choiceOptions maps a choice field's options to value/label pairs. A {value,
// label} map uses value as the group-by key (what pickValues stores); label
// defaults to value when blank. Blank values are skipped.
func choiceOptions(options []any) []stat.CategoryOption {
	out := make([]stat.CategoryOption, 0, len(options))
	for _, o := range options {
		switch v := o.(type) {
		case string:
			if v != "" {
				out = append(out, stat.CategoryOption{Value: v, Label: v})
			}
		case map[string]any:
			val, _ := v["value"].(string)
			if val == "" {
				continue
			}
			lbl, _ := v["label"].(string)
			if lbl == "" {
				lbl = val
			}
			out = append(out, stat.CategoryOption{Value: val, Label: lbl})
		}
	}
	return out
}

// statColumnResolver maps a table column value-key to its positional index in
// form_values.col (the order of the field's options), implementing
// stat.ColumnResolver so a table-column DSL source resolves to its column.
type statColumnResolver struct {
	tpl *template.Manager
}

func (s statColumnResolver) ColumnIndex(tplFile, fieldKey, columnKey string) (int, bool) {
	t, err := s.tpl.LoadTemplate(tplFile)
	if err != nil {
		return 0, false
	}
	return columnIndexIn(t, fieldKey, columnKey)
}

// columnIndexIn finds a table column's position by its option value key. The
// position matches what pickValues stores in form_values.col (cells are indexed
// by their position in the field's options).
func columnIndexIn(t *template.Template, fieldKey, columnKey string) (int, bool) {
	for _, f := range t.Fields {
		if f.Key != fieldKey {
			continue
		}
		for i, o := range f.Options {
			if m, ok := o.(map[string]any); ok {
				if v, _ := m["value"].(string); v == columnKey {
					return i, true
				}
			}
		}
		return 0, false
	}
	return 0, false
}

// pluginStatObjectAdapter bridges Stat.EvaluateObject into the Lua
// formidable.statistical(tpl, name) surface, flattening the Grid to a JSON map.
type pluginStatObjectAdapter struct {
	svc *stat.Service
}

func (a pluginStatObjectAdapter) ListObjects(template string) ([]map[string]any, error) {
	objs, err := a.svc.ListObjects(template)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(objs))
	for _, o := range objs {
		kind := "dsl"
		if o.Composite != nil {
			kind = "composite"
		} else if o.Scaling != nil {
			kind = "scaling"
		}
		out = append(out, map[string]any{"name": o.Name, "label": o.Label, "dsl": o.DSL, "kind": kind})
	}
	return out, nil
}

func (a pluginStatObjectAdapter) EvaluateObject(template, name string) (map[string]any, error) {
	g, err := a.svc.EvaluateObject(template, name)
	if err != nil {
		return nil, err
	}
	return toJSONMap(g)
}

func (a pluginStatObjectAdapter) EvaluateComposite(template, name string) (map[string]any, error) {
	cg, err := a.svc.EvaluateComposite(template, name)
	if err != nil {
		return nil, err
	}
	return toJSONMap(cg)
}

// statGridMap collapses a (*Grid, error) pair into the JSON map the Lua bridge
// expects, short-circuiting on error.
func statGridMap(g *stat.Grid, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	return toJSONMap(g)
}

// pluginHTTPAdapter wires plugin.HTTPClient to the running wiki HTTP server:
// IsAvailable mirrors wiki.Status().Running; Fetch proxies via
// system.ProxyFetchRemote against the loopback URL on the wiki's actual port.
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

// pluginClientAdapter is the api-client store as plugins see it: create-only
// install of the clients a plugin ships, and the read-only calls behind
// formidable.client. No write, no definition edit, no credential surface.
type pluginClientAdapter struct {
	m   *connection.Manager
	inv *connection.Invoker
}

func (a pluginClientAdapter) HasClient(id string) bool { return a.m.Has(id) }

func (a pluginClientAdapter) InstallClient(id string, read func(name string) ([]byte, error)) (bool, error) {
	return a.m.Install(id, read)
}

// CreateClient decodes a script's definition strictly, so a misspelt option is
// an error instead of a silently ignored one. An id that already exists is
// reported as not created, never as a failure and never as an update.
func (a pluginClientAdapter) CreateClient(def map[string]any, spec []byte) (bool, error) {
	raw, err := json.Marshal(def)
	if err != nil {
		return false, err
	}
	var c connection.Connection
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return false, fmt.Errorf("bad option: %w", err)
	}
	if c.SpecFile != "" || c.SpecURL != "" {
		return false, errors.New("bad option: the document is named by spec, not spec_file or spec_url")
	}
	if a.m.Has(c.ID) {
		return false, nil
	}
	if err := a.m.Create(c, spec); err != nil {
		if errors.Is(err, connection.ErrExists) && a.m.Has(c.ID) {
			return false, nil
		}
		return false, clientErr(err)
	}
	return true, nil
}

// InspectDocument parses a document without storing it.
func (a pluginClientAdapter) InspectDocument(spec []byte) (plugin.DocumentInfo, error) {
	cat, err := connection.ParseSpec(spec)
	if err != nil {
		return plugin.DocumentInfo{}, err
	}
	return plugin.DocumentInfo{
		Title:      cat.Title,
		Operations: len(cat.Operations),
		Ext:        connection.SpecExtension(spec),
		Server:     connection.FirstAbsoluteServer(cat),
	}, nil
}

func (a pluginClientAdapter) ReadSettings(def []byte) (plugin.ClientSettings, error) {
	s, err := connection.ReadBundleSettings(def)
	return plugin.ClientSettings{Name: s.Name, BaseURL: s.BaseURL, Custom: s.Custom}, err
}

func (a pluginClientAdapter) WithSettings(def []byte, id string, s plugin.ClientSettings) ([]byte, error) {
	return connection.SetBundleSettings(def, id, connection.BundleSettings{Name: s.Name, BaseURL: s.BaseURL})
}

// ClientOperations lists the document's operations, so a script can find what
// a client derived from a document alone is able to call.
func (a pluginClientAdapter) ClientOperations(id string) ([]map[string]any, error) {
	cat, err := a.m.Catalog(id)
	if err != nil {
		return nil, clientErr(err)
	}
	out := make([]map[string]any, 0, len(cat.Operations))
	for _, op := range cat.Operations {
		params := make([]any, 0, len(op.Params))
		for _, p := range op.Params {
			params = append(params, map[string]any{"name": p.Name, "in": p.In, "required": p.Required, "type": p.Type})
		}
		row := map[string]any{
			"id": op.ID, "method": op.Method, "path": op.Path, "summary": op.Summary, "params": params,
		}
		if op.Body != nil {
			types := make([]any, 0, len(op.Body.ContentTypes))
			for _, ct := range op.Body.ContentTypes {
				types = append(types, ct)
			}
			row["body"] = map[string]any{"required": op.Body.Required, "content_types": types}
		}
		out = append(out, row)
	}
	return out, nil
}

func (a pluginClientAdapter) ClientCall(ctx context.Context, client, operation string, params map[string]string, body any, contentType string) (map[string]any, error) {
	res, err := a.inv.Call(ctx, connection.CallRequest{
		Connection: client, Operation: operation, Params: params, Body: body, ContentType: contentType,
	})
	if err != nil {
		return nil, clientErr(err)
	}
	return toJSONMap(res)
}

func (a pluginClientAdapter) ListClients() ([]map[string]any, error) {
	rows, err := a.m.List()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row, err := toJSONMap(r)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// ClientResources describes what a client offers: each resource, whether it
// can search and resolve an id, and the fields a caller may select.
func (a pluginClientAdapter) ClientResources(id string) ([]map[string]any, error) {
	c, _, err := a.m.Get(id)
	if err != nil {
		return nil, clientErr(err)
	}
	out := make([]map[string]any, 0, len(c.Resources))
	for _, r := range c.Resources {
		fields := make([]any, 0, len(r.Fields))
		for _, f := range r.Fields {
			fields = append(fields, map[string]any{"key": f.Key, "label": f.Label, "type": f.Type})
		}
		out = append(out, map[string]any{
			"key":        r.Key,
			"label":      r.Label,
			"can_search": r.SearchParam != "",
			"can_fetch":  strings.TrimSpace(r.Get.Operation) != "",
			"fields":     fields,
		})
	}
	return out, nil
}

func (a pluginClientAdapter) ClientItems(ctx context.Context, client, resource string, q plugin.ClientQuery) (map[string]any, error) {
	page, err := a.inv.List(ctx, connection.ListRequest{
		Connection: client,
		Resource:   resource,
		Search:     q.Search,
		Limit:      q.Limit,
		Offset:     q.Offset,
		Cursor:     q.Cursor,
		Select:     q.Select,
		Params:     q.Params,
	})
	if err != nil {
		return nil, clientErr(err)
	}
	// A nil slice would reach Lua as nil; a script iterating an empty page must get a table.
	if page.Items == nil {
		page.Items = []connection.Item{}
	}
	return toJSONMap(page)
}

func (a pluginClientAdapter) ClientFetch(ctx context.Context, client, resource, id string, q plugin.ClientQuery) (map[string]any, error) {
	item, err := a.inv.Fetch(ctx, connection.FetchRequest{
		Connection: client,
		Resource:   resource,
		ID:         id,
		Select:     q.Select,
		Params:     q.Params,
	})
	if err != nil {
		return nil, clientErr(err)
	}
	return toJSONMap(item)
}

// ClientSnapshot shapes an item through connection.Snapshot, so a record a
// plugin writes holds exactly what the api-client field's own picker stores.
func (a pluginClientAdapter) ClientSnapshot(item map[string]any, keys []string) (map[string]any, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var it connection.Item
	if err := json.Unmarshal(raw, &it); err != nil {
		return nil, fmt.Errorf("not an item from client.items or client.fetch: %w", err)
	}
	if keys == nil {
		for k := range it.Fields {
			keys = append(keys, k)
		}
	}
	snap := connection.Snapshot(it, keys, time.Now())
	if snap == nil {
		return nil, errors.New("item has no id")
	}
	return snap, nil
}

// clientErr turns an InvokeError into the plugin's ClientError: flat
// "code: message" as text (the InvokeError's own Error() is JSON for the Wails
// boundary), with status, delivery and the remote's answer kept beside it for
// formidable.client.attempt.
func clientErr(err error) error {
	var ie *connection.InvokeError
	if !errors.As(err, &ie) {
		return err
	}
	out := &plugin.ClientError{
		Code:     string(ie.Code),
		Message:  ie.Message,
		Status:   ie.Status,
		Delivery: ie.Delivery(),
		Body:     ie.Body,
	}
	if strings.TrimSpace(ie.Body) != "" {
		var data any
		if json.Unmarshal([]byte(ie.Body), &data) == nil {
			out.Data = data
		}
	}
	return out
}

// pdfExporter is the slice of *pdf.Service a plugin reaches: the same export
// the Storage workspace runs, guarded against a concurrent second export.
type pdfExporter interface {
	GetStatus() pdf.Status
	ExportPDF(templateFilename, datafile string, opts pdf.ExportOpts) (pdf.Result, error)
}

// pluginPDFAdapter puts PDF export behind formidable.pdf. svc is set after the
// PDF service exists, which is later in the composition root than the plugin
// manager; until then, and whenever export is not set up, it reads as inactive.
type pluginPDFAdapter struct {
	svc pdfExporter
}

func (a *pluginPDFAdapter) PDFActive() bool {
	return a != nil && a.svc != nil && a.svc.GetStatus().Active
}

func (a *pluginPDFAdapter) ExportPDF(template, datafile string, opts map[string]any) (map[string]any, error) {
	if a == nil || a.svc == nil {
		return nil, pdfErr(pdf.ErrPDFNotActivated)
	}
	var o pdf.ExportOpts
	if opts != nil {
		raw, err := json.Marshal(opts)
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return nil, fmt.Errorf("bad option: %w", err)
		}
	}
	res, err := a.svc.ExportPDF(template, datafile, o)
	if err != nil {
		return nil, pdfErr(err)
	}
	return map[string]any{
		"path":        res.Path,
		"bytes":       res.Bytes,
		"duration_ms": res.Duration.Milliseconds(),
	}, nil
}

// pdfErr flattens an export failure to "code: message. hint", the same shape
// clientErr gives, so a script sees plainly that export is not set up.
func pdfErr(err error) error {
	ee := pdf.MapExportError(err)
	if ee == nil {
		return err
	}
	msg := strings.TrimSuffix(ee.Message, ".")
	if ee.Hint != "" {
		msg += ". " + ee.Hint
	}
	return fmt.Errorf("%s: %s", ee.Code, msg)
}

// pluginTemplateInstaller is the template store as plugins see it: create-only
// creation of the templates a plugin holds, through the same save path as the
// template editor, so indexing and enablement follow as they do there.
type pluginTemplateInstaller struct {
	tpl *template.Manager
}

func (a pluginTemplateInstaller) HasTemplate(name string) bool { return a.tpl.Exists(name) }

func (a pluginTemplateInstaller) CreateTemplate(name string, yaml []byte) (bool, error) {
	err := a.tpl.CreateFromYAML(name, yaml)
	if errors.Is(err, template.ErrExists) {
		return false, nil
	}
	return err == nil, err
}

func (a pluginTemplateInstaller) InspectTemplate(name string, yaml []byte) (plugin.TemplateInfo, error) {
	if err := template.CheckFilename(name); err != nil {
		return plugin.TemplateInfo{}, err
	}
	t, err := template.ParseYAML(yaml)
	if err != nil {
		return plugin.TemplateInfo{}, err
	}
	return plugin.TemplateInfo{Name: t.Name, Fields: len(t.Fields)}, nil
}

func (a pluginTemplateInstaller) TemplateSource(name string) (string, error) {
	return a.tpl.Source(name)
}

func (a pluginTemplateInstaller) TemplateFiles() ([]string, error) { return a.tpl.ListTemplates() }

func (a pluginTemplateInstaller) ReplaceTemplate(name string, yaml []byte) error {
	return a.tpl.ReplaceFromYAML(name, yaml)
}

func (a pluginTemplateInstaller) TemplateDiffers(name string, yaml []byte) (bool, error) {
	return a.tpl.DiffersFromYAML(name, yaml)
}
