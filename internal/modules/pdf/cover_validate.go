package pdf

import (
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
)

// CurrentCoverSchemaVersion is the cover schema version this build
// renders. Cover files declaring a higher version are rejected with
// `version-unsupported`.
const CurrentCoverSchemaVersion = 1

// CoverIssueSeverity gates whether ValidateCover.OK flips to false.
// Errors block render/save; warnings are advisory.
type CoverIssueSeverity string

const (
	CoverIssueError   CoverIssueSeverity = "error"
	CoverIssueWarning CoverIssueSeverity = "warning"
)

// CoverIssue is one finding from the validator. Codes are stable for
// the lifetime of CurrentCoverSchemaVersion so the frontend can pin
// translations.
type CoverIssue struct {
	Severity CoverIssueSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
}

// CoverTokenInfo carries metadata parsed from the magic comment. Only
// Version participates in validation; Name and Description are informational.
type CoverTokenInfo struct {
	Version     int    `json:"version"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// CoverValidation is the validator's result. OK is the "should we
// proceed" gate; Issues carries every finding (warnings included).
type CoverValidation struct {
	OK     bool            `json:"ok"`
	Token  *CoverTokenInfo `json:"token,omitempty"`
	Issues []CoverIssue    `json:"issues,omitempty"`
}

var (
	// leadingCommentRe captures the first HTML comment before any other
	// content; the magic line must live inside it.
	leadingCommentRe = regexp.MustCompile(`(?s)\A\s*<!--(.*?)-->`)
	magicLineRe      = regexp.MustCompile(`(?m)^\s*formidable-cover:\s*(\d+)\s*$`)
	nameLineRe       = regexp.MustCompile(`(?m)^\s*name:\s*(.+?)\s*$`)
	descLineRe       = regexp.MustCompile(`(?m)^\s*description:\s*(.+?)\s*$`)
	coverClassRe     = regexp.MustCompile(`class\s*=\s*["'][^"']*\bcover\b[^"']*["']`)
)

// ValidateCover inspects a cover-template HTML string and returns a
// structured verdict. Pure: no filesystem, no processes, no panic on
// malformed input.
func ValidateCover(html string) CoverValidation {
	v := CoverValidation{OK: true}

	leading := leadingCommentRe.FindStringSubmatch(html)
	if leading == nil {
		v.OK = false
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueError, Code: "no-magic-line",
			Message: "Cover file is missing the required leading HTML comment with `formidable-cover: <version>`.",
		})
	} else {
		commentBody := leading[1]
		magic := magicLineRe.FindStringSubmatch(commentBody)
		if magic == nil {
			v.OK = false
			v.Issues = append(v.Issues, CoverIssue{
				Severity: CoverIssueError, Code: "no-magic-line",
				Message: "Leading HTML comment does not contain `formidable-cover: <version>`.",
			})
		} else {
			ver, _ := strconv.Atoi(magic[1])
			v.Token = &CoverTokenInfo{Version: ver}
			if ver > CurrentCoverSchemaVersion {
				v.OK = false
				v.Issues = append(v.Issues, CoverIssue{
					Severity: CoverIssueError, Code: "version-unsupported",
					Message: fmt.Sprintf("Cover schema version %d not supported (this build understands %d).", ver, CurrentCoverSchemaVersion),
				})
			}
			if n := nameLineRe.FindStringSubmatch(commentBody); n != nil {
				v.Token.Name = strings.TrimSpace(n[1])
			}
			if d := descLineRe.FindStringSubmatch(commentBody); d != nil {
				v.Token.Description = strings.TrimSpace(d[1])
			}
		}
	}

	// data-cover-end sentinel: picoloom's pagination needs it for
	// cover/body boundary detection.
	if !strings.Contains(html, "data-cover-end") {
		v.OK = false
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueError, Code: "no-cover-end",
			Message: "Cover file must contain the `<span data-cover-end></span>` sentinel for picoloom pagination.",
		})
	}

	// picoloom renders the cover via html/template; a parse failure here
	// would otherwise explode at render time.
	if _, err := template.New("cover").Parse(html); err != nil {
		v.OK = false
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueError, Code: "template-parse",
			Message: "Cover file does not parse as a Go html/template: " + err.Error(),
		})
	}

	if !strings.Contains(html, "{{.Title}}") {
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueWarning, Code: "no-title-placeholder",
			Message: "Cover file has no {{.Title}} placeholder - document titles will not appear on the cover.",
		})
	}
	if !coverClassRe.MatchString(html) {
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueWarning, Code: "no-cover-class",
			Message: "Cover file has no element with class=\"cover\" - theme CSS may not style it correctly.",
		})
	}

	return v
}
