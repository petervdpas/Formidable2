package render

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/aymerick/raymond/ast"
	"github.com/aymerick/raymond/parser"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Diagnostic is one finding from ValidateMarkdownTemplate. Errors are
// fatal (template won't render); warnings flag suspicious things like
// unknown helper names that would silently render to nothing.
type Diagnostic struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
	Helper   string `json:"helper,omitempty"`
}

// ValidationReport is the result of ValidateMarkdownTemplate. OK is
// true exactly when no error-severity diagnostics were found.
// Diagnostics is never nil so JS callers can iterate without a guard.
type ValidationReport struct {
	OK          bool         `json:"ok"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// builtinBlockHelpers names Handlebars built-ins that aren't in
// Formidable's helper catalog but are always valid as block heads.
var builtinBlockHelpers = map[string]struct{}{
	"if": {}, "unless": {}, "each": {}, "with": {}, "lookup": {},
	"helperMissing": {}, "blockHelperMissing": {},
}

var parseErrorLineRe = regexp.MustCompile(`Parse error on line (\d+):`)

// ValidateMarkdownTemplate parses the Handlebars source and returns a
// report. Empty input returns OK with no diagnostics (matches
// RenderMarkdown's "No template defined." short-circuit).
func ValidateMarkdownTemplate(src string) ValidationReport {
	out := ValidationReport{OK: true, Diagnostics: []Diagnostic{}}
	if strings.TrimSpace(src) == "" {
		return out
	}

	program, err := parser.Parse(src)
	if err != nil {
		d := Diagnostic{Severity: SeverityError, Message: err.Error()}
		if m := parseErrorLineRe.FindStringSubmatch(err.Error()); len(m) == 2 {
			if n, conv := strconv.Atoi(m[1]); conv == nil {
				d.Line = n
			}
		}
		out.OK = false
		out.Diagnostics = append(out.Diagnostics, d)
		return out
	}
	if program == nil {
		return out
	}

	v := &helperLintVisitor{known: knownHelperNames(), seen: map[string]struct{}{}}
	program.Accept(v)
	if len(v.diags) > 0 {
		out.Diagnostics = append(out.Diagnostics, v.diags...)
	}
	return out
}

func knownHelperNames() map[string]struct{} {
	known := make(map[string]struct{}, len(builtinHelpers)+len(builtinBlockHelpers))
	for _, h := range builtinHelpers {
		known[h.Name] = struct{}{}
	}
	for k := range builtinBlockHelpers {
		known[k] = struct{}{}
	}
	return known
}

// helperLintVisitor walks the parsed AST and flags helper invocations
// whose name is not in the known set. "Helper invocation" is conservative
// on purpose: a bare `{{foo}}` with no params is left alone because it
// may legitimately be a field/context lookup. We flag only when the
// expression has params or a hash (so it can't be a passive lookup) or
// when it heads a block (so it must dispatch to a helper).
type helperLintVisitor struct {
	known map[string]struct{}
	diags []Diagnostic
	// seen dedupes "unknown helper X" warnings per validation run so a
	// typo repeated 10 times doesn't produce 10 panel rows.
	seen map[string]struct{}
}

func (v *helperLintVisitor) flag(name string, line int, asBlock bool) {
	if name == "" {
		return
	}
	if _, ok := v.known[name]; ok {
		return
	}
	if _, dup := v.seen[name]; dup {
		return
	}
	v.seen[name] = struct{}{}
	msg := "Unknown helper '" + name + "'."
	if asBlock {
		msg = "Unknown block helper '" + name + "'."
	}
	v.diags = append(v.diags, Diagnostic{
		Severity: SeverityWarning,
		Message:  msg,
		Line:     line,
		Helper:   name,
	})
}

func (v *helperLintVisitor) visitParams(params []ast.Node) {
	for _, p := range params {
		if p != nil {
			p.Accept(v)
		}
	}
}

func (v *helperLintVisitor) VisitProgram(node *ast.Program) any {
	for _, n := range node.Body {
		n.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitMustache(node *ast.MustacheStatement) any {
	if node.Expression != nil {
		expr := node.Expression
		if len(expr.Params) > 0 || expr.Hash != nil {
			v.flag(expr.HelperName(), expr.Line, false)
		}
		expr.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitBlock(node *ast.BlockStatement) any {
	if node.Expression != nil {
		v.flag(node.Expression.HelperName(), node.Expression.Line, true)
		node.Expression.Accept(v)
	}
	if node.Program != nil {
		node.Program.Accept(v)
	}
	if node.Inverse != nil {
		node.Inverse.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitPartial(node *ast.PartialStatement) any {
	if node.Name != nil {
		node.Name.Accept(v)
	}
	v.visitParams(node.Params)
	if node.Hash != nil {
		node.Hash.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitContent(node *ast.ContentStatement) any { return nil }
func (v *helperLintVisitor) VisitComment(node *ast.CommentStatement) any { return nil }

func (v *helperLintVisitor) VisitExpression(node *ast.Expression) any {
	v.visitParams(node.Params)
	if node.Hash != nil {
		node.Hash.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitSubExpression(node *ast.SubExpression) any {
	if node.Expression != nil {
		v.flag(node.Expression.HelperName(), node.Expression.Line, false)
		node.Expression.Accept(v)
	}
	return nil
}

func (v *helperLintVisitor) VisitPath(node *ast.PathExpression) any     { return nil }
func (v *helperLintVisitor) VisitString(node *ast.StringLiteral) any    { return nil }
func (v *helperLintVisitor) VisitBoolean(node *ast.BooleanLiteral) any  { return nil }
func (v *helperLintVisitor) VisitNumber(node *ast.NumberLiteral) any    { return nil }

func (v *helperLintVisitor) VisitHash(node *ast.Hash) any {
	for _, p := range node.Pairs {
		if p != nil {
			p.Accept(v)
		}
	}
	return nil
}

func (v *helperLintVisitor) VisitHashPair(node *ast.HashPair) any {
	if node.Val != nil {
		node.Val.Accept(v)
	}
	return nil
}
