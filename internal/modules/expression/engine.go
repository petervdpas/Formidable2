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

// fieldRefPatcher rewrites builder field refs into the env shape: F["key"] -> $env["key"], L["text"] -> "text".
// O[] is left untouched; it resolves at runtime against the per-record O map injected by Manager.EvaluateList.
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

// engine is the compile/evaluate primitive, owning the helper registry and a program cache keyed by source text.
type engine struct {
	helpers map[string]any // name -> Go function passed via expr.Env
	cache   sync.Map       // expression text -> *vm.Program
}

// newEngine wires the default helper set.
func newEngine() *engine {
	return &engine{helpers: builtinHelpers()}
}

// builtinHelpers returns the safe-helper map (everything except debug).
func builtinHelpers() map[string]any {
	return map[string]any{
		"isSimilar":        isSimilar,
		"typeOf":           typeOf,
		"today":            today,
		"isOverdue":        isOverdue,
		"isDueSoon":        isDueSoon,
		"isOverdueInDays":  isOverdueInDays,
		"isExpiredAfter":   isExpiredAfter,
		"isUpcomingBefore": isUpcomingBefore,
		"isFuture":         isFuture,
		"isToday":          isToday,
		"daysBetween":      daysBetween,
		"ageInDays":        ageInDays,
		"defaultText":      defaultText,
		"notEmpty":         notEmpty,
	}
}

// Compile parses src into a cached *vm.Program; AllowUndefinedVariables makes a missing record field
// resolve to nil rather than an "unknown identifier" rejection (only some records may populate a field).
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

// wrapHelper boxes a typed Go function as the func(...any) (any, error) shape expr.Function accepts,
// so helpers register with their natural signatures without a hand-written wrapper each.
func wrapHelper(fn any) func(args ...any) (any, error) {
	rv := reflect.ValueOf(fn)
	rt := rv.Type()
	return func(args ...any) (any, error) {
		in := make([]reflect.Value, len(args))
		for i, a := range args {
			if a == nil {
				// A nil (missing identifier) lands in the helper as the param type's zero value.
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

// paramTypeAt returns the i-th parameter's reflect.Type, accounting for variadic helpers.
func paramTypeAt(rt reflect.Type, i int) reflect.Type {
	if rt.IsVariadic() && i >= rt.NumIn()-1 {
		return rt.In(rt.NumIn() - 1).Elem()
	}
	if i >= rt.NumIn() {
		return reflect.TypeOf((*any)(nil)).Elem()
	}
	return rt.In(i)
}

// Evaluate compiles (or caches) src and runs it against ctx, returning a normalised Result (see normalize).
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

// EvaluateRaw is Evaluate without the Result normalisation: it returns the raw
// typed value the expression produced (number/string/bool/...). Formula fields
// need the scalar value, not a styled Result, so they evaluate through here.
func (e *engine) EvaluateRaw(src string, ctx map[string]any) (any, error) {
	prog, err := e.Compile(src)
	if err != nil {
		return nil, err
	}
	return expr.Run(prog, mergeHelpersInto(ctx, e.helpers))
}

// mergeHelpersInto combines ctx and helpers into a fresh map; ctx wins on collision (a user field shadows a helper).
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

// normalize converts a raw result into a Result; string/[]any/map are mapped, anything else is stringified.
// Map handling is permissive (unknown keys ignored, mismatches stringified) so an off expression degrades gracefully.
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
