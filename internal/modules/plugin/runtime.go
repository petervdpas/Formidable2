package plugin

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// newSandboxedState builds a Lua state with only the pure-Lua
// standard libraries (base, table, string, math) — no os, io,
// debug, package, or coroutine. Then it nils out the file/code
// loading globals (`load`, `loadstring`, `loadfile`, `dofile`,
// `require`) that base would otherwise expose.
//
// Allowlist over denylist: this keeps the sandbox stable when
// gopher-lua adds new libraries in future versions.
func newSandboxedState() *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	for _, pair := range []struct {
		name string
		open lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		if err := L.CallByParam(lua.P{
			Fn:      L.NewFunction(pair.open),
			NRet:    0,
			Protect: true,
		}, lua.LString(pair.name)); err != nil {
			// Sandbox setup is build-time, not user-input; a failure
			// here is a Formidable bug, surfaced loudly.
			panic(fmt.Sprintf("plugin sandbox init: %v", err))
		}
	}
	for _, name := range []string{
		"load", "loadstring", "loadfile", "dofile", "require",
	} {
		L.SetGlobal(name, lua.LNil)
	}
	return L
}

// runtimeDeps groups the bridges the `formidable` Lua table needs.
// Each access interface is optional — when nil, calling into that
// namespace from Lua raises a "<namespace>: not configured" error
// instead of silently doing nothing. This makes test failures
// loud and makes wiring gaps in app.go obvious.
type runtimeDeps struct {
	LogSink    *[]string
	PluginID   string
	KV         *KV
	Template   TemplateAccess
	Collection CollectionAccess
	Form       FormAccess
	Render     RenderAccess
	FS         FSAccess
	Exec       ExecRunner
}

// installFormidable mounts the `formidable` global table on an
// already-sandboxed LState. The script's first statement can
// already see `formidable.api_version`, `formidable.log.*`, and
// every other namespace whose access dep is wired in `deps`.
func installFormidable(L *lua.LState, deps runtimeDeps) {
	f := L.NewTable()
	f.RawSetString("api_version", lua.LNumber(LuaAPIVersion))
	f.RawSetString("log", buildLogTable(L, deps.LogSink))
	f.RawSetString("kv", buildKVTable(L, deps.PluginID, deps.KV))
	f.RawSetString("template", buildTemplateTable(L, deps.Template))
	f.RawSetString("collection", buildCollectionTable(L, deps.Collection))
	f.RawSetString("form", buildFormTable(L, deps.Form))
	f.RawSetString("render", buildRenderTable(L, deps.Render))
	f.RawSetString("fs", buildFSTable(L, deps.FS))
	f.RawSetString("exec", buildExecValue(L, deps.Exec))
	L.SetGlobal("formidable", f)
}

func buildLogTable(L *lua.LState, sink *[]string) *lua.LTable {
	log := L.NewTable()
	for _, level := range []string{"info", "warn", "error", "debug"} {
		lvl := level
		log.RawSetString(level, L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			parts := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				parts = append(parts, lua.LVAsString(L.Get(i)))
			}
			line := "[" + lvl + "] " + strings.Join(parts, " ")
			if sink != nil {
				*sink = append(*sink, line)
			}
			return 0
		}))
	}
	return log
}

// scriptOpts is the input bundle for runScript. Source is the Lua
// source text (typically the contents of main.lua); Fn is the
// global function to invoke; Arg is an optional Go-shaped value
// that becomes the function's single argument; the access fields
// populate the matching `formidable.*` namespaces.
type scriptOpts struct {
	Source string
	Fn     string
	Arg    any

	// Access deps — leave nil to disable a namespace; calls into
	// it from Lua then raise a "<namespace>: not configured" error.
	PluginID   string
	KV         *KV
	Template   TemplateAccess
	Collection CollectionAccess
	Form       FormAccess
	Render     RenderAccess
	FS         FSAccess
	Exec       ExecRunner
}

// runScript spawns a fresh sandboxed state, loads Source, calls
// the named global function, and returns the converted return
// value plus any log lines emitted during the call.
//
// Per-invocation state is the contract — there is no plugin-state
// leakage across calls. Persistent state must go through KV.
func runScript(opts scriptOpts) (RunResult, error) {
	L := newSandboxedState()
	defer L.Close()

	var logs []string
	installFormidable(L, runtimeDeps{
		LogSink:    &logs,
		PluginID:   opts.PluginID,
		KV:         opts.KV,
		Template:   opts.Template,
		Collection: opts.Collection,
		Form:       opts.Form,
		Render:     opts.Render,
		FS:         opts.FS,
		Exec:       opts.Exec,
	})

	if err := L.DoString(opts.Source); err != nil {
		return RunResult{LogLines: logs}, fmt.Errorf("plugin: load script: %w", err)
	}

	fn := L.GetGlobal(opts.Fn)
	if fn == lua.LNil {
		return RunResult{LogLines: logs}, fmt.Errorf("plugin: function '%s' not defined", opts.Fn)
	}

	args := []lua.LValue{}
	if opts.Arg != nil {
		args = append(args, goToLua(L, opts.Arg))
	}
	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, args...); err != nil {
		return RunResult{LogLines: logs}, fmt.Errorf("plugin: call: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)
	return RunResult{Value: luaToGo(ret), LogLines: logs}, nil
}
