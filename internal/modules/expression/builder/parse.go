package builder

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Parse turns an expr-lang source string back into a Config the
// dialog can edit. It accepts only the AST shape Compile produces;
// hand-authored or otherwise-formatted expressions return an error
// so the dialog can fall back to an empty config (and warn the
// user that their existing source can't be edited visually).
//
// fields is the FieldRef slice for the template's expression-item
// fields — needed so we can pin a field's RuleKind for predicates
// (e.g. resolve a bare-identifier predicate as "boolean is true"
// instead of erroring on the missing comparison) and bound the
// ageInDays / option-label patterns to known fields.
//
// Round-trip identity (Compile → Parse → Compile == identity) is
// the design contract and is exercised by parse_test.go.
func Parse(src string, fields []FieldRef) (Config, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return DefaultConfig(), nil
	}

	tree, err := parser.Parse(src)
	if err != nil {
		return Config{}, fmt.Errorf("builder: parse: %w", err)
	}

	cfg := Config{Rules: []Rule{}, Default: Outcome{}}
	if err := walkTopLevel(tree.Node, &cfg); err != nil {
		return Config{}, fmt.Errorf("builder: %w", err)
	}
	return cfg, nil
}

// walkTopLevel processes the ternary chain. Each ConditionalNode
// becomes a rule; the terminal else (non-conditional) becomes the
// default outcome. Rule IDs are session-scoped — assigned r1, r2…
// in encountered order to match the frontend counter.
func walkTopLevel(node ast.Node, cfg *Config) error {
	cn, ok := node.(*ast.ConditionalNode)
	if !ok {
		out, err := parseOutcome(node)
		if err != nil {
			return fmt.Errorf("default outcome: %w", err)
		}
		cfg.Default = out
		return nil
	}

	preds, err := parsePredicateClause(cn.Cond)
	if err != nil {
		return fmt.Errorf("rule %d predicate: %w", len(cfg.Rules)+1, err)
	}
	out, err := parseOutcome(cn.Exp1)
	if err != nil {
		return fmt.Errorf("rule %d outcome: %w", len(cfg.Rules)+1, err)
	}
	cfg.Rules = append(cfg.Rules, Rule{
		ID:         fmt.Sprintf("r%d", len(cfg.Rules)+1),
		Predicates: preds,
		Outcome:    out,
	})
	return walkTopLevel(cn.Exp2, cfg)
}

// bracketAccess matches a `<Namespace>["arg"]` MemberNode shape and
// returns (arg, true) when the namespace identifier matches. Used
// for F[], L[], O[] — the three uniform accessors Compile emits.
// Anything else (bare identifiers, struct field access, dynamic
// indexing) returns (_, false).
func bracketAccess(node ast.Node, namespace string) (string, bool) {
	mn, ok := node.(*ast.MemberNode)
	if !ok {
		return "", false
	}
	id, ok := mn.Node.(*ast.IdentifierNode)
	if !ok || id.Value != namespace {
		return "", false
	}
	sn, ok := mn.Property.(*ast.StringNode)
	if !ok {
		return "", false
	}
	return sn.Value, true
}

// fieldKeyOf extracts the key from an `F["key"]` reference. F[] is
// the only field-value form Compile emits — bare identifiers and
// $env[] are intentionally rejected so hand-authored or stale
// expressions fail Parse and trigger the dialog's "couldn't load,
// cleared" flow rather than silently misinterpreting them.
func fieldKeyOf(node ast.Node) (string, bool) {
	return bracketAccess(node, "F")
}

// ── Predicate clause ────────────────────────────────────────────

func parsePredicateClause(node ast.Node) ([]Predicate, error) {
	// Empty predicates compile to literal `true`.
	if bn, ok := node.(*ast.BoolNode); ok && bn.Value {
		return []Predicate{}, nil
	}

	// && groups: either a single enum-not-equals-multi-value predicate
	// (`<field> != "a" && <field> != "b" …`) or a cross-field AND of
	// independent predicates. Disambiguation: same-field-and-all-!= →
	// enum predicate; otherwise per-leaf predicates.
	if bn, ok := node.(*ast.BinaryNode); ok && bn.Operator == "&&" {
		leaves := flattenAnd(bn)
		if p, err := parseEnumNotEqualsGroup(leaves); err == nil {
			return []Predicate{p}, nil
		}
		out := make([]Predicate, 0, len(leaves))
		for i, leaf := range leaves {
			p, err := parseSinglePredicate(leaf)
			if err != nil {
				return nil, fmt.Errorf("&& clause %d: %w", i+1, err)
			}
			out = append(out, p)
		}
		return out, nil
	}

	// || group: enum-equals-multi-value predicate.
	if bn, ok := node.(*ast.BinaryNode); ok && bn.Operator == "||" {
		p, err := parseEnumEqualsGroup(flattenOr(bn))
		if err != nil {
			return nil, err
		}
		return []Predicate{p}, nil
	}

	// Single predicate.
	p, err := parseSinglePredicate(node)
	if err != nil {
		return nil, err
	}
	return []Predicate{p}, nil
}

func parseSinglePredicate(node ast.Node) (Predicate, error) {
	// Bare identifier or $env-lookup → boolean rule with value=true.
	if key, ok := fieldKeyOf(node); ok {
		t := true
		return Predicate{Kind: KindBoolean, FieldKey: key, BoolValue: &t}, nil
	}

	// !<ref> → boolean rule with value=false.
	if un, ok := node.(*ast.UnaryNode); ok && un.Operator == "!" {
		key, ok := fieldKeyOf(un.Node)
		if !ok {
			return Predicate{}, fmt.Errorf("! applied to non-field-reference")
		}
		f := false
		return Predicate{Kind: KindBoolean, FieldKey: key, BoolValue: &f}, nil
	}

	// Binary comparison: enum / number / dateGt-dateLt.
	if bn, ok := node.(*ast.BinaryNode); ok {
		return parseBinaryPredicate(bn)
	}

	// Date helper call.
	if cn, ok := node.(*ast.CallNode); ok {
		return parseDateCall(cn)
	}

	return Predicate{}, fmt.Errorf("unrecognised predicate node %T", node)
}

func parseBinaryPredicate(bn *ast.BinaryNode) (Predicate, error) {
	op := bn.Operator

	// dateGt / dateLt: ageInDays(<ref>) > N or < N.
	if op == ">" || op == "<" {
		if call, ok := bn.Left.(*ast.CallNode); ok {
			if id, ok := call.Callee.(*ast.IdentifierNode); ok && id.Value == "ageInDays" {
				if len(call.Arguments) != 1 {
					return Predicate{}, fmt.Errorf("ageInDays needs exactly one arg")
				}
				fieldKey, ok := fieldKeyOf(call.Arguments[0])
				if !ok {
					return Predicate{}, fmt.Errorf("ageInDays arg not field reference")
				}
				argInt, ok := bn.Right.(*ast.IntegerNode)
				if !ok {
					return Predicate{}, fmt.Errorf("ageInDays comparison RHS not integer")
				}
				arg := argInt.Value
				dop := DateOpDateGt
				if op == "<" {
					dop = DateOpDateLt
				}
				return Predicate{Kind: KindDate, FieldKey: fieldKey, DateOp: dop, DateArg: &arg}, nil
			}
		}
	}

	// <ref> <op> <literal>
	key, ok := fieldKeyOf(bn.Left)
	if !ok {
		return Predicate{}, fmt.Errorf("binary LHS not field reference (op %q)", op)
	}

	if str, ok := bn.Right.(*ast.StringNode); ok {
		switch op {
		case "==":
			return Predicate{Kind: KindEnum, FieldKey: key, EnumOp: EnumOpEquals, EnumValues: []string{str.Value}}, nil
		case "!=":
			return Predicate{Kind: KindEnum, FieldKey: key, EnumOp: EnumOpNotEquals, EnumValues: []string{str.Value}}, nil
		}
	}
	if num, ok := numberValueOf(bn.Right); ok {
		var nop NumberOp
		switch op {
		case "==":
			nop = NumberOpEq
		case "!=":
			nop = NumberOpNe
		case ">":
			nop = NumberOpGt
		case ">=":
			nop = NumberOpGe
		case "<":
			nop = NumberOpLt
		case "<=":
			nop = NumberOpLe
		default:
			return Predicate{}, fmt.Errorf("unrecognised number op %q", op)
		}
		v := num
		return Predicate{Kind: KindNumber, FieldKey: key, NumberOp: nop, NumberValue: &v}, nil
	}
	return Predicate{}, fmt.Errorf("unrecognised binary predicate %q on %q", op, key)
}

func parseDateCall(cn *ast.CallNode) (Predicate, error) {
	id, ok := cn.Callee.(*ast.IdentifierNode)
	if !ok {
		return Predicate{}, fmt.Errorf("call callee not identifier")
	}
	op := DateOp(id.Value)
	switch op {
	case DateOpIsOverdue, DateOpIsToday, DateOpIsFuture,
		DateOpIsDueSoon, DateOpIsOverdueInDays,
		DateOpIsExpiredAfter, DateOpIsUpcomingBefore:
		// ok
	default:
		return Predicate{}, fmt.Errorf("unknown date helper %q", id.Value)
	}
	if len(cn.Arguments) < 1 || len(cn.Arguments) > 2 {
		return Predicate{}, fmt.Errorf("date helper %q has %d args (want 1 or 2)", id.Value, len(cn.Arguments))
	}
	fieldKey, ok := fieldKeyOf(cn.Arguments[0])
	if !ok {
		return Predicate{}, fmt.Errorf("date helper field arg not field reference")
	}
	p := Predicate{Kind: KindDate, FieldKey: fieldKey, DateOp: op}
	if len(cn.Arguments) == 2 {
		n, ok := cn.Arguments[1].(*ast.IntegerNode)
		if !ok {
			return Predicate{}, fmt.Errorf("date helper second arg not integer")
		}
		v := n.Value
		p.DateArg = &v
	}
	return p, nil
}

func parseEnumEqualsGroup(leaves []ast.Node) (Predicate, error) {
	if len(leaves) < 2 {
		return Predicate{}, fmt.Errorf("|| group has < 2 leaves")
	}
	var fieldKey string
	values := make([]string, 0, len(leaves))
	for i, leaf := range leaves {
		eb, ok := leaf.(*ast.BinaryNode)
		if !ok || eb.Operator != "==" {
			return Predicate{}, fmt.Errorf("|| leaf %d not ==", i)
		}
		key, ok := fieldKeyOf(eb.Left)
		if !ok {
			return Predicate{}, fmt.Errorf("|| leaf %d LHS not field reference", i)
		}
		str, ok := eb.Right.(*ast.StringNode)
		if !ok {
			return Predicate{}, fmt.Errorf("|| leaf %d RHS not string", i)
		}
		if i == 0 {
			fieldKey = key
		} else if key != fieldKey {
			return Predicate{}, fmt.Errorf("|| group has mixed fields")
		}
		values = append(values, str.Value)
	}
	return Predicate{Kind: KindEnum, FieldKey: fieldKey, EnumOp: EnumOpEquals, EnumValues: values}, nil
}

func parseEnumNotEqualsGroup(leaves []ast.Node) (Predicate, error) {
	if len(leaves) < 2 {
		return Predicate{}, fmt.Errorf("&& group has < 2 leaves")
	}
	var fieldKey string
	values := make([]string, 0, len(leaves))
	for i, leaf := range leaves {
		eb, ok := leaf.(*ast.BinaryNode)
		if !ok || eb.Operator != "!=" {
			return Predicate{}, fmt.Errorf("&& leaf %d not !=", i)
		}
		key, ok := fieldKeyOf(eb.Left)
		if !ok {
			return Predicate{}, fmt.Errorf("&& leaf %d LHS not field reference", i)
		}
		str, ok := eb.Right.(*ast.StringNode)
		if !ok {
			return Predicate{}, fmt.Errorf("&& leaf %d RHS not string", i)
		}
		if i == 0 {
			fieldKey = key
		} else if key != fieldKey {
			return Predicate{}, fmt.Errorf("&& group has mixed fields")
		}
		values = append(values, str.Value)
	}
	return Predicate{Kind: KindEnum, FieldKey: fieldKey, EnumOp: EnumOpNotEquals, EnumValues: values}, nil
}

func flattenOr(bn *ast.BinaryNode) []ast.Node {
	var out []ast.Node
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if b, ok := n.(*ast.BinaryNode); ok && b.Operator == "||" {
			walk(b.Left)
			walk(b.Right)
			return
		}
		out = append(out, n)
	}
	walk(bn)
	return out
}

func flattenAnd(bn *ast.BinaryNode) []ast.Node {
	var out []ast.Node
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if b, ok := n.(*ast.BinaryNode); ok && b.Operator == "&&" {
			walk(b.Left)
			walk(b.Right)
			return
		}
		out = append(out, n)
	}
	walk(bn)
	return out
}

// ── Outcome ─────────────────────────────────────────────────────

func parseOutcome(node ast.Node) (Outcome, error) {
	mn, ok := node.(*ast.MapNode)
	if !ok {
		return Outcome{}, fmt.Errorf("outcome not map literal (got %T)", node)
	}
	var out Outcome
	for i, p := range mn.Pairs {
		pair, ok := p.(*ast.PairNode)
		if !ok {
			return Outcome{}, fmt.Errorf("map entry %d not PairNode", i)
		}
		key, err := mapKeyName(pair.Key)
		if err != nil {
			return Outcome{}, fmt.Errorf("map entry %d: %w", i, err)
		}
		switch key {
		case "text":
			parts, err := parseTextChain(pair.Value)
			if err != nil {
				return Outcome{}, fmt.Errorf("text: %w", err)
			}
			// Single-part text round-trips through the legacy
			// `Text *TextSource` field so the existing single-source
			// dialog picker keeps working unchanged. Multi-part
			// concat round-trips through `Parts` and is preserved
			// verbatim for the eventual concat-aware UI.
			if len(parts) == 1 {
				ts := parts[0]
				out.Text = &ts
			} else if len(parts) > 0 {
				out.Parts = parts
			}
		case "color":
			sn, ok := pair.Value.(*ast.StringNode)
			if !ok {
				return Outcome{}, fmt.Errorf("color not string")
			}
			out.Color = sn.Value
		case "bg":
			sn, ok := pair.Value.(*ast.StringNode)
			if !ok {
				return Outcome{}, fmt.Errorf("bg not string")
			}
			out.Bg = sn.Value
		case "classes":
			arr, ok := pair.Value.(*ast.ArrayNode)
			if !ok {
				return Outcome{}, fmt.Errorf("classes not array")
			}
			for j, item := range arr.Nodes {
				sn, ok := item.(*ast.StringNode)
				if !ok {
					return Outcome{}, fmt.Errorf("class entry %d not string", j)
				}
				out.Classes = append(out.Classes, sn.Value)
			}
		default:
			return Outcome{}, fmt.Errorf("unknown outcome key %q", key)
		}
	}
	return out, nil
}

// mapKeyName accepts either a quoted "text" string key or a bare
// `text` identifier — Compile emits the latter, but both shapes are
// valid expr-lang and Parse accepts either to stay tolerant on the
// edge case where a hand-authored variant came via a similar route.
func mapKeyName(node ast.Node) (string, error) {
	if sn, ok := node.(*ast.StringNode); ok {
		return sn.Value, nil
	}
	if id, ok := node.(*ast.IdentifierNode); ok {
		return id.Value, nil
	}
	return "", fmt.Errorf("map key not string or identifier (got %T)", node)
}

// parseTextChain flattens a `+` chain of L/F/O accessors into the
// ordered Parts list. A single accessor (no `+`) yields a one-
// element slice. Anything that isn't an accessor — a number
// literal, a function call, a struct access — surfaces as an
// error so the dialog can fall back to its empty config.
func parseTextChain(node ast.Node) ([]TextSource, error) {
	leaves := flattenPlus(node)
	if len(leaves) > MaxConcatParts {
		return nil, fmt.Errorf("text has %d parts, max is %d", len(leaves), MaxConcatParts)
	}
	parts := make([]TextSource, 0, len(leaves))
	for i, leaf := range leaves {
		ts, err := parseTextSource(leaf)
		if err != nil {
			return nil, fmt.Errorf("part %d: %w", i+1, err)
		}
		parts = append(parts, *ts)
	}
	return parts, nil
}

// parseTextSource reads a single text part. Strict shape matching:
// only the three accessor forms Compile emits.
//
//	L["text"] → literal
//	F["key"]  → fieldValue
//	O["key"]  → fieldLabel
func parseTextSource(node ast.Node) (*TextSource, error) {
	if v, ok := bracketAccess(node, "L"); ok {
		return &TextSource{Kind: TextKindLiteral, Value: v}, nil
	}
	if k, ok := bracketAccess(node, "F"); ok {
		return &TextSource{Kind: TextKindFieldValue, FieldKey: k}, nil
	}
	if k, ok := bracketAccess(node, "O"); ok {
		return &TextSource{Kind: TextKindFieldLabel, FieldKey: k}, nil
	}
	return nil, fmt.Errorf("unrecognised text source %T", node)
}

// flattenPlus walks a left-leaning chain of `+` BinaryNodes and
// returns the leaves in source order. A non-`+` node returns as a
// single-leaf slice. We don't validate the leaf kinds here —
// callers (parseTextChain) check each leaf against the accessor
// vocabulary.
func flattenPlus(n ast.Node) []ast.Node {
	bn, ok := n.(*ast.BinaryNode)
	if !ok || bn.Operator != "+" {
		return []ast.Node{n}
	}
	return append(flattenPlus(bn.Left), flattenPlus(bn.Right)...)
}

// ── Number helpers ──────────────────────────────────────────────

func numberValueOf(node ast.Node) (float64, bool) {
	if n, ok := node.(*ast.IntegerNode); ok {
		return float64(n.Value), true
	}
	if n, ok := node.(*ast.FloatNode); ok {
		return n.Value, true
	}
	// expr-lang parses `-3` as UnaryNode("-", IntegerNode(3)), not as
	// a signed integer literal — peel one level of unary minus.
	if un, ok := node.(*ast.UnaryNode); ok && un.Operator == "-" {
		if v, ok := numberValueOf(un.Node); ok {
			return -v, true
		}
	}
	return 0, false
}
