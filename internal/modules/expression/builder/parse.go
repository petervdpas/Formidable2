package builder

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Parse turns Compile's output back into a Config; non-canonical sources error so the dialog can fall back.
// Compile -> Parse -> Compile identity is the design contract (parse_test.go).
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

// walkTopLevel turns each ConditionalNode into a rule (IDs r1, r2... in order) and the terminal else into Default.
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

// bracketAccess matches a <Namespace>["arg"] MemberNode (F/L/O) and returns (arg, true) on a namespace match.
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

// fieldKeyOf extracts the key from F["key"], the only field form Compile emits; bare ids/$env are rejected
// so stale expressions fail Parse and trigger the dialog's "couldn't load" flow instead of misinterpretation.
func fieldKeyOf(node ast.Node) (string, bool) {
	return bracketAccess(node, "F")
}

func parsePredicateClause(node ast.Node) ([]Predicate, error) {
	// Empty predicates compile to literal `true`.
	if bn, ok := node.(*ast.BoolNode); ok && bn.Value {
		return []Predicate{}, nil
	}

	// && is either a same-field enum-not-equals group or a cross-field AND of independent predicates.
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

	// || is an enum-equals multi-value predicate.
	if bn, ok := node.(*ast.BinaryNode); ok && bn.Operator == "||" {
		p, err := parseEnumEqualsGroup(flattenOr(bn))
		if err != nil {
			return nil, err
		}
		return []Predicate{p}, nil
	}

	p, err := parseSinglePredicate(node)
	if err != nil {
		return nil, err
	}
	return []Predicate{p}, nil
}

func parseSinglePredicate(node ast.Node) (Predicate, error) {
	// F["key"] -> boolean true.
	if key, ok := fieldKeyOf(node); ok {
		t := true
		return Predicate{Kind: KindBoolean, FieldKey: key, BoolValue: &t}, nil
	}

	// !<ref> -> boolean false.
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
			// Single-part round-trips through the legacy Text field; multi-part through Parts.
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

// mapKeyName accepts a quoted or bare map key; Compile emits bare, but both are valid expr-lang.
func mapKeyName(node ast.Node) (string, error) {
	if sn, ok := node.(*ast.StringNode); ok {
		return sn.Value, nil
	}
	if id, ok := node.(*ast.IdentifierNode); ok {
		return id.Value, nil
	}
	return "", fmt.Errorf("map key not string or identifier (got %T)", node)
}

// parseTextChain flattens a `+` chain of L/F/O accessors into ordered Parts; a non-accessor leaf errors.
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

// parseTextSource reads one text part, accepting only L (literal) / F (fieldValue) / O (fieldLabel),
// each optionally wrapped in the str() coercion Compile now emits (a bare accessor is the legacy form).
func parseTextSource(node ast.Node) (*TextSource, error) {
	node = unwrapStr(node)
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

// unwrapStr peels a str(<accessor>) call back to its single argument; a node that
// is not a str() call is returned unchanged (so legacy bare accessors still parse).
func unwrapStr(node ast.Node) ast.Node {
	cn, ok := node.(*ast.CallNode)
	if !ok {
		return node
	}
	id, ok := cn.Callee.(*ast.IdentifierNode)
	if !ok || id.Value != "str" || len(cn.Arguments) != 1 {
		return node
	}
	return cn.Arguments[0]
}

// flattenPlus returns the leaves of a `+` chain in source order (a non-`+` node yields one leaf).
func flattenPlus(n ast.Node) []ast.Node {
	bn, ok := n.(*ast.BinaryNode)
	if !ok || bn.Operator != "+" {
		return []ast.Node{n}
	}
	return append(flattenPlus(bn.Left), flattenPlus(bn.Right)...)
}

func numberValueOf(node ast.Node) (float64, bool) {
	if n, ok := node.(*ast.IntegerNode); ok {
		return float64(n.Value), true
	}
	if n, ok := node.(*ast.FloatNode); ok {
		return n.Value, true
	}
	// expr-lang parses -3 as UnaryNode("-", Integer(3)); peel one level of unary minus.
	if un, ok := node.(*ast.UnaryNode); ok && un.Operator == "-" {
		if v, ok := numberValueOf(un.Node); ok {
			return -v, true
		}
	}
	return 0, false
}
