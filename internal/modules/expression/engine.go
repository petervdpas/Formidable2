package expression

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
)

// fieldRefPatcher rewrites builder-emitted field references into
// the shape expr-lang evaluates against the env map.
//
//	F["key"]  →  $env["key"]
//	L["text"] →  "text"
//
// The builder emits F[] / L[] uniformly so concat chains have a
// predictable AST and so hyphenated keys round-trip identically to
// plain identifiers (no $env-vs-bare-id forking). O[] is left
// untouched — it resolves at runtime against the per-record `O`
// map injected by Manager.EvaluateList.
type fieldRefPatcher struct{}

func (fieldRefPatcher) Visit(node *ast.Node) {
	mn, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	id, ok := mn.Node.(*ast.IdentifierNode)
	if !ok {
		return
	}
	sn, ok := mn.Property.(*ast.StringNode)
	if !ok {
		return
	}
	switch id.Value {
	case "F":
		ast.Patch(node, &ast.MemberNode{
			Node:     &ast.IdentifierNode{Value: "$env"},
			Property: &ast.StringNode{Value: sn.Value},
		})
	case "L":
		ast.Patch(node, &ast.StringNode{Value: sn.Value})
	}
}

// engine is the low-level compile/evaluate primitive, owning the
// helper registry and a program cache keyed by expression text. The
// public Manager wraps it with template/storage-aware methods; tests
// exercise engine directly via newEngine() so they can pin behaviour
// without booting a full Manager.
type engine struct {
	helpers map[string]any // name → Go function passed via expr.Env
	cache   sync.Map       // expression text → *vm.Program
}

// newEngine wires the default helper set. Splitting helper
// registration from construction lets tests build a stripped engine
// later if we ever want to (today nothing does — the safe-helpers
// list is intentionally fixed).
func newEngine() *engine {
	return &engine{helpers: builtinHelpers()}
}

// builtinHelpers returns the default safe-helper map. Mirrors the
// `safe=true` set from the original controls/expressionHelpers.js
// (everything except `debug`). The keys are the names callers use in
// expressions; values are Go functions registered with expr.Env so
// expr-lang resolves them at compile time.
func builtinHelpers() map[string]any {
	return map[string]any{
		"isSimilar":         isSimilar,
		"typeOf":            typeOf,
		"today":             today,
		"isOverdue":         isOverdue,
		"isDueSoon":         isDueSoon,
		"isOverdueInDays":   isOverdueInDays,
		"isExpiredAfter":    isExpiredAfter,
		"isUpcomingBefore":  isUpcomingBefore,
		"isFuture":          isFuture,
		"isToday":           isToday,
		"daysBetween":       daysBetween,
		"ageInDays":         ageInDays,
		"defaultText":       defaultText,
		"notEmpty":          notEmpty,
	}
}

// Compile parses src and returns a cached *vm.Program. The same env
// shape (helpers + AllowUndefinedVariables) is used at compile and
// run time so a missing record field surfaces as nil instead of an
// "unknown identifier" rejection — matching what users expect when
// only some records have a given field populated.
func (e *engine) Compile(src string) (*vm.Program, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if cached, ok := e.cache.Load(src); ok {
		return cached.(*vm.Program), nil
	}

	opts := []expr.Option{
		expr.AllowUndefinedVariables(),
		expr.Patch(fieldRefPatcher{}),
	}
	for name, fn := range e.helpers {
		opts = append(opts, expr.Function(name, wrapHelper(fn)))
	}

	prog, err := expr.Compile(src, opts...)
	if err != nil {
		return nil, err
	}
	e.cache.Store(src, prog)
	return prog, nil
}

// wrapHelper boxes a strongly-typed Go function as the variadic
// `func(...any) (any, error)` shape that expr.Function accepts. This
// lets us register helpers with their natural Go signatures
// (`isOverdue(any) bool`) without writing a wrapper per helper. The
// reflect call sits on the call path but the program-cache means we
// only pay it once per actual invocation, not per Compile.
func wrapHelper(fn any) func(args ...any) (any, error) {
	rv := reflect.ValueOf(fn)
	rt := rv.Type()
	return func(args ...any) (any, error) {
		in := make([]reflect.Value, len(args))
		for i, a := range args {
			if a == nil {
				// Use the zero value of the parameter type so a
				// missing identifier (resolved to nil) lands in the
				// helper as its expected zero — matches JS coercion.
				pt := paramTypeAt(rt, i)
				in[i] = reflect.Zero(pt)
				continue
			}
			in[i] = reflect.ValueOf(a)
		}
		out := rv.Call(in)
		if len(out) == 0 {
			return nil, nil
		}
		return out[0].Interface(), nil
	}
}

// paramTypeAt returns the i-th parameter's reflect.Type, accounting
// for variadic helpers (none today, but stay flexible).
func paramTypeAt(rt reflect.Type, i int) reflect.Type {
	if rt.IsVariadic() && i >= rt.NumIn()-1 {
		return rt.In(rt.NumIn() - 1).Elem()
	}
	if i >= rt.NumIn() {
		return reflect.TypeOf((*any)(nil)).Elem()
	}
	return rt.In(i)
}

// Evaluate compiles src (or hits the cache) and runs it against ctx,
// returning a normalised Result. Three result shapes are
// recognised:
//
//   - string  → {Text: v}
//   - []any   → {Text: csv(v), Items: stringify(v)}
//   - map     → unmarshal known keys into Result; unknown keys
//     ignored so users can't smuggle garbage into the JSON envelope
//
// Anything else is stringified into Text via fmt.Sprint — keeps
// numeric and boolean returns useful without surprising the caller.
func (e *engine) Evaluate(src string, ctx map[string]any) (Result, error) {
	prog, err := e.Compile(src)
	if err != nil {
		return Result{}, err
	}
	env := mergeHelpersInto(ctx, e.helpers)
	raw, err := expr.Run(prog, env)
	if err != nil {
		return Result{}, err
	}
	return normalize(raw), nil
}

// mergeHelpersInto returns a fresh map combining ctx and helpers.
// AllowUndefinedVariables means missing identifiers fall through to
// nil; we still need helpers in the env so calls like `today()`
// resolve. ctx wins on collision so a user field named `today` would
// shadow the helper — surprising but consistent with the JS original
// where local sandbox bindings beat helper bindings.
func mergeHelpersInto(ctx map[string]any, helpers map[string]any) map[string]any {
	out := make(map[string]any, len(ctx)+len(helpers))
	for k, v := range helpers {
		out[k] = v
	}
	for k, v := range ctx {
		out[k] = v
	}
	return out
}

// normalize converts the raw evaluation result into a Result.
// Map handling is permissive: unknown keys ignored, type-mismatches
// stringified rather than erroring so a slightly-off expression
// degrades gracefully (sidebar shows a value, not an error pill).
func normalize(raw any) Result {
	if raw == nil {
		return Result{}
	}
	switch v := raw.(type) {
	case string:
		return Result{Text: v}
	case map[string]any:
		return mapToResult(v)
	case []any:
		strs := make([]string, len(v))
		for i, x := range v {
			strs[i] = fmt.Sprint(x)
		}
		return Result{Text: strings.Join(strs, ", "), Items: strs}
	default:
		return Result{Text: fmt.Sprint(v)}
	}
}

func mapToResult(m map[string]any) Result {
	var item Result
	if v, ok := m["text"].(string); ok {
		item.Text = v
	} else if v, ok := m["text"]; ok {
		item.Text = fmt.Sprint(v)
	}
	if v, ok := m["color"].(string); ok {
		item.Color = v
	}
	if v, ok := m["bg"].(string); ok {
		item.Bg = v
	}
	if v, ok := m["classes"].([]any); ok {
		for _, c := range v {
			if s, ok := c.(string); ok {
				item.Classes = append(item.Classes, s)
			}
		}
	}
	if v, ok := m["items"].([]any); ok {
		for _, x := range v {
			item.Items = append(item.Items, fmt.Sprint(x))
		}
	}
	return item
}
