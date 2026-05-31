package builder

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Convert best-effort migrates legacy sidebar_expression shapes (pipe form, array-wrapped cascade, bare
// identifiers/literals) into the canonical F/L/O form. Invoked only when Parse fails; unrecognised input errors.
func Convert(src string, fields []FieldRef) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", nil
	}

	src = unwrapPipeForm(src)
	src = unwrapSingletonArray(src)

	tree, err := parser.Parse(src)
	if err != nil {
		return "", fmt.Errorf("convert: parse: %w", err)
	}

	fkeys := make(map[string]bool, len(fields))
	for _, f := range fields {
		fkeys[f.Key] = true
	}

	mutateConvert(&tree.Node, fkeys, false)
	out := tree.Node.String()

	if cfg, err := Parse(out, fields); err == nil {
		if canon, err := Compile(cfg, fields); err == nil && canon != "" {
			return canon, nil
		}
	}
	return out, nil
}

// unwrapPipeForm matches [ <X> | <Y> ] and returns <Y> (a text-level pass since expr-lang can't read `|`);
// dropping <X> is lossless because legacy <Y> is self-contained.
func unwrapPipeForm(s string) string {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return s
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	idx := findTopLevelByte(inner, '|', true)
	if idx < 0 {
		return s
	}
	right := strings.TrimSpace(inner[idx+1:])
	return right
}

// unwrapSingletonArray strips [ X ] to X when X has no top-level comma; the new engine would CSV-stringify the array.
func unwrapSingletonArray(s string) string {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return s
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if findTopLevelByte(inner, ',', false) >= 0 {
		return s
	}
	return inner
}

// findTopLevelByte returns the first index of ch at bracket depth 0 outside strings; skipDoubled ignores XX runs
// (so `||` isn't mistaken for a pipe).
func findTopLevelByte(s string, ch byte, skipDoubled bool) int {
	depth := 0
	inStr := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
			continue
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			depth--
			continue
		}
		if depth != 0 || c != ch {
			continue
		}
		if skipDoubled {
			if i+1 < len(s) && s[i+1] == ch {
				i++
				continue
			}
			if i > 0 && s[i-1] == ch {
				continue
			}
		}
		return i
	}
	return -1
}

// mutateConvert rewrites bare identifiers/literals into F[]/L[]. inTextChain rides down a text: `+` chain so a
// literal in a text concat is wrapped, but a literal as a predicate `==` RHS stays literal.
func mutateConvert(np *ast.Node, fieldKeys map[string]bool, inTextChain bool) {
	switch n := (*np).(type) {
	case *ast.IdentifierNode:
		if fieldKeys[n.Value] {
			*np = &ast.MemberNode{
				Node:     &ast.IdentifierNode{Value: "F"},
				Property: &ast.StringNode{Value: n.Value},
			}
		}
	case *ast.StringNode:
		if inTextChain {
			*np = &ast.MemberNode{
				Node:     &ast.IdentifierNode{Value: "L"},
				Property: &ast.StringNode{Value: n.Value},
			}
		}
	case *ast.BinaryNode:
		childChain := inTextChain && n.Operator == "+"
		mutateConvert(&n.Left, fieldKeys, childChain)
		mutateConvert(&n.Right, fieldKeys, childChain)
		if rewritten, ok := simplifyBoolCompare(n); ok {
			*np = rewritten
		}
	case *ast.UnaryNode:
		mutateConvert(&n.Node, fieldKeys, inTextChain)
	case *ast.ConditionalNode:
		mutateConvert(&n.Cond, fieldKeys, false)
		mutateConvert(&n.Exp1, fieldKeys, inTextChain)
		mutateConvert(&n.Exp2, fieldKeys, inTextChain)
	case *ast.CallNode:
		// Callee is a helper name, not a field; recurse only into arguments.
		for i := range n.Arguments {
			mutateConvert(&n.Arguments[i], fieldKeys, false)
		}
	case *ast.MemberNode:
		// F/L/O are already canonical; don't descend (we'd rewrite the IdentifierNode("F")).
		if id, ok := n.Node.(*ast.IdentifierNode); ok {
			switch id.Value {
			case "F", "L", "O":
				return
			}
		}
		mutateConvert(&n.Node, fieldKeys, false)
	case *ast.ArrayNode:
		for i := range n.Nodes {
			mutateConvert(&n.Nodes[i], fieldKeys, inTextChain)
		}
	case *ast.MapNode:
		for _, p := range n.Pairs {
			pair, ok := p.(*ast.PairNode)
			if !ok {
				continue
			}
			isText := mapKeyEquals(pair.Key, "text")
			mutateConvert(&pair.Value, fieldKeys, isText)
		}
	}
}

// simplifyBoolCompare normalises F["x"] == true / != false into the bare/negated ref the predicate parser expects.
func simplifyBoolCompare(n *ast.BinaryNode) (ast.Node, bool) {
	if n.Operator != "==" && n.Operator != "!=" {
		return nil, false
	}
	bn, ok := n.Right.(*ast.BoolNode)
	if !ok {
		return nil, false
	}
	if _, ok := n.Left.(*ast.MemberNode); !ok {
		return nil, false
	}
	wantTrue := bn.Value
	if n.Operator == "!=" {
		wantTrue = !wantTrue
	}
	if wantTrue {
		return n.Left, true
	}
	return &ast.UnaryNode{Operator: "!", Node: n.Left}, true
}

func mapKeyEquals(node ast.Node, want string) bool {
	if s, ok := node.(*ast.StringNode); ok {
		return s.Value == want
	}
	if id, ok := node.(*ast.IdentifierNode); ok {
		return id.Value == want
	}
	return false
}
