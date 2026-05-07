package plugin

import (
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// ─────────────────────────────────────────────────────────────────
// lvalue conversion — Lua ↔ Go bridge.
// JSON-shaped values (string/number/bool/nil + arrays + maps) are
// the only things the runtime exposes across Wails to Vue, so the
// conversion only needs to handle that subset. Functions/userdata
// stringify to a sentinel; tests pin that down so future drift is
// loud.
// ─────────────────────────────────────────────────────────────────

func TestLuaToGo_Scalars(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cases := map[string]struct {
		lv   lua.LValue
		want any
	}{
		"nil":    {lua.LNil, nil},
		"true":   {lua.LBool(true), true},
		"false":  {lua.LBool(false), false},
		"int":    {lua.LNumber(42), float64(42)},
		"float":  {lua.LNumber(3.14), float64(3.14)},
		"string": {lua.LString("hi"), "hi"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := luaToGo(c.lv)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v (%T), want %v", got, got, c.want)
			}
		})
	}
}

func TestLuaToGo_ArrayTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	tbl := L.NewTable()
	tbl.Append(lua.LString("a"))
	tbl.Append(lua.LString("b"))
	tbl.Append(lua.LString("c"))
	got := luaToGo(tbl)
	want := []any{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLuaToGo_MapTable(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString("Alice"))
	tbl.RawSetString("age", lua.LNumber(30))
	got := luaToGo(tbl).(map[string]any)
	if got["name"] != "Alice" || got["age"] != float64(30) {
		t.Fatalf("got %v", got)
	}
}

func TestLuaToGo_MixedKeysFlattenToMap(t *testing.T) {
	// Lua tables can mix array and hash parts. We pick "object"
	// when any key is non-numeric or numeric keys are sparse;
	// integer keys get stringified. Keeps the conversion total —
	// scripts can't crash the bridge with a weird table shape.
	L := lua.NewState()
	defer L.Close()
	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LString("first"))
	tbl.RawSetString("name", lua.LString("Alice"))
	got := luaToGo(tbl).(map[string]any)
	if got["1"] != "first" || got["name"] != "Alice" {
		t.Fatalf("got %v", got)
	}
}

func TestLuaToGo_NestedTables(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	if err := L.DoString(`
		result = {
			name = "Alice",
			tags = {"a", "b"},
			meta = { active = true, count = 7 }
		}
	`); err != nil {
		t.Fatalf("doString: %v", err)
	}
	got := luaToGo(L.GetGlobal("result")).(map[string]any)
	if got["name"] != "Alice" {
		t.Fatalf("name: %v", got["name"])
	}
	tags := got["tags"].([]any)
	if len(tags) != 2 || tags[0] != "a" {
		t.Fatalf("tags: %v", tags)
	}
	meta := got["meta"].(map[string]any)
	if meta["active"] != true || meta["count"] != float64(7) {
		t.Fatalf("meta: %v", meta)
	}
}

func TestGoToLua_Roundtrip(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	original := map[string]any{
		"name": "Alice",
		"tags": []any{"a", "b"},
		"meta": map[string]any{"active": true, "count": float64(7)},
	}
	lv := goToLua(L, original)
	got := luaToGo(lv)
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("roundtrip drift:\n  got  %#v\n  want %#v", got, original)
	}
}

// ─────────────────────────────────────────────────────────────────
// Sandbox — the Lua VM must not be able to read/write outside the
// app's controlled bindings. These tests are the contract.
// ─────────────────────────────────────────────────────────────────

func TestSandbox_StripsDangerousGlobals(t *testing.T) {
	L := newSandboxedState()
	defer L.Close()
	for _, name := range []string{"os", "io", "debug"} {
		t.Run(name, func(t *testing.T) {
			if err := L.DoString(`assert(` + name + ` == nil)`); err != nil {
				t.Fatalf("%s leaked: %v", name, err)
			}
		})
	}
}

func TestSandbox_StripsPackageLoadlib(t *testing.T) {
	L := newSandboxedState()
	defer L.Close()
	if err := L.DoString(`
		assert(package == nil or package.loadlib == nil, "loadlib leaked")
		assert(package == nil or package.cpath == nil, "cpath leaked")
	`); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestSandbox_LeavesMathAndStringIntact(t *testing.T) {
	// Sandboxing should not break the standard pure-Lua libs —
	// plugin authors expect string/math/table to work normally.
	L := newSandboxedState()
	defer L.Close()
	if err := L.DoString(`
		assert(math.floor(3.9) == 3)
		assert(string.upper("abc") == "ABC")
		assert(table.concat({"a","b"}, ",") == "a,b")
	`); err != nil {
		t.Fatalf("standard libs broken: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────
// runScript — the entry point Manager.Run uses. Loads main.lua,
// calls a named global function with a Go-shaped argument, returns
// the result (Go-shaped) plus any log lines emitted via
// formidable.log.* during the call.
// ─────────────────────────────────────────────────────────────────

func TestRunScript_ReturnsValue(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run(ctx) return { ok = true, n = 42 } end`,
		Fn:     "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["ok"] != true || got["n"] != float64(42) {
		t.Fatalf("got %v", got)
	}
}

func TestRunScript_ScriptLoadError(t *testing.T) {
	_, err := runScript(scriptOpts{
		Source: `function run(ctx`,
		Fn:     "run",
	})
	if err == nil || !strings.Contains(err.Error(), "load") {
		t.Fatalf("want load-error, got %v", err)
	}
}

func TestRunScript_RuntimeError(t *testing.T) {
	_, err := runScript(scriptOpts{
		Source: `function run(ctx) error("boom") end`,
		Fn:     "run",
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want runtime error containing 'boom', got %v", err)
	}
}

func TestRunScript_MissingFunction(t *testing.T) {
	_, err := runScript(scriptOpts{
		Source: `local x = 1`,
		Fn:     "run",
	})
	if err == nil || !strings.Contains(err.Error(), "function 'run'") {
		t.Fatalf("want missing-function error, got %v", err)
	}
}

func TestRunScript_PassesContextArgument(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function echo(ctx) return ctx end`,
		Fn:     "echo",
		Arg:    map[string]any{"hello": "world"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["hello"] != "world" {
		t.Fatalf("got %v", got)
	}
}

func TestRunScript_APIVersionExposed(t *testing.T) {
	res, _ := runScript(scriptOpts{
		Source: `function v() return formidable.api_version end`,
		Fn:     "v",
	})
	if res.Value != float64(LuaAPIVersion) {
		t.Fatalf("api_version mismatch: got %v want %v", res.Value, LuaAPIVersion)
	}
}

func TestRunScript_LogLinesCaptured(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			formidable.log.info("hello")
			formidable.log.warn("careful")
			return true
		end`,
		Fn: "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.LogLines) != 2 {
		t.Fatalf("logs: %v", res.LogLines)
	}
	if !strings.Contains(res.LogLines[0], "hello") || !strings.HasPrefix(res.LogLines[0], "[info]") {
		t.Fatalf("logs[0]: %q", res.LogLines[0])
	}
	if !strings.HasPrefix(res.LogLines[1], "[warn]") {
		t.Fatalf("logs[1]: %q", res.LogLines[1])
	}
}
