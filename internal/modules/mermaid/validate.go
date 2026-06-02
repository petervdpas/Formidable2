package mermaid

import (
	"regexp"
	"strconv"
	"strings"

	mc "github.com/sammcj/mermaid-check"
	"github.com/sammcj/mermaid-check/validator"
)

const (
	codeParseError = "parse_error"
	codeSemantic   = "semantic"
)

// canonicalType maps the parser's inconsistent type names onto stable canonical
// ones (it reports "graph" for graph-syntax flowcharts and the raw header for
// state). Anything absent passes through unchanged.
var canonicalType = map[string]string{
	"graph":           "flowchart",
	"stateDiagram-v2": "state",
}

// linePrefix lifts the "line N:" the parser embeds in its error message into a
// structured line number.
var linePrefix = regexp.MustCompile(`^line (\d+):\s*`)

// Validate parses Mermaid source and reports its diagram type plus any issues,
// using the vendored mermaid-check parser + semantic validators. Empty source
// is OK with no type. A parse failure yields a single positioned error issue;
// semantic findings carry their own line/column/severity.
func Validate(source string) Result {
	source, offset := stripFrontmatter(source)
	if strings.TrimSpace(source) == "" {
		return Result{OK: true}
	}

	diagram, err := mc.Parse(source)
	if err != nil {
		return Result{OK: false, Errors: []Issue{parseIssue(err.Error(), offset)}}
	}

	res := Result{OK: true, DiagramType: canonical(diagram.GetType())}
	for _, ve := range mc.Validate(diagram, false) {
		if ve.Severity == validator.SeverityError {
			res.OK = false
		}
		res.Errors = append(res.Errors, Issue{
			Line:     shift(ve.Line, offset),
			Col:      ve.Column,
			Code:     codeSemantic,
			Severity: severityName(ve.Severity),
			Message:  ve.Message,
		})
	}
	return res
}

func parseIssue(msg string, offset int) Issue {
	issue := Issue{Code: codeParseError, Severity: "error", Message: msg}
	if m := linePrefix.FindStringSubmatch(msg); m != nil {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil {
			issue.Line = shift(n, offset)
			issue.Message = strings.TrimSpace(msg[len(m[0]):])
		}
	}
	return issue
}

// shift maps a line number from frontmatter-stripped source back to the
// original. A 0 line (unpositioned) stays 0.
func shift(line, offset int) int {
	if line <= 0 {
		return line
	}
	return line + offset
}

func canonical(t string) string {
	if c, ok := canonicalType[t]; ok {
		return c
	}
	return t
}

func severityName(s validator.Severity) string {
	switch s {
	case validator.SeverityError:
		return "error"
	case validator.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

// stripFrontmatter removes a leading YAML frontmatter block (a "---" fence as
// the first non-blank line, through its closing "---") and returns the cleaned
// source plus the number of lines removed. The vendored parser rejects
// frontmatter that Mermaid itself accepts, and its line-sensitive sub-parsers
// choke on blank leading lines, so the block is deleted; the returned offset
// maps reported line numbers back onto the original source.
func stripFrontmatter(source string) (string, int) {
	lines := strings.Split(source, "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first == -1 || strings.TrimSpace(lines[first]) != "---" {
		return source, 0
	}
	for i := first + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n"), i + 1
		}
	}
	return source, 0
}
