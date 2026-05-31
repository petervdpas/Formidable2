package plugin

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

//go:embed builtins.lua
var builtinsLua string

// newSandboxedState builds a Lua state with only base/table/string/math (no os, io, debug, package, coroutine),
// then nils the file/code-loading globals (load, loadstring, loadfile, dofile, require) that base exposes.
// Allowlist over denylist keeps the sandbox stable when gopher-lua adds libraries.
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
			// Build-time setup, not user input: a failure here is a Formidable bug.
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

// runtimeDeps groups the bridges the `formidable` Lua table needs; a nil access interface raises "<namespace>: not configured" on use.
type runtimeDeps struct {
	LogSink       *[]string
	ToastSink     *[]ToastEvent
	RunBarOut     RunBarEmitter
	RunStatOut    RunStatusEmitter
	RunChartOut   RunChartEmitter
	RunOptionsOut RunOptionsEmitter
	Ctx           context.Context
	PluginID      string
	Plugin        PluginInfo
	KV            *KV
	Template      TemplateAccess
	Collection    CollectionAccess
	Form          FormAccess
	Render        RenderAccess
	FM            FMAccess
	FS            FSAccess
	Storage       StorageAccess
	Exec          ExecRunner
	API           HTTPClient
	Stats         StatsAccess
	Facets        FacetStatsAccess
	StatObject    StatObjectAccess
	// I18nMessages is the plugin's active-locale translations with the `plugin.<id>.` prefix stripped; nil/empty makes t() return the key verbatim.
	I18nMessages map[string]string
}

// installFormidable mounts the `formidable` global table on an already-sandboxed LState.
func installFormidable(L *lua.LState, deps runtimeDeps) {
	f := L.NewTable()
	f.RawSetString("api_version", lua.LNumber(LuaAPIVersion))
	f.RawSetString("log", buildLogTable(L, deps.LogSink))
	f.RawSetString("toast", buildToastTable(L, deps.ToastSink))
	f.RawSetString("json", buildJSONTable(L))
	f.RawSetString("path", buildPathTable(L))
	f.RawSetString("url", buildURLTable(L))
	f.RawSetString("api", buildAPITable(L, deps.API))
	f.RawSetString("kv", buildKVTable(L, deps.PluginID, deps.KV))
	f.RawSetString("plugin", buildPluginTable(L, deps.Plugin))
	f.RawSetString("template", buildTemplateTable(L, deps.Template))
	f.RawSetString("collection", buildCollectionTable(L, deps.Collection))
	f.RawSetString("form", buildFormTable(L, deps.Form))
	f.RawSetString("render", buildRenderTable(L, deps.PluginID, deps.Render, deps.FM))
	f.RawSetString("fm", buildFMTable(L, deps.PluginID, deps.FM))
	f.RawSetString("run", buildRunTable(L, deps.RunBarOut, deps.RunStatOut, deps.RunChartOut, deps.RunOptionsOut))
	f.RawSetString("storage", buildStorageTable(L, deps.Storage))
	f.RawSetString("stats", buildStatsTable(L, deps.Stats))
	f.RawSetString("facets", buildFacetsTable(L, deps.Facets))
	f.RawSetString("statistical", buildStatisticalValue(L, deps.StatObject))
	f.RawSetString("fs", buildFSTable(L, deps.FS))
	f.RawSetString("i18n", buildI18nTable(L, deps.I18nMessages))
	// formidable.cancelled() lets plugins poll for Stop inside pcall-heavy loops: gopher-lua's context-cancel error is catchable by pcall, which swallows it.
	ctxRef := deps.Ctx
	f.RawSetString("cancelled", L.NewFunction(func(L *lua.LState) int {
		if ctxRef == nil {
			L.Push(lua.LFalse)
			return 1
		}
		L.Push(lua.LBool(ctxRef.Err() != nil))
		return 1
	}))
	// formidable.sleep(seconds) is a context-aware pause: Stop wakes it immediately. Fractional seconds work; negative/zero is a no-op.
	f.RawSetString("sleep", L.NewFunction(func(L *lua.LState) int {
		sec := float64(L.OptNumber(1, 0))
		if sec <= 0 {
			return 0
		}
		d := time.Duration(sec * float64(time.Second))
		if ctxRef == nil {
			time.Sleep(d)
			return 0
		}
		select {
		case <-time.After(d):
		case <-ctxRef.Done():
		}
		return 0
	}))
	f.RawSetString("exec", buildExecValue(L, deps.Exec))
	L.SetGlobal("formidable", f)

	// Lua-side stdlib composes the Go namespaces. Skip on an already-cancelled context (Run-after-Cancel race); the post-run check surfaces it.
	if ctxRef != nil && ctxRef.Err() != nil {
		return
	}
	if err := L.DoString(builtinsLua); err != nil {
		// Cancel can race into DoString mid-script; treat a now-done context as expected cancellation, not a panic.
		if ctxRef != nil && ctxRef.Err() != nil {
			return
		}
		// A real builtins.lua failure is a Formidable bug (shipped in-tree): panic so it's loud in development.
		panic(fmt.Sprintf("plugin: builtins.lua install: %v", err))
	}
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

// scriptOpts is the input bundle for runScript. Fn is the global to invoke; Arg becomes its single argument.
// A non-nil Ctx is wired via L.SetContext; cancelling it aborts the script at the next instruction (mapped to ErrPluginCancelled).
type scriptOpts struct {
	Source string
	Fn     string
	Arg    any
	Ctx    context.Context

	// Access deps: nil disables a namespace (calls raise "<namespace>: not configured").
	PluginID      string
	Plugin        PluginInfo
	KV            *KV
	Template      TemplateAccess
	Collection    CollectionAccess
	Form          FormAccess
	Render        RenderAccess
	FM            FMAccess
	FS            FSAccess
	Storage       StorageAccess
	Exec          ExecRunner
	API           HTTPClient
	Stats         StatsAccess
	Facets        FacetStatsAccess
	StatObject    StatObjectAccess
	RunBarOut     RunBarEmitter
	RunStatOut    RunStatusEmitter
	RunChartOut   RunChartEmitter
	RunOptionsOut RunOptionsEmitter
	// I18nMessages: active-locale translations (prefix stripped); nil = none.
	I18nMessages map[string]string
}

// runScript spawns a fresh sandboxed state, loads Source, calls Fn, and returns the converted value plus log lines.
// State is per-invocation (no leakage across calls); persistent state must go through KV.
func runScript(opts scriptOpts) (RunResult, error) {
	L := newSandboxedState()
	defer L.Close()
	if opts.Ctx != nil {
		L.SetContext(opts.Ctx)
	}

	var logs []string
	var toasts []ToastEvent
	installFormidable(L, runtimeDeps{
		LogSink:       &logs,
		ToastSink:     &toasts,
		RunBarOut:     opts.RunBarOut,
		RunStatOut:    opts.RunStatOut,
		RunChartOut:   opts.RunChartOut,
		RunOptionsOut: opts.RunOptionsOut,
		Ctx:           opts.Ctx,
		PluginID:      opts.PluginID,
		Plugin:        opts.Plugin,
		KV:            opts.KV,
		Template:      opts.Template,
		Collection:    opts.Collection,
		Form:          opts.Form,
		Render:        opts.Render,
		FM:            opts.FM,
		FS:            opts.FS,
		Storage:       opts.Storage,
		Exec:          opts.Exec,
		API:           opts.API,
		Stats:         opts.Stats,
		Facets:        opts.Facets,
		StatObject:    opts.StatObject,
		I18nMessages:  opts.I18nMessages,
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
	// Post-call ctx check: an in-script pcall can catch the cancel error, so surface ErrPluginCancelled even when CallByParam returned nil.
	if opts.Ctx != nil && opts.Ctx.Err() != nil {
		return RunResult{LogLines: logs, Toasts: toasts}, ErrPluginCancelled
	}
	return RunResult{Value: luaToGo(ret), LogLines: logs, Toasts: toasts}, nil
}

// mapCancelled re-tags gopher-lua's generic ctx-cancel error as ErrPluginCancelled, so callers needn't string-match "Stop" vs "buggy plugin".
func mapCancelled(ctx context.Context, err error) error {
	if ctx == nil || err == nil {
		return err
	}
	if cerr := ctx.Err(); cerr != nil && (errors.Is(cerr, context.Canceled) || errors.Is(cerr, context.DeadlineExceeded)) {
		return ErrPluginCancelled
	}
	return err
}
