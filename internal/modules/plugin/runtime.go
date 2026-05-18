package plugin

import (
	"context"
	"errors"
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
	LogSink     *[]string
	ToastSink   *[]ToastEvent
	RunBarOut   RunBarEmitter
	RunStatOut  RunStatusEmitter
	Ctx         context.Context
	PluginID    string
	Plugin      PluginInfo
	KV          *KV
	Template    TemplateAccess
	Collection  CollectionAccess
	Form        FormAccess
	Render      RenderAccess
	FM          FMAccess
	FS          FSAccess
	Exec        ExecRunner
	API         HTTPClient
}

// installFormidable mounts the `formidable` global table on an
// already-sandboxed LState. The script's first statement can
// already see `formidable.api_version`, `formidable.log.*`, and
// every other namespace whose access dep is wired in `deps`.
func installFormidable(L *lua.LState, deps runtimeDeps) {
	f := L.NewTable()
	f.RawSetString("api_version", lua.LNumber(LuaAPIVersion))
	f.RawSetString("log", buildLogTable(L, deps.LogSink))
	f.RawSetString("toast", buildToastTable(L, deps.ToastSink))
	f.RawSetString("json", buildJSONTable(L))
	f.RawSetString("api", buildAPITable(L, deps.API))
	f.RawSetString("kv", buildKVTable(L, deps.PluginID, deps.KV))
	f.RawSetString("plugin", buildPluginTable(L, deps.Plugin))
	f.RawSetString("template", buildTemplateTable(L, deps.Template))
	f.RawSetString("collection", buildCollectionTable(L, deps.Collection))
	f.RawSetString("form", buildFormTable(L, deps.Form))
	f.RawSetString("render", buildRenderTable(L, deps.PluginID, deps.Render, deps.FM))
	f.RawSetString("fm", buildFMTable(L, deps.PluginID, deps.FM))
	f.RawSetString("run", buildRunTable(L, deps.RunBarOut, deps.RunStatOut))
	f.RawSetString("fs", buildFSTable(L, deps.FS))
	// formidable.cancelled() — cheap predicate so plugins can poll
	// for user-requested Stop inside pcall-heavy loops. Necessary
	// because gopher-lua's context-cancel error IS catchable by pcall
	// (every per-item Go binding in wikiwonder is pcall'd for failure
	// isolation, which swallows the cancel). Plugins should call this
	// at the top of any long loop; it's a single ctx.Err() check.
	ctxRef := deps.Ctx
	f.RawSetString("cancelled", L.NewFunction(func(L *lua.LState) int {
		if ctxRef == nil {
			L.Push(lua.LFalse)
			return 1
		}
		L.Push(lua.LBool(ctxRef.Err() != nil))
		return 1
	}))
	f.RawSetString("exec", buildExecValue(L, deps.Exec))
	L.SetGlobal("formidable", f)
}

func buildToastTable(L *lua.LState, sink *[]ToastEvent) *lua.LTable {
	t := L.NewTable()
	for _, level := range []string{"info", "success", "warn", "error"} {
		lvl := level
		t.RawSetString(level, L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			parts := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				parts = append(parts, lua.LVAsString(L.Get(i)))
			}
			if sink != nil {
				*sink = append(*sink, ToastEvent{
					Level:   lvl,
					Message: strings.Join(parts, " "),
				})
			}
			return 0
		}))
	}
	return t
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
// populate the matching `formidable.*` namespaces. Ctx, when non-nil,
// is wired into the LState via L.SetContext — cancelling it aborts
// the running script at the next instruction boundary (gopher-lua
// raises a context-cancelled error which we map to ErrPluginCancelled).
type scriptOpts struct {
	Source string
	Fn     string
	Arg    any
	Ctx    context.Context

	// Access deps — leave nil to disable a namespace; calls into
	// it from Lua then raise a "<namespace>: not configured" error.
	PluginID    string
	Plugin      PluginInfo
	KV          *KV
	Template    TemplateAccess
	Collection  CollectionAccess
	Form        FormAccess
	Render      RenderAccess
	FM          FMAccess
	FS          FSAccess
	Exec        ExecRunner
	API         HTTPClient
	RunBarOut   RunBarEmitter
	RunStatOut  RunStatusEmitter
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
	if opts.Ctx != nil {
		L.SetContext(opts.Ctx)
	}

	var logs []string
	var toasts []ToastEvent
	installFormidable(L, runtimeDeps{
		LogSink:    &logs,
		ToastSink:  &toasts,
		RunBarOut:  opts.RunBarOut,
		RunStatOut: opts.RunStatOut,
		Ctx:        opts.Ctx,
		PluginID:    opts.PluginID,
		Plugin:      opts.Plugin,
		KV:          opts.KV,
		Template:    opts.Template,
		Collection:  opts.Collection,
		Form:        opts.Form,
		Render:      opts.Render,
		FM:          opts.FM,
		FS:          opts.FS,
		Exec:        opts.Exec,
		API:         opts.API,
	})

	if err := L.DoString(opts.Source); err != nil {
		return RunResult{LogLines: logs, Toasts: toasts}, mapCancelled(opts.Ctx, fmt.Errorf("plugin: load script: %w", err))
	}

	fn := L.GetGlobal(opts.Fn)
	if fn == lua.LNil {
		return RunResult{LogLines: logs, Toasts: toasts}, fmt.Errorf("plugin: function '%s' not defined", opts.Fn)
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
		return RunResult{LogLines: logs, Toasts: toasts}, mapCancelled(opts.Ctx, fmt.Errorf("plugin: call: %w", err))
	}

	ret := L.Get(-1)
	L.Pop(1)
	// Post-call ctx check: pcall inside the script can catch
	// gopher-lua's context-cancel error and let the loop run to
	// completion. Even when the script "succeeded" — i.e.
	// CallByParam returned nil — if the run's context was cancelled
	// at any point we surface that as ErrPluginCancelled. The
	// frontend's kind="cancelled" branch then fires correctly.
	if opts.Ctx != nil && opts.Ctx.Err() != nil {
		return RunResult{LogLines: logs, Toasts: toasts}, ErrPluginCancelled
	}
	return RunResult{Value: luaToGo(ret), LogLines: logs, Toasts: toasts}, nil
}

// mapCancelled re-tags a gopher-lua error as ErrPluginCancelled when
// the run's context was cancelled. gopher-lua surfaces ctx-cancel as a
// generic runtime error inside L.DoString / CallByParam; without this
// translation the caller would have to string-match on the wrapped
// message to distinguish "user pressed Stop" from "buggy plugin".
func mapCancelled(ctx context.Context, err error) error {
	if ctx == nil || err == nil {
		return err
	}
	if cerr := ctx.Err(); cerr != nil && (errors.Is(cerr, context.Canceled) || errors.Is(cerr, context.DeadlineExceeded)) {
		return ErrPluginCancelled
	}
	return err
}
