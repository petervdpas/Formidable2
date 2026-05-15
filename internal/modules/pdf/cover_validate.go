package pdf

import (
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
)

// CurrentCoverSchemaVersion is the schema version this build knows
// how to render. Bumped when the placeholder grammar or required
// markers change. Cover files declare their version in the magic
// comment; values higher than this build's are rejected by
// ValidateCover with `version-unsupported`.
const CurrentCoverSchemaVersion = 1

// CoverIssueSeverity gates whether ValidateCover.OK flips to false.
// Errors block render/save; warnings are advisory and surface in the
// UI as soft hints.
type CoverIssueSeverity string

const (
	CoverIssueError   CoverIssueSeverity = "error"
	CoverIssueWarning CoverIssueSeverity = "warning"
)

// CoverIssue is one finding from the validator. Codes are stable for
// the lifetime of CurrentCoverSchemaVersion so the frontend can pin
// translations / per-issue help.
type CoverIssue struct {
	Severity CoverIssueSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
}

// CoverTokenInfo carries the metadata parsed out of the magic comment.
// Name and Description are optional and purely informational — only
// Version participates in validation.
type CoverTokenInfo struct {
	Version     int    `json:"version"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// CoverValidation is the validator's structured result. OK is the
// errors.Is-style "should we proceed" gate; Issues carries every
// finding (warnings included) for the UI to display.
type CoverValidation struct {
	OK     bool            `json:"ok"`
	Token  *CoverTokenInfo `json:"token,omitempty"`
	Issues []CoverIssue    `json:"issues,omitempty"`
}

var (
	// leadingCommentRe captures the first HTML comment that appears
	// before any other non-whitespace content. The magic line must
	// live inside this comment.
	leadingCommentRe = regexp.MustCompile(`(?s)\A\s*<!--(.*?)-->`)
	magicLineRe      = regexp.MustCompile(`(?m)^\s*formidable-cover:\s*(\d+)\s*$`)
	nameLineRe       = regexp.MustCompile(`(?m)^\s*name:\s*(.+?)\s*$`)
	descLineRe       = regexp.MustCompile(`(?m)^\s*description:\s*(.+?)\s*$`)
	coverClassRe     = regexp.MustCompile(`class\s*=\s*["'][^"']*\bcover\b[^"']*["']`)
)

// ValidateCover inspects a cover-template HTML string and returns a
// structured verdict. The function is pure — it never touches the
// filesystem, never spawns processes, and never panics on malformed
// input.
//
// Used by:
//   - Loader (Manager.Export → ResolveCoverTemplateSet) — refuses
//     invalid covers at render time so a broken file in the on-disk
//     library can't produce a corrupt PDF.
//   - SaveCover (Wails service) — refuses to write covers that
//     wouldn't render, so the on-disk library never accumulates
//     known-bad files.
func ValidateCover(html string) CoverValidation {
	v := CoverValidation{OK: true}

	// 1. Magic-line check — presence is the verification token.
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

	// 2. data-cover-end sentinel — picoloom's pagination depends on
	// this marker. Without it cover/body boundary detection fails.
	if !strings.Contains(html, "data-cover-end") {
		v.OK = false
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueError, Code: "no-cover-end",
			Message: "Cover file must contain the `<span data-cover-end></span>` sentinel for picoloom pagination.",
		})
	}

	// 3. html/template parse — picoloom uses Go's html/template at
	// render time; if it can't parse, the render explodes.
	if _, err := template.New("cover").Parse(html); err != nil {
		v.OK = false
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueError, Code: "template-parse",
			Message: "Cover file does not parse as a Go html/template: " + err.Error(),
		})
	}

	// 4. Warnings — recoverable but worth flagging.
	if !strings.Contains(html, "{{.Title}}") {
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueWarning, Code: "no-title-placeholder",
			Message: "Cover file has no {{.Title}} placeholder — document titles will not appear on the cover.",
		})
	}
	if !coverClassRe.MatchString(html) {
		v.Issues = append(v.Issues, CoverIssue{
			Severity: CoverIssueWarning, Code: "no-cover-class",
			Message: "Cover file has no element with class=\"cover\" — theme CSS may not style it correctly.",
		})
	}

	return v
}
