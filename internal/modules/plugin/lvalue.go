package plugin

import (
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// luaToGo converts a Lua value to a JSON-shaped Go value:
// nil → nil, bool → bool, number → float64, string → string,
// table → []any when array-shaped, else map[string]any.
//
// Functions, userdata, threads, and channels are out-of-scope for
// the JSON envelope we ship to Vue and convert to nil. Plugin
// authors who try to return one of those see an empty result and
// can ask formidable.log why.
func luaToGo(lv lua.LValue) any {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		return tableToGo(v)
	default:
		return nil
	}
}

// tableToGo decides whether a Lua table is array-shaped (all keys
// are positive integers 1..N with no holes) or map-shaped, then
// converts accordingly. Mixed-shape tables (rare but legal in
// Lua) become maps with stringified keys so the conversion is
// total — script bugs can't crash the bridge.
func tableToGo(t *lua.LTable) any {
	hasStringKey := false
	t.ForEach(func(k, _ lua.LValue) {
		if _, ok := k.(lua.LString); ok {
			hasStringKey = true
		}
	})

	n := t.Len()
	if n > 0 && !hasStringKey {
		out := make([]any, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, luaToGo(t.RawGetInt(i)))
		}
		return out
	}

	out := map[string]any{}
	t.ForEach(func(k, v lua.LValue) {
		out[stringifyKey(k)] = luaToGo(v)
	})
	return out
}

func stringifyKey(k lua.LValue) string {
	switch v := k.(type) {
	case lua.LString:
		return string(v)
	case lua.LNumber:
		if float64(v) == float64(int(v)) {
			return strconv.Itoa(int(v))
		}
		return v.String()
	default:
		return k.String()
	}
}

// goToLua converts a JSON-shaped Go value back into a Lua value.
// Counterpart to luaToGo; used to push a context argument across
// to the script and to round-trip kv values.
func goToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case string:
		return lua.LString(x)
	case int:
		return lua.LNumber(x)
	case int32:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case float32:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	case []any:
		t := L.NewTable()
		for _, item := range x {
			t.Append(goToLua(L, item))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, val := range x {
			t.RawSetString(k, goToLua(L, val))
		}
		return t
	default:
		return lua.LNil
	}
}
