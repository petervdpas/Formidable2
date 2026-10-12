package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Access interfaces: each lists the exact methods Lua can reach. New Lua surface means an explicit interface change here.

// TemplateAccess is the formidable.template.* surface.
type TemplateAccess interface {
	ListTemplates() []map[string]any
	GetTemplate(filename string) (map[string]any, error)
}

// CollectionAccess is the formidable.collection.* surface.
type CollectionAccess interface {
	ListCollection(templateFilename string) ([]map[string]any, error)
}

// FormAccess is the formidable.form.* surface; SaveForm goes through the storage manager's atomic-write path so plugin writes never tear.
type FormAccess interface {
	LoadForm(templateFilename, datafile string) (map[string]any, error)
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) error
}

// RenderAccess is the formidable.render.* surface: rendered markdown/HTML for a (template, datafile) pair.
type RenderAccess interface {
	RenderMarkdown(templateFilename, datafile string) (string, error)
	RenderHTML(templateFilename, datafile string) (string, error)
}

// FMAccess is the formidable.fm.* surface. Parse returns (data, body) with the `---...---` block removed; data is nil when absent.
// Build re-emits a frontmatter block from data; nil/empty data returns body unchanged.
type FMAccess interface {
	Parse(markdown string) (map[string]any, string, error)
	Build(data map[string]any, body string) string
}

// FSAccess is the formidable.fs.* surface. Unsandboxed: plugin authors
// are trusted, and the user reviewed the plugin before installing it.
type FSAccess interface {
	Read(path string) (string, error)
	Write(path, content string) error
	Mkdir(path string) error
	List(path string) ([]string, error)
	Exists(path string) bool
	Copy(from, to string) error
	Remove(path string) error
}

// StorageAccess is the formidable.storage.* surface; ImageBytes returns (nil, nil) when the file isn't present.
type StorageAccess interface {
	ImageBytes(templateFilename, name string) ([]byte, error)
}

// StatsAccess is the formidable.stats.* surface; each method returns the stat Result flattened to a map[string]any.
// col is the table-column index (nil for a scalar field); percentile is in [0,100], nil to skip.
type StatsAccess interface {
	Distribution(template, fieldKey string, col *int) (map[string]any, error)
	NumericStats(template, fieldKey string, col *int, percentile *float64) (map[string]any, error)
	TimeSeries(template, fieldKey string, col *int, period string) (map[string]any, error)
}

// FacetStatsAccess is the formidable.facets.* surface; same Result map shape as StatsAccess, TotalForms is the percentage denominator.
type FacetStatsAccess interface {
	FacetDistribution(template, facetKey string) (map[string]any, error)
	FacetCross(template, keyA, keyB string) (map[string]any, error)
	TotalForms(template string) (int, error)
}

// StatObjectAccess is the formidable.statistical surface. ListObjects yields {name, label, dsl, kind};
// EvaluateObject runs one into a rank-N values grid; EvaluateComposite runs a hop route into a {parent, branches} grid.
type StatObjectAccess interface {
	ListObjects(template string) ([]map[string]any, error)
	EvaluateObject(template, name string) (map[string]any, error)
	EvaluateComposite(template, name string) (map[string]any, error)
}

// ClientQuery is the options table of formidable.client.items / fetch. Select names declared fields to project (empty = all); Params overlay the resource binding's.
type ClientQuery struct {
	Search string
	Limit  int
	Offset int
	Cursor string
	Select []string
	Params map[string]string
}

// ClientAccess is the read side of the api-client store exposed as formidable.client. Results are JSON-shaped; ctx is the Run's, so Stop cancels a remote call.
// Read-only by construction: no write, no definition edit, no credential.
type ClientAccess interface {
	ListClients() ([]map[string]any, error)
	ClientResources(id string) ([]map[string]any, error)
	ClientItems(ctx context.Context, client, resource string, q ClientQuery) (map[string]any, error)
	ClientFetch(ctx context.Context, client, resource, id string, q ClientQuery) (map[string]any, error)
	// ClientOperations lists every operation in the client's document; ClientCall runs one by id, whatever its method, and returns {method, url, duration_ms, status, content_type, data | body}.
	// body is nil for none, a string to send verbatim, or JSON-shaped data; contentType overrides the media type the document declares.
	ClientOperations(id string) ([]map[string]any, error)
	ClientCall(ctx context.Context, client, operation string, params map[string]string, body any, contentType string) (map[string]any, error)
	// ClientSnapshot turns an item into the value an api-client field stores; nil keys keeps every field the item carries.
	ClientSnapshot(item map[string]any, keys []string) (map[string]any, error)
}

// PDFAccess is the formidable.pdf.* surface: the same export the Storage workspace offers, for one form.
// ExportPDF returns {path, bytes, duration_ms}; opts carries the export options (output_path, style, cover_template, disable_cover, disable_theme) or nil.
type PDFAccess interface {
	PDFActive() bool
	ExportPDF(template, datafile string, opts map[string]any) (map[string]any, error)
}

// ExecOptions narrows what `formidable.exec(cmd, args, opts)` accepts; a Go struct (not a free-form map) keeps the contract from drifting.
type ExecOptions struct {
	Cwd     string
	Env     map[string]string
	Timeout time.Duration
}

// ExecResult is the value Lua sees when exec returns.
type ExecResult struct {
	Stdout string
	Stderr string
	Exit   int
}

// ExecRunner is the formidable.exec surface.
type ExecRunner interface {
	Exec(cmd string, args []string, opts ExecOptions) (ExecResult, error)
}

// RunBarEmitter receives one RunBarEvent per formidable.run.bar call, fired synchronously; production wires it to a Wails event. Nil drops events.
type RunBarEmitter func(RunBarEvent)

// RunStatusEmitter receives one RunStatusEvent per formidable.run.status call (same semantics as RunBarEmitter).
type RunStatusEmitter func(RunStatusEvent)

// RunChartEmitter receives one RunChartEvent per formidable.run.chart call (same semantics as RunBarEmitter).
type RunChartEmitter func(RunChartEvent)

// RunOptionsEmitter receives one RunOptionsEvent per formidable.run.options call (same semantics as RunBarEmitter).
type RunOptionsEmitter func(RunOptionsEvent)

// nilGuard returns a closure that errors `<ns>: not configured` when the dep wasn't wired, so every namespace fails uniformly.
func nilGuard(ns string) lua.LGFunction {
	return func(L *lua.LState) int {
		L.RaiseError("%s: not configured", ns)
		return 0
	}
}

func buildKVTable(L *lua.LState, pluginID string, kv *KV) *lua.LTable {
	t := L.NewTable()
	if kv == nil || pluginID == "" {
		for _, name := range []string{"get", "set", "delete", "keys"} {
			t.RawSetString(name, L.NewFunction(nilGuard("kv")))
		}
		return t
	}

	t.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		v, ok, err := kv.Get(pluginID, key)
		if err != nil {
			L.RaiseError("kv.get: %v", err)
			return 0
		}
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLua(L, v))
		return 1
	}))
	t.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := luaToGo(L.Get(2))
		if err := kv.Set(pluginID, key, val); err != nil {
			L.RaiseError("kv.set: %v", err)
		}
		return 0
	}))
	t.RawSetString("delete", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		if err := kv.Delete(pluginID, key); err != nil {
			L.RaiseError("kv.delete: %v", err)
		}
		return 0
	}))
	t.RawSetString("keys", L.NewFunction(func(L *lua.LState) int {
		keys, err := kv.Keys(pluginID)
		if err != nil {
			L.RaiseError("kv.keys: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, k := range keys {
			out.Append(lua.LString(k))
		}
		L.Push(out)
		return 1
	}))
	return t
}

func buildTemplateTable(L *lua.LState, t TemplateAccess) *lua.LTable {
	tbl := L.NewTable()
	if t == nil {
		tbl.RawSetString("list", L.NewFunction(nilGuard("template")))
		tbl.RawSetString("get", L.NewFunction(nilGuard("template")))
		return tbl
	}
	tbl.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		out := L.NewTable()
		for _, m := range t.ListTemplates() {
			out.Append(goToLua(L, m))
		}
		L.Push(out)
		return 1
	}))
	tbl.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		got, err := t.GetTemplate(name)
		if err != nil {
			L.RaiseError("template.get: %v", err)
			return 0
		}
		L.Push(goToLua(L, got))
		return 1
	}))
	return tbl
}

func buildCollectionTable(L *lua.LState, c CollectionAccess) *lua.LTable {
	tbl := L.NewTable()
	if c == nil {
		tbl.RawSetString("list", L.NewFunction(nilGuard("collection")))
		return tbl
	}
	tbl.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		rows, err := c.ListCollection(name)
		if err != nil {
			L.RaiseError("collection.list: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, r := range rows {
			out.Append(goToLua(L, r))
		}
		L.Push(out)
		return 1
	}))
	return tbl
}

func buildFormTable(L *lua.LState, f FormAccess) *lua.LTable {
	tbl := L.NewTable()
	if f == nil {
		for _, name := range []string{"load", "save"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("form")))
		}
		return tbl
	}
	tbl.RawSetString("load", L.NewFunction(func(L *lua.LState) int {
		tpl := L.CheckString(1)
		df := L.CheckString(2)
		got, err := f.LoadForm(tpl, df)
		if err != nil {
			L.RaiseError("form.load: %v", err)
			return 0
		}
		L.Push(goToLua(L, got))
		return 1
	}))
	tbl.RawSetString("save", L.NewFunction(func(L *lua.LState) int {
		tpl := L.CheckString(1)
		df := L.CheckString(2)
		raw := luaToGo(L.Get(3))
		data, ok := raw.(map[string]any)
		if !ok {
			L.RaiseError("form.save: data must be a table")
			return 0
		}
		if err := f.SaveForm(context.Background(), tpl, df, data); err != nil {
			L.RaiseError("form.save: %v", err)
		}
		return 0
	}))
	return tbl
}

func buildRenderTable(L *lua.LState, pluginID string, r RenderAccess, fm FMAccess) *lua.LTable {
	tbl := L.NewTable()
	if r == nil {
		for _, name := range []string{"markdown", "html", "frontmatter", "pluginBlock"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("render")))
		}
		return tbl
	}
	mk := func(fn func(string, string) (string, error), label string) lua.LGFunction {
		return func(L *lua.LState) int {
			tpl := L.CheckString(1)
			df := L.CheckString(2)
			out, err := fn(tpl, df)
			if err != nil {
				L.RaiseError("render.%s: %v", label, err)
				return 0
			}
			L.Push(lua.LString(out))
			return 1
		}
	}
	tbl.RawSetString("markdown", L.NewFunction(mk(r.RenderMarkdown, "markdown")))
	tbl.RawSetString("html", L.NewFunction(mk(r.RenderHTML, "html")))

	// render.frontmatter / render.pluginBlock compose render+parse; both nil-guard when FM access isn't wired.
	if fm == nil {
		tbl.RawSetString("frontmatter", L.NewFunction(nilGuard("render.frontmatter")))
		tbl.RawSetString("pluginBlock", L.NewFunction(nilGuard("render.pluginBlock")))
		return tbl
	}
	tbl.RawSetString("frontmatter", L.NewFunction(func(L *lua.LState) int {
		tpl := L.CheckString(1)
		df := L.CheckString(2)
		md, err := r.RenderMarkdown(tpl, df)
		if err != nil {
			L.RaiseError("render.frontmatter: %v", err)
			return 0
		}
		data, body, err := fm.Parse(md)
		if err != nil {
			L.RaiseError("render.frontmatter: %v", err)
			return 0
		}
		if data == nil {
			L.Push(lua.LNil)
		} else {
			L.Push(goToLua(L, data))
		}
		L.Push(lua.LString(body))
		return 2
	}))
	tbl.RawSetString("pluginBlock", L.NewFunction(func(L *lua.LState) int {
		tpl := L.CheckString(1)
		df := L.CheckString(2)
		md, err := r.RenderMarkdown(tpl, df)
		if err != nil {
			L.RaiseError("render.pluginBlock: %v", err)
			return 0
		}
		data, _, err := fm.Parse(md)
		if err != nil {
			L.RaiseError("render.pluginBlock: %v", err)
			return 0
		}
		if data == nil || pluginID == "" {
			L.Push(lua.LNil)
			return 1
		}
		plugins, ok := data["plugins"].(map[string]any)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		block, ok := plugins[pluginID]
		if !ok || block == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLua(L, block))
		return 1
	}))
	return tbl
}

// buildFMTable mounts formidable.fm.parse/build/pluginBlock. pluginBlock maps parsed `data` to data.plugins[<this id>];
// the id is captured at build time so plugins never hardcode their own id.
func buildFMTable(L *lua.LState, pluginID string, fm FMAccess) *lua.LTable {
	tbl := L.NewTable()
	if fm == nil {
		for _, name := range []string{"parse", "build", "pluginBlock"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("fm")))
		}
		return tbl
	}
	tbl.RawSetString("parse", L.NewFunction(func(L *lua.LState) int {
		md := L.CheckString(1)
		data, body, err := fm.Parse(md)
		if err != nil {
			L.RaiseError("fm.parse: %v", err)
			return 0
		}
		if data == nil {
			L.Push(lua.LNil)
		} else {
			L.Push(goToLua(L, data))
		}
		L.Push(lua.LString(body))
		return 2
	}))
	tbl.RawSetString("build", L.NewFunction(func(L *lua.LState) int {
		body := L.CheckString(2)
		var data map[string]any
		if v := L.Get(1); v.Type() == lua.LTTable {
			raw := luaToGo(v)
			if m, ok := raw.(map[string]any); ok {
				data = m
			}
		}
		L.Push(lua.LString(fm.Build(data, body)))
		return 1
	}))
	tbl.RawSetString("pluginBlock", L.NewFunction(func(L *lua.LState) int {
		if pluginID == "" {
			L.Push(lua.LNil)
			return 1
		}
		v := L.Get(1)
		if v.Type() != lua.LTTable {
			L.Push(lua.LNil)
			return 1
		}
		root, _ := v.(*lua.LTable)
		plugins := root.RawGetString("plugins")
		if plugins.Type() != lua.LTTable {
			L.Push(lua.LNil)
			return 1
		}
		ptbl, _ := plugins.(*lua.LTable)
		L.Push(ptbl.RawGetString(pluginID))
		return 1
	}))
	return tbl
}

// buildRunTable mounts formidable.run.bar/status/chart/options; each fires synchronously into its emitter, nil emitters raise "run: not configured".
func buildRunTable(L *lua.LState, barEmit RunBarEmitter, statusEmit RunStatusEmitter, chartEmit RunChartEmitter, optionsEmit RunOptionsEmitter) *lua.LTable {
	tbl := L.NewTable()
	if barEmit == nil {
		tbl.RawSetString("bar", L.NewFunction(nilGuard("run")))
	} else {
		tbl.RawSetString("bar", L.NewFunction(func(L *lua.LState) int {
			done := L.OptInt(1, 0)
			total := L.OptInt(2, 0)
			barEmit(RunBarEvent{Done: done, Total: total})
			return 0
		}))
	}
	if statusEmit == nil {
		tbl.RawSetString("status", L.NewFunction(nilGuard("run")))
	} else {
		tbl.RawSetString("status", L.NewFunction(func(L *lua.LState) int {
			text := L.OptString(1, "")
			statusEmit(RunStatusEvent{Text: text})
			return 0
		}))
	}
	if chartEmit == nil {
		tbl.RawSetString("chart", L.NewFunction(nilGuard("run")))
	} else {
		tbl.RawSetString("chart", L.NewFunction(func(L *lua.LState) int {
			spec, _ := luaToGo(L.CheckTable(1)).(map[string]any)
			chartEmit(RunChartEvent{Spec: spec})
			return 0
		}))
	}
	if optionsEmit == nil {
		tbl.RawSetString("options", L.NewFunction(nilGuard("run")))
	} else {
		tbl.RawSetString("options", L.NewFunction(func(L *lua.LState) int {
			field := L.CheckString(1)
			opts, _ := luaToGo(L.CheckTable(2)).([]any)
			optionsEmit(RunOptionsEvent{Field: field, Options: opts})
			return 0
		}))
	}
	return tbl
}

func buildFSTable(L *lua.LState, fs FSAccess) *lua.LTable {
	tbl := L.NewTable()
	if fs == nil {
		for _, name := range []string{"read", "write", "mkdir", "list", "exists", "copy", "remove"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("fs")))
		}
		return tbl
	}
	tbl.RawSetString("read", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		s, err := fs.Read(p)
		if err != nil {
			L.RaiseError("fs.read: %v", err)
			return 0
		}
		L.Push(lua.LString(s))
		return 1
	}))
	tbl.RawSetString("write", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		c := L.CheckString(2)
		if err := fs.Write(p, c); err != nil {
			L.RaiseError("fs.write: %v", err)
		}
		return 0
	}))
	tbl.RawSetString("mkdir", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		if err := fs.Mkdir(p); err != nil {
			L.RaiseError("fs.mkdir: %v", err)
		}
		return 0
	}))
	tbl.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		names, err := fs.List(p)
		if err != nil {
			L.RaiseError("fs.list: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, n := range names {
			out.Append(lua.LString(n))
		}
		L.Push(out)
		return 1
	}))
	tbl.RawSetString("exists", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		L.Push(lua.LBool(fs.Exists(p)))
		return 1
	}))
	tbl.RawSetString("copy", L.NewFunction(func(L *lua.LState) int {
		from := L.CheckString(1)
		to := L.CheckString(2)
		if err := fs.Copy(from, to); err != nil {
			L.RaiseError("fs.copy: %v", err)
		}
		return 0
	}))
	tbl.RawSetString("remove", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckString(1)
		if err := fs.Remove(p); err != nil {
			L.RaiseError("fs.remove: %v", err)
		}
		return 0
	}))
	return tbl
}

// buildStorageTable mounts storage.imageBytes(tpl, name): an 8-bit-clean Lua string (binary-safe) or nil when absent.
func buildStorageTable(L *lua.LState, s StorageAccess) *lua.LTable {
	tbl := L.NewTable()
	if s == nil {
		tbl.RawSetString("imageBytes", L.NewFunction(nilGuard("storage")))
		return tbl
	}
	tbl.RawSetString("imageBytes", L.NewFunction(func(L *lua.LState) int {
		tpl := L.CheckString(1)
		name := L.CheckString(2)
		b, err := s.ImageBytes(tpl, name)
		if err != nil {
			L.RaiseError("storage.imageBytes: %v", err)
			return 0
		}
		if b == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(b))
		return 1
	}))
	return tbl
}

// optIntArg reads an optional integer Lua arg at position n, returning nil when absent (e.g. omitted table-column index).
func optIntArg(L *lua.LState, n int) *int {
	if v := L.Get(n); v.Type() == lua.LTNumber {
		i := int(lua.LVAsNumber(v))
		return &i
	}
	return nil
}

func optFloatArg(L *lua.LState, n int) *float64 {
	if v := L.Get(n); v.Type() == lua.LTNumber {
		f := float64(lua.LVAsNumber(v))
		return &f
	}
	return nil
}

// buildStatsTable mounts stats.distribution/numeric/timeSeries.
func buildStatsTable(L *lua.LState, s StatsAccess) *lua.LTable {
	tbl := L.NewTable()
	if s == nil {
		for _, name := range []string{"distribution", "numeric", "timeSeries"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("stats")))
		}
		return tbl
	}
	tbl.RawSetString("distribution", L.NewFunction(func(L *lua.LState) int {
		out, err := s.Distribution(L.CheckString(1), L.CheckString(2), optIntArg(L, 3))
		if err != nil {
			L.RaiseError("stats.distribution: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	tbl.RawSetString("numeric", L.NewFunction(func(L *lua.LState) int {
		out, err := s.NumericStats(L.CheckString(1), L.CheckString(2), optIntArg(L, 3), optFloatArg(L, 4))
		if err != nil {
			L.RaiseError("stats.numeric: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	tbl.RawSetString("timeSeries", L.NewFunction(func(L *lua.LState) int {
		// Lua signature: timeSeries(tpl, field, period [, col]); col is arg 4, period arg 3.
		out, err := s.TimeSeries(L.CheckString(1), L.CheckString(2), optIntArg(L, 4), L.CheckString(3))
		if err != nil {
			L.RaiseError("stats.timeSeries: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	return tbl
}

// buildFacetsTable mounts formidable.facets.distribution / .cross /
// .totalForms.
func buildFacetsTable(L *lua.LState, f FacetStatsAccess) *lua.LTable {
	tbl := L.NewTable()
	if f == nil {
		for _, name := range []string{"distribution", "cross", "totalForms"} {
			tbl.RawSetString(name, L.NewFunction(nilGuard("facets")))
		}
		return tbl
	}
	tbl.RawSetString("distribution", L.NewFunction(func(L *lua.LState) int {
		out, err := f.FacetDistribution(L.CheckString(1), L.CheckString(2))
		if err != nil {
			L.RaiseError("facets.distribution: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	tbl.RawSetString("cross", L.NewFunction(func(L *lua.LState) int {
		out, err := f.FacetCross(L.CheckString(1), L.CheckString(2), L.CheckString(3))
		if err != nil {
			L.RaiseError("facets.cross: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	tbl.RawSetString("totalForms", L.NewFunction(func(L *lua.LState) int {
		n, err := f.TotalForms(L.CheckString(1))
		if err != nil {
			L.RaiseError("facets.totalForms: %v", err)
			return 0
		}
		L.Push(lua.LNumber(n))
		return 1
	}))
	return tbl
}

// buildStatisticalValue returns the formidable.statistical value: a table with list/eval/evalComposite plus a
// __call metatable so the legacy callable form `statistical(tpl, name)` keeps evaluating (pre-.list back-compat).
func buildStatisticalValue(L *lua.LState, a StatObjectAccess) lua.LValue {
	if a == nil {
		return L.NewFunction(nilGuard("statistical"))
	}
	eval := func(tpl, name string) func(*lua.LState) int {
		return func(L *lua.LState) int {
			out, err := a.EvaluateObject(tpl, name)
			if err != nil {
				L.RaiseError("statistical: %v", err)
				return 0
			}
			L.Push(goToLua(L, out))
			return 1
		}
	}
	tbl := L.NewTable()
	tbl.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		out, err := a.ListObjects(L.CheckString(1))
		if err != nil {
			L.RaiseError("statistical.list: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	tbl.RawSetString("eval", L.NewFunction(func(L *lua.LState) int {
		return eval(L.CheckString(1), L.CheckString(2))(L)
	}))
	tbl.RawSetString("evalComposite", L.NewFunction(func(L *lua.LState) int {
		out, err := a.EvaluateComposite(L.CheckString(1), L.CheckString(2))
		if err != nil {
			L.RaiseError("statistical.evalComposite: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	// __call receives self at arg 1, so tpl/name are args 2/3.
	mt := L.NewTable()
	mt.RawSetString("__call", L.NewFunction(func(L *lua.LState) int {
		return eval(L.CheckString(2), L.CheckString(3))(L)
	}))
	L.SetMetatable(tbl, mt)
	return tbl
}

// buildExecValue returns formidable.exec as a callable function (not a table), so `formidable.exec("git", {"status"})` works directly.
func buildExecValue(L *lua.LState, runner ExecRunner) lua.LValue {
	if runner == nil {
		return L.NewFunction(nilGuard("exec"))
	}
	return L.NewFunction(func(L *lua.LState) int {
		cmd := L.CheckString(1)
		argsTbl := L.OptTable(2, L.NewTable())
		args := []string{}
		argsTbl.ForEach(func(_, v lua.LValue) {
			args = append(args, lua.LVAsString(v))
		})
		opts := ExecOptions{}
		if optsTbl, ok := L.Get(3).(*lua.LTable); ok {
			if cwd, ok := optsTbl.RawGetString("cwd").(lua.LString); ok {
				opts.Cwd = string(cwd)
			}
			if envTbl, ok := optsTbl.RawGetString("env").(*lua.LTable); ok {
				opts.Env = map[string]string{}
				envTbl.ForEach(func(k, v lua.LValue) {
					opts.Env[lua.LVAsString(k)] = lua.LVAsString(v)
				})
			}
			if ms, ok := optsTbl.RawGetString("timeout_ms").(lua.LNumber); ok {
				opts.Timeout = time.Duration(int64(ms)) * time.Millisecond
			}
		}
		res, err := runner.Exec(cmd, args, opts)
		if err != nil {
			L.RaiseError("exec: %v", err)
			return 0
		}
		out := L.NewTable()
		out.RawSetString("stdout", lua.LString(res.Stdout))
		out.RawSetString("stderr", lua.LString(res.Stderr))
		out.RawSetString("exit", lua.LNumber(res.Exit))
		L.Push(out)
		return 1
	})
}

// buildI18nTable mounts formidable.i18n.t(key). msgs is already stripped of its `plugin.<id>.` prefix so keys match the i18n/<locale>.json shape.
// A missing key returns the key verbatim (same fallback as vue-i18n).
func buildI18nTable(L *lua.LState, msgs map[string]string) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("t", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		if v, ok := msgs[key]; ok {
			L.Push(lua.LString(v))
			return 1
		}
		L.Push(lua.LString(key))
		return 1
	}))
	return t
}

// buildAPITable mounts formidable.api.fetch when an HTTPClient is wired (else nil-guard); manifests with requires_internal_server also gate via Manager.Run.
func buildAPITable(L *lua.LState, client HTTPClient) *lua.LTable {
	t := L.NewTable()
	if client == nil {
		t.RawSetString("fetch", L.NewFunction(nilGuard("api")))
		return t
	}
	t.RawSetString("fetch", L.NewFunction(func(L *lua.LState) int {
		method := L.CheckString(1)
		path := L.CheckString(2)
		body := ""
		if v := L.Get(3); v.Type() == lua.LTString {
			body = lua.LVAsString(v)
		}
		var headers map[string]string
		if v, ok := L.Get(4).(*lua.LTable); ok {
			headers = map[string]string{}
			v.ForEach(func(k, v lua.LValue) {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			})
		}
		res, err := client.Fetch(method, path, body, headers)
		if err != nil {
			L.RaiseError("api.fetch: %v", err)
			return 0
		}
		out := L.NewTable()
		out.RawSetString("status", lua.LNumber(res.Status))
		out.RawSetString("body", lua.LString(res.Body))
		if len(res.Headers) > 0 {
			h := L.NewTable()
			for k, v := range res.Headers {
				h.RawSetString(k, lua.LString(v))
			}
			out.RawSetString("headers", h)
		}
		L.Push(out)
		return 1
	}))
	return t
}

// buildJSONTable mounts formidable.json.encode/decode (always available); round-trips through goToLua/luaToGo for consistent shape.
func buildJSONTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		v := L.Get(1)
		raw, err := json.Marshal(luaToGo(v))
		if err != nil {
			L.RaiseError("json.encode: %v", err)
			return 0
		}
		L.Push(lua.LString(raw))
		return 1
	}))
	t.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			L.RaiseError("json.decode: %v", err)
			return 0
		}
		L.Push(goToLua(L, v))
		return 1
	}))
	return t
}

// buildPathTable mounts formidable.path.join/stripExt. join uses path.Join (forward-slash, collapses doubles).
// stripExt removes an exact caller-supplied suffix (".yaml", ".meta.json") so multi-dot suffixes round-trip.
func buildPathTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("join", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		parts := make([]string, 0, top)
		for i := 1; i <= top; i++ {
			parts = append(parts, lua.LVAsString(L.Get(i)))
		}
		L.Push(lua.LString(path.Join(parts...)))
		return 1
	}))
	t.RawSetString("stripExt", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		ext := L.CheckString(2)
		L.Push(lua.LString(strings.TrimSuffix(s, ext)))
		return 1
	}))
	return t
}

// buildURLTable mounts formidable.url.encode/decode via net/url PathEscape/PathUnescape, matching the renderer's /api/images/<stem>/<name> encoding.
func buildURLTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		L.Push(lua.LString(url.PathEscape(s)))
		return 1
	}))
	t.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		s := L.CheckString(1)
		d, err := url.PathUnescape(s)
		if err != nil {
			L.RaiseError("url.decode: %v", err)
			return 0
		}
		L.Push(lua.LString(d))
		return 1
	}))
	return t
}

// buildPluginTable exposes the running plugin's own metadata at formidable.plugin. Unset fields come through as zero values (never nil) so authors skip nil-checks.
func buildPluginTable(L *lua.LState, info PluginInfo) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("id", lua.LString(info.ID))
	t.RawSetString("name", lua.LString(info.Name))
	t.RawSetString("version", lua.LString(info.Version))
	t.RawSetString("author", lua.LString(info.Author))
	t.RawSetString("description", lua.LString(info.Description))
	t.RawSetString("mode", lua.LString(info.Mode))
	t.RawSetString("command", lua.LString(info.Command))
	t.RawSetString("requires_internal_server", lua.LBool(info.RequiresInternalServer))
	t.RawSetString("debug", lua.LBool(info.Debug))
	// `form` is a 1-indexed Lua table of field definitions, or empty when the plugin has no form.json.
	if len(info.Form) == 0 {
		t.RawSetString("form", L.NewTable())
	} else {
		t.RawSetString("form", goToLua(L, info.Form))
	}
	return t
}

// buildClientTable mounts formidable.client.{list, resources, operations, items, fetch, call, snapshot} over the api-client store, plus exists / create when admin is wired.
// declared is Manifest.APIClients: a script reaches only those, so the manifest stays the complete list of what a plugin calls or creates.
func buildClientTable(L *lua.LState, ctx context.Context, c ClientAccess, admin ClientAdmin, declared []string) *lua.LTable {
	t := L.NewTable()
	defer mountClientAdmin(L, t, admin, declared)
	if c == nil {
		for _, name := range []string{"list", "resources", "operations", "items", "fetch", "call", "snapshot"} {
			t.RawSetString(name, L.NewFunction(nilGuard("client")))
		}
		return t
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// checkClient reads the client id at arg 1 and raises unless the manifest declares it.
	checkClient := func(L *lua.LState, fn string) string {
		id := L.CheckString(1)
		if !slices.Contains(declared, id) {
			L.RaiseError("client.%s: api-client %q is not listed under api_clients in plugin.json", fn, id)
		}
		return id
	}
	t.RawSetString("list", L.NewFunction(func(L *lua.LState) int {
		all, err := c.ListClients()
		if err != nil {
			L.RaiseError("client.list: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, row := range all {
			if id, _ := row["id"].(string); slices.Contains(declared, id) {
				out.Append(goToLua(L, row))
			}
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("resources", L.NewFunction(func(L *lua.LState) int {
		id := checkClient(L, "resources")
		rows, err := c.ClientResources(id)
		if err != nil {
			L.RaiseError("client.resources: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, row := range rows {
			out.Append(goToLua(L, row))
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("items", L.NewFunction(func(L *lua.LState) int {
		id := checkClient(L, "items")
		resource := L.CheckString(2)
		q, err := clientQueryArg(L.Get(3))
		if err != nil {
			L.RaiseError("client.items: %v", err)
			return 0
		}
		page, err := c.ClientItems(ctx, id, resource, q)
		if err != nil {
			L.RaiseError("client.items: %v", err)
			return 0
		}
		L.Push(goToLua(L, page))
		return 1
	}))
	t.RawSetString("fetch", L.NewFunction(func(L *lua.LState) int {
		id := checkClient(L, "fetch")
		resource := L.CheckString(2)
		itemID := L.CheckString(3)
		if strings.TrimSpace(itemID) == "" {
			L.RaiseError("client.fetch: no id to resolve")
			return 0
		}
		q, err := clientQueryArg(L.Get(4))
		if err != nil {
			L.RaiseError("client.fetch: %v", err)
			return 0
		}
		item, err := c.ClientFetch(ctx, id, resource, itemID, q)
		if err != nil {
			L.RaiseError("client.fetch: %v", err)
			return 0
		}
		L.Push(goToLua(L, item))
		return 1
	}))
	t.RawSetString("operations", L.NewFunction(func(L *lua.LState) int {
		id := checkClient(L, "operations")
		rows, err := c.ClientOperations(id)
		if err != nil {
			L.RaiseError("client.operations: %v", err)
			return 0
		}
		out := L.NewTable()
		for _, row := range rows {
			out.Append(goToLua(L, row))
		}
		L.Push(out)
		return 1
	}))
	// call(id, operation, params, body, content_type) reaches an operation straight from the document, so a client derived from a document alone is usable without any resource binding.
	// It sends whatever method the operation declares. body is optional: a table goes out as JSON (or as a form when that is all the operation accepts), a string verbatim.
	t.RawSetString("call", L.NewFunction(func(L *lua.LState) int {
		id := checkClient(L, "call")
		operation := L.CheckString(2)
		var params map[string]string
		if v := L.Get(3); v != lua.LNil {
			tbl, ok := v.(*lua.LTable)
			if !ok {
				L.RaiseError("client.call: params must be a table of name = value (or nil)")
				return 0
			}
			var err error
			if params, err = clientParams(tbl); err != nil {
				L.RaiseError("client.call: %v", err)
				return 0
			}
		}
		var body any
		switch v := L.Get(4); v.Type() {
		case lua.LTNil:
		case lua.LTTable, lua.LTString, lua.LTNumber, lua.LTBool:
			body = luaToGo(v)
		default:
			L.RaiseError("client.call: body must be a table, string, number or boolean")
			return 0
		}
		contentType := ""
		if v := L.Get(5); v != lua.LNil {
			s, ok := v.(lua.LString)
			if !ok {
				L.RaiseError("client.call: content type must be a string")
				return 0
			}
			contentType = string(s)
		}
		res, err := c.ClientCall(ctx, id, operation, params, body, contentType)
		if err != nil {
			L.RaiseError("client.call: %v", err)
			return 0
		}
		L.Push(goToLua(L, res))
		return 1
	}))
	// snapshot makes no remote call, so it names no client: it only shapes an item the script already holds.
	t.RawSetString("snapshot", L.NewFunction(func(L *lua.LState) int {
		tbl, ok := L.Get(1).(*lua.LTable)
		if !ok {
			L.RaiseError("client.snapshot: item must be a table from client.items or client.fetch")
			return 0
		}
		item, ok := tableToGo(tbl).(map[string]any)
		if !ok {
			L.RaiseError("client.snapshot: item must be a table from client.items or client.fetch")
			return 0
		}
		var keys []string
		if v := L.Get(2); v != lua.LNil {
			list, ok := v.(*lua.LTable)
			if !ok {
				L.RaiseError("client.snapshot: keys must be a list of field keys")
				return 0
			}
			keys = []string{}
			for i := 1; i <= list.Len(); i++ {
				name, ok := list.RawGetInt(i).(lua.LString)
				if !ok {
					L.RaiseError("client.snapshot: keys must be a list of field keys")
					return 0
				}
				keys = append(keys, string(name))
			}
		}
		snap, err := c.ClientSnapshot(item, keys)
		if err != nil {
			L.RaiseError("client.snapshot: %v", err)
			return 0
		}
		L.Push(goToLua(L, snap))
		return 1
	}))
	return t
}

// clientQueryArg reads the optional options table. An unknown key is an error, since a misspelt option would otherwise silently fetch something else.
func clientQueryArg(v lua.LValue) (ClientQuery, error) {
	var q ClientQuery
	if v == lua.LNil {
		return q, nil
	}
	opts, ok := v.(*lua.LTable)
	if !ok {
		return q, errors.New("options must be a table")
	}
	var err error
	fail := func(format string, args ...any) {
		if err == nil {
			err = fmt.Errorf(format, args...)
		}
	}
	count := func(name string, v lua.LValue) int {
		n, ok := v.(lua.LNumber)
		if !ok || float64(n) < 0 || float64(n) != float64(int(n)) {
			fail("option %q must be a whole number, zero or more", name)
			return 0
		}
		return int(n)
	}
	text := func(name string, v lua.LValue) string {
		s, ok := v.(lua.LString)
		if !ok {
			fail("option %q must be a string", name)
		}
		return string(s)
	}
	opts.ForEach(func(k, v lua.LValue) {
		switch key := lua.LVAsString(k); key {
		case "search":
			q.Search = text(key, v)
		case "cursor":
			q.Cursor = text(key, v)
		case "limit":
			q.Limit = count(key, v)
		case "offset":
			q.Offset = count(key, v)
		case "select":
			list, ok := v.(*lua.LTable)
			if !ok {
				fail(`option "select" must be a list of field keys`)
				return
			}
			for i := 1; i <= list.Len(); i++ {
				name, ok := list.RawGetInt(i).(lua.LString)
				if !ok {
					fail(`option "select" must be a list of field keys`)
					return
				}
				q.Select = append(q.Select, string(name))
			}
		case "params":
			params, ok := v.(*lua.LTable)
			if !ok {
				fail(`option "params" must be a table of name = value`)
				return
			}
			got, perr := clientParams(params)
			if perr != nil {
				fail("%v", perr)
				return
			}
			q.Params = got
		default:
			fail("unknown option %q", key)
		}
	})
	return q, err
}

// clientParams reads operation parameters: scalar values only, each sent as its string form.
func clientParams(t *lua.LTable) (map[string]string, error) {
	out := map[string]string{}
	var err error
	t.ForEach(func(k, v lua.LValue) {
		switch v.Type() {
		case lua.LTString, lua.LTNumber, lua.LTBool:
			out[lua.LVAsString(k)] = v.String()
		default:
			if err == nil {
				err = fmt.Errorf("param %q must be a string, number or boolean", lua.LVAsString(k))
			}
		}
	})
	return out, err
}

// mountClientAdmin adds formidable.client.exists(id) and create(id, opts). create is create-only and carries no credential:
// opts.spec names the OpenAPI document inside the plugin folder; name, base_url, auth, dialect, headers and resources are optional, and what is left out is read off the document.
// It returns true when the client was created and false when it already existed and was left untouched.
func mountClientAdmin(L *lua.LState, t *lua.LTable, admin ClientAdmin, declared []string) {
	if admin == nil {
		for _, name := range []string{"exists", "create"} {
			t.RawSetString(name, L.NewFunction(nilGuard("client")))
		}
		return
	}
	check := func(L *lua.LState, fn string) string {
		id := L.CheckString(1)
		if !slices.Contains(declared, id) {
			L.RaiseError("client.%s: api-client %q is not listed under api_clients in plugin.json", fn, id)
		}
		return id
	}
	t.RawSetString("exists", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(admin.Exists(check(L, "exists"))))
		return 1
	}))
	t.RawSetString("create", L.NewFunction(func(L *lua.LState) int {
		id := check(L, "create")
		tbl, ok := L.Get(2).(*lua.LTable)
		if !ok {
			L.RaiseError("client.create: options must be a table with at least spec")
			return 0
		}
		opts, ok := tableToGo(tbl).(map[string]any)
		if !ok {
			L.RaiseError("client.create: options must be a table with at least spec")
			return 0
		}
		created, err := admin.Create(id, opts)
		if err != nil {
			L.RaiseError("client.create: %v", err)
			return 0
		}
		L.Push(lua.LBool(created))
		return 1
	}))
}

// buildPDFTable mounts formidable.pdf.active() and formidable.pdf.export(template, datafile, opts).
// export writes the PDF to disk and returns its path, so a script reads or sends the file from there; nothing of the PDF passes through Lua.
func buildPDFTable(L *lua.LState, p PDFAccess) *lua.LTable {
	t := L.NewTable()
	if p == nil {
		for _, name := range []string{"active", "export"} {
			t.RawSetString(name, L.NewFunction(nilGuard("pdf")))
		}
		return t
	}
	t.RawSetString("active", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(p.PDFActive()))
		return 1
	}))
	t.RawSetString("export", L.NewFunction(func(L *lua.LState) int {
		template := L.CheckString(1)
		datafile := L.CheckString(2)
		if strings.TrimSpace(template) == "" || strings.TrimSpace(datafile) == "" {
			L.RaiseError("pdf.export: a template and a datafile are required")
			return 0
		}
		var opts map[string]any
		if v := L.Get(3); v != lua.LNil {
			tbl, ok := v.(*lua.LTable)
			if !ok {
				L.RaiseError("pdf.export: options must be a table")
				return 0
			}
			if opts, ok = tableToGo(tbl).(map[string]any); !ok {
				L.RaiseError("pdf.export: options must be a table of name = value")
				return 0
			}
		}
		out, err := p.ExportPDF(template, datafile, opts)
		if err != nil {
			L.RaiseError("pdf.export: %v", err)
			return 0
		}
		L.Push(goToLua(L, out))
		return 1
	}))
	return t
}
