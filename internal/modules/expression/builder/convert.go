package builder

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// Convert is a best-effort migrator from legacy sidebar_expression
// shapes into the new builder canonical form. Frontend triggers it
// only when Parse fails — the converted output is fed straight back
// into Parse → Compile so the dialog can edit a clean DSL.
//
// Handled legacy shapes:
//
//   - [ <X> | <outcome> ]    →   <outcome>     (old "pipe" form)
//   - [ <ternary> ]          →   <ternary>     (array-wrapped cascade)
//   - bare identifier `key`  →   F["key"]      (when key ∈ fields)
//   - bare "literal" in text →   L["literal"]  (inside text concat)
//
// Anything Convert can't recognise after the pre-pass surfaces an
// error so the user knows the source needs hand-editing rather than
// silently dropping pieces.
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

// unwrapPipeForm matches `[ <X> | <Y> ]` and returns <Y>. expr-lang's
// parser can't read the `|` (it parses pipe as `Y(X)` and expects an
// identifier on the right), so this transformation has to happen at
// the text level. In every legacy template we saw, <Y> is fully
// self-contained — references whatever fields it needs inside its
// text/classes — so dropping <X> is lossless for chip rendering.
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

// unwrapSingletonArray strips `[ X ]` to `X` when X has no top-level
// comma — i.e. when the array wrapping was decorative rather than a
// real list. Legacy sources wrapped a single ternary or outcome in
// `[...]` because old Formidable's runtime evaluated arrays as a
// per-row sidebar feed; the new engine expects a single SidebarItem
// expression and would CSV-stringify the array, producing garbage
// (engine.go:219-227).
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

// findTopLevelByte returns the index of the first occurrence of `ch`
// in s at bracket/paren/brace depth 0 and outside string literals.
// When skipDoubled is true, `||` (or any `XX` repeat) is treated as a
// different token — necessary for `|` since `||` is logical-or and
// must not split a pipe form.
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

// mutateConvert walks the AST rewriting two legacy idioms into the
// canonical F[]/L[]/O[] vocabulary. The inTextChain flag rides down
// through `+` BinaryNodes inside an outcome's `text:` pair so a
// bare string literal in `"prefix " + field` gets wrapped, while a
// string literal somewhere else (like a `==` RHS in a predicate) does
// NOT — those have to stay literal because Compile emits them
// verbatim in comparisons.
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
		// Leave the callee identifier alone — that's a helper name,
		// not a field reference. Arguments may contain field refs
		// (e.g. isExpiredAfter(<ref>, 30)) so recurse there.
		for i := range n.Arguments {
			mutateConvert(&n.Arguments[i], fieldKeys, false)
		}
	case *ast.MemberNode:
		// F["k"] / L["s"] / O["k"] are already canonical — don't
		// descend (we'd otherwise rewrite the IdentifierNode("F")).
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

// simplifyBoolCompare normalises legacy explicit boolean comparisons
// (`F["x"] == true`, `F["x"] != false`) into the bare/negated field
// reference the new builder's predicate parser expects. The parser
// rejects `F[..] == true` outright — boolean predicates compile to
// `F[..]` or `!F[..]`, never to a literal-bool comparison.
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
