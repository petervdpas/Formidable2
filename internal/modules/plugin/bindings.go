package plugin

import (
	"context"
	"encoding/json"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ─────────────────────────────────────────────────────────────────
// Access interfaces — the surface formidable.* needs from the rest
// of the app. Implementations live in app.go (real, wired against
// template.Manager / dataprovider.Manager / render.Manager / *system.
// Manager / os/exec) and in bindings_test.go (mocks).
//
// Tight contract: each interface lists the *exact* methods Lua can
// reach, no more. New Lua surface = explicit interface change here.
// ─────────────────────────────────────────────────────────────────

// TemplateAccess is the formidable.template.* surface.
type TemplateAccess interface {
	ListTemplates() []map[string]any
	GetTemplate(filename string) (map[string]any, error)
}

// CollectionAccess is the formidable.collection.* surface.
type CollectionAccess interface {
	ListCollection(templateFilename string) ([]map[string]any, error)
}

// FormAccess is the formidable.form.* surface. SaveForm goes
// through the storage manager's atomic-write path so plugin
// writes never produce torn files.
type FormAccess interface {
	LoadForm(templateFilename, datafile string) (map[string]any, error)
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) error
}

// RenderAccess is the formidable.render.* surface — rendered
// markdown / HTML for a (template, datafile) pair.
type RenderAccess interface {
	RenderMarkdown(templateFilename, datafile string) (string, error)
	RenderHTML(templateFilename, datafile string) (string, error)
}

// FMAccess is the formidable.fm.* surface — YAML frontmatter
// parse/build helpers backed by the render module. Lua plugins
// can't safely YAML-parse on their own, and asking every plugin
// author to ship a parser is a sharp edge — so the operations
// the renderer already uses (ParseFrontmatter / BuildFrontmatter)
// are exposed verbatim. Parse returns (data, body) where body is
// the markdown with the leading `---…---` block removed; data is
// nil when no frontmatter was present. Build re-emits a leading
// frontmatter block from data and prepends it to body; nil/empty
// data returns body unchanged.
type FMAccess interface {
	Parse(markdown string) (map[string]any, string, error)
	Build(data map[string]any, body string) string
}

// FSAccess is the formidable.fs.* surface. v1 is unsandboxed —
// plugin authors are trusted (they wrote the plugin); the user
// reviewed it before installing into <AppRoot>/plugins/.
type FSAccess interface {
	Read(path string) (string, error)
	Write(path, content string) error
	Mkdir(path string) error
	List(path string) ([]string, error)
	Exists(path string) bool
}

// ExecOptions narrows what `formidable.exec(cmd, args, opts)`
// accepts. Cwd / Env / Timeout map to the corresponding os/exec
// fields. Keeping it a Go struct rather than a free-form map
// makes the contract impossible to drift.
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

// ExecRunner is the formidable.exec surface. Implementations:
//   - real: os/exec wrapper in exec.go (planned slice 7 wiring)
//   - test: mockExec in bindings_test.go
type ExecRunner interface {
	Exec(cmd string, args []string, opts ExecOptions) (ExecResult, error)
}

// ProgressEmitter receives one ProgressEvent per formidable.progress.tick
// call. Unlike log/toast (which are buffered for end-of-Run delivery),
// progress events fire synchronously into this callback during the run.
// Implementations in production wire to a Wails event dispatcher so the
// frontend can update a progress bar live; tests use a recording closure.
// Nil callback = drop events on the floor (progress namespace nil-guards
// in that case so a misconfigured runtime fails loudly).
type ProgressEmitter func(ProgressEvent)

// ─────────────────────────────────────────────────────────────────
// Namespace builders — each returns a Lua table that goes onto
// `formidable.*`. When the access dep is nil the table still gets
// installed but every call raises a clear "X: not configured"
// error so plugin authors learn what's missing immediately.
// ─────────────────────────────────────────────────────────────────

// nilGuard returns a closure that errors with `<ns>: not
// configured` when L's runtime didn't get the dep wired. Used by
// every namespace builder so the failure mode is uniform.
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

	// render.frontmatter / render.pluginBlock compose render+parse so
	// plugins don't have to repeat the two-step pattern. Both surfaces
	// degrade to nil-guards when FM access isn't wired so the failure
	// mode stays uniform across namespaces.
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

// buildFMTable mounts formidable.fm.parse / formidable.fm.build /
// formidable.fm.pluginBlock. Parse returns (data, body); data is a
// Lua table when frontmatter was present, nil when it wasn't.
// Build is the inverse: nil/empty data returns body unchanged so
// plugins can branch on "should I emit any frontmatter at all"
// cleanly. PluginBlock is the shorthand for the per-plugin FM
// convention: pass parsed `data` and get back data.plugins[<this
// plugin id>] or nil. The plugin id is captured at table-build
// time so plugins never hardcode their own id in the lookup.
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

// buildProgressTable mounts formidable.progress.tick(done, total, msg, stage).
// done and total are integers (gopher-lua coerces; OptInt defaults to 0
// when omitted). msg is the per-item label; stage is the optional
// section/phase label (e.g. current template stem during a per-template
// export) — the dialog renders stage prominently above msg so the
// user can see "which section am I in" at a glance. When the emitter
// is nil every call raises "progress: not configured" so
// misconfigurations surface immediately.
func buildProgressTable(L *lua.LState, emit ProgressEmitter) *lua.LTable {
	tbl := L.NewTable()
	if emit == nil {
		tbl.RawSetString("tick", L.NewFunction(nilGuard("progress")))
		return tbl
	}
	tbl.RawSetString("tick", L.NewFunction(func(L *lua.LState) int {
		done := L.OptInt(1, 0)
		total := L.OptInt(2, 0)
		msg := L.OptString(3, "")
		stage := L.OptString(4, "")
		emit(ProgressEvent{Done: done, Total: total, Stage: stage, Message: msg})
		return 0
	}))
	return tbl
}

func buildFSTable(L *lua.LState, fs FSAccess) *lua.LTable {
	tbl := L.NewTable()
	if fs == nil {
		for _, name := range []string{"read", "write", "mkdir", "list", "exists"} {
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
	return tbl
}

// buildExecValue returns the function value for `formidable.exec`.
// Note `exec` is a callable directly, not a table — that's how
// plugin authors expect shell-out to feel: `formidable.exec("git",
// {"status"})`. Returns a function (not a table) so calling
// `formidable.exec(...)` works without an extra .run lookup.
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

// buildAPITable mounts formidable.api.fetch when an HTTPClient is
// wired. With no client, the namespace exists but every call
// raises "api: not configured" — same shape as every other
// namespace nil-guard. Manifests that declare requires_internal_server
// also gate availability through a Run-time precheck (Manager.Run).
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

// buildJSONTable mounts formidable.json.encode/decode. Always
// available — pure utility, no host deps. Round-trips through
// goToLua / luaToGo so the same lvalue conversion the rest of the
// runtime uses governs shape.
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

// buildPluginTable exposes the running plugin's own metadata as
// a read-only Lua table at formidable.plugin. Fields that aren't
// set in PluginInfo come through as their zero values (empty
// string, false) — never nil — so plugin authors can sniff them
// without nil-checking. The table is a fresh per-invocation
// snapshot; mutating it from Lua is harmless and ignored.
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
	// `form` is a Lua table (1-indexed) of field definitions, or
	// an empty table when the plugin has no form.json. goToLua
	// recursively converts the Go shape so json.encode round-trips
	// it back to identical JSON.
	if len(info.Form) == 0 {
		t.RawSetString("form", L.NewTable())
	} else {
		t.RawSetString("form", goToLua(L, info.Form))
	}
	return t
}
