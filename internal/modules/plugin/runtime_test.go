package plugin

import (
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// ─────────────────────────────────────────────────────────────────
// lvalue conversion - Lua ↔ Go bridge.
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
	// integer keys get stringified. Keeps the conversion total -
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
// Sandbox - the Lua VM must not be able to read/write outside the
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
	// Sandboxing should not break the standard pure-Lua libs -
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
// runScript - the entry point Manager.Run uses. Loads main.lua,
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

func TestRunScript_ToastEventsCollected(t *testing.T) {
	// The four toast levels (info/success/warn/error) must each
	// surface as a structured event in RunResult.Toasts so the
	// frontend can dispatch them through useToast verbatim.
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			formidable.toast.info("FYI")
			formidable.toast.success("Saved!")
			formidable.toast.warn("watch out")
			formidable.toast.error("Boom")
			return true
		end`,
		Fn: "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.Toasts) != 4 {
		t.Fatalf("got %d toasts, want 4: %+v", len(res.Toasts), res.Toasts)
	}
	want := []struct {
		level string
		msg   string
	}{
		{"info", "FYI"},
		{"success", "Saved!"},
		{"warn", "watch out"},
		{"error", "Boom"},
	}
	for i, w := range want {
		if res.Toasts[i].Level != w.level || res.Toasts[i].Message != w.msg {
			t.Fatalf("toast[%d] = %+v, want level=%q msg=%q",
				i, res.Toasts[i], w.level, w.msg)
		}
	}
}

// fakeAPI is the test stand-in for an APIAccess. running controls
// whether the precheck sees the server as up; calls records every
// Fetch invocation; resp is what Fetch returns. Mirrors the
// kvTestFS / OSFS pattern in this package.
type fakeAPI struct {
	running bool
	calls   []apiCall
	resp    apiResp
	err     error
}

type apiCall struct {
	method, path, body string
	headers            map[string]string
}

type apiResp struct {
	status  int
	body    string
	headers map[string]string
}

func (a *fakeAPI) IsAvailable() bool { return a.running }
func (a *fakeAPI) Fetch(method, path, body string, headers map[string]string) (HTTPResponse, error) {
	a.calls = append(a.calls, apiCall{method, path, body, headers})
	if a.err != nil {
		return HTTPResponse{}, a.err
	}
	return HTTPResponse{Status: a.resp.status, Body: a.resp.body, Headers: a.resp.headers}, nil
}

func TestRunScript_FormidableAPI_FetchRoundtrips(t *testing.T) {
	api := &fakeAPI{
		running: true,
		resp: apiResp{
			status:  200,
			body:    `{"ok":true}`,
			headers: map[string]string{"content-type": "application/json"},
		},
	}
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			local r = formidable.api.fetch("GET", "/api/templates", nil, nil)
			return { s = r.status, b = r.body }
		end`,
		Fn:  "run",
		API: api,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["s"] != float64(200) || got["b"] != `{"ok":true}` {
		t.Fatalf("got %+v", got)
	}
	if len(api.calls) != 1 || api.calls[0].method != "GET" || api.calls[0].path != "/api/templates" {
		t.Fatalf("calls: %+v", api.calls)
	}
}

func TestRunScript_FormidableAPI_NotConfiguredErrors(t *testing.T) {
	// When deps.API is nil, the api namespace is absent - calls
	// fail loudly so plugin authors notice they forgot the flag.
	_, err := runScript(scriptOpts{
		Source: `function run(ctx)
			return formidable.api.fetch("GET", "/x", nil, nil)
		end`,
		Fn: "run",
	})
	if err == nil {
		t.Fatal("expected error when api is not configured")
	}
}

func TestRunScript_FormidableJSON_EncodeDecodeRoundtrip(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			local enc = formidable.json.encode({ a = 1, b = "hi", c = { 10, 20 } })
			local dec = formidable.json.decode(enc)
			return dec
		end`,
		Fn: "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["a"] != float64(1) || got["b"] != "hi" {
		t.Fatalf("got %+v", got)
	}
	c := got["c"].([]any)
	if len(c) != 2 || c[0] != float64(10) {
		t.Fatalf("array c: %+v", c)
	}
}

func TestRunScript_FormidableJSON_DecodeBadStringErrors(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			local ok, err = pcall(function()
				formidable.json.decode("{not json")
			end)
			return { ok = ok, err = tostring(err) }
		end`,
		Fn: "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["ok"] != false {
		t.Fatalf("expected pcall to fail; got %+v", got)
	}
}

func TestRunScript_I18nT_ReturnsTranslatedValue(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run() return formidable.i18n.t("commands.run.label") end`,
		Fn:     "run",
		I18nMessages: map[string]string{
			"name":                 "Demo Plugin",
			"commands.run.label":   "Run it",
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "Run it" {
		t.Fatalf("got %v, want %q", res.Value, "Run it")
	}
}

func TestRunScript_I18nT_MissingKeyReturnsKey(t *testing.T) {
	res, err := runScript(scriptOpts{
		Source: `function run() return formidable.i18n.t("commands.nope.label") end`,
		Fn:     "run",
		I18nMessages: map[string]string{
			"name": "Demo Plugin",
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "commands.nope.label" {
		t.Fatalf("got %v, want literal key fallback", res.Value)
	}
}

func TestRunScript_I18nT_NoMessagesAtAllReturnsKey(t *testing.T) {
	// Plugin without an i18n/ folder yields nil/empty messages; t()
	// must still be callable and degrade to the literal key.
	res, err := runScript(scriptOpts{
		Source: `function run() return formidable.i18n.t("name") end`,
		Fn:     "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Value != "name" {
		t.Fatalf("got %v, want literal-key fallback when no messages", res.Value)
	}
}

func TestRunScript_ToastIgnoresExtraArgs(t *testing.T) {
	// Multiple positional args concat with a space, mirroring
	// formidable.log.* - keeps the API consistent.
	res, err := runScript(scriptOpts{
		Source: `function run(ctx)
			formidable.toast.success("hello", "world", 42)
			return true
		end`,
		Fn: "run",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(res.Toasts) != 1 {
		t.Fatalf("got %d toasts, want 1", len(res.Toasts))
	}
	if res.Toasts[0].Message != "hello world 42" {
		t.Fatalf("got %q", res.Toasts[0].Message)
	}
}
