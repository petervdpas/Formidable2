// Package pdf owns Formidable's PDF export pipeline. The runtime
// engine is picoloom v2 (Go library, headless Chrome via go-rod) —
// see design/pdf-export.md for the full plan.
//
// The module is opt-in: until the user activates it from the
// Information workspace, every action returns ErrPDFNotActivated.
// Activation probes for a system Chrome/Chromium binary, falling
// back to go-rod's managed download. Activation state persists in
// a per-machine state file (`<AppRoot>/config/.pdf-state.json`),
// NOT in the active profile's user.json — `browser_bin` is a
// machine-specific path and would break under gigot/git sync.
// Stage 2 wires the store via system.Manager.
//
// Stage 1 (this commit) ships the type surface + Wails bindings;
// Stage 2 lands real activation; Stage 3 builds the frontmatter
// parser; Stage 4 wires the render → picoloom → file pipeline.
package pdf

import (
	"errors"
	"time"
)

// ErrPDFNotActivated is the typed error every Service method returns
// while the engine is inactive. The frontend treats this as a routing
// signal: re-open the Information page and highlight the activation
// panel.
var ErrPDFNotActivated = errors.New("pdf: not activated")

// Source describes where the active Chrome/Chromium binary came from.
// Values are stable and used in both the Status struct and persisted
// config — do not rename without a migration.
type Source string

const (
	// SourceUnset is the inactive state — no browser bound, no
	// activation attempted. Default for fresh profiles.
	SourceUnset Source = "unset"
	// SourceSystem means the user pointed activation at an existing
	// system binary (e.g. /usr/bin/chromium). Updates ride the OS
	// package manager.
	SourceSystem Source = "system"
	// SourceManaged means activation triggered go-rod's download and
	// the binary lives under ~/.cache/rod/browser/<revision>/.
	// Updates require an explicit re-download.
	SourceManaged Source = "managed"
)

// Status is the live snapshot of the PDF engine. Safe to call before
// or after activation; zero value means inactive.
//
// ExportDir is the per-machine "where renders land" preference. It is
// independent of activation — the user can set or change it while
// inactive, and Deactivate does not wipe it. Empty string means
// "no default — Stage 4 will fall back to placing PDFs next to the
// form" (see design/pdf-export.md, Stage 4 output path resolution).
type Status struct {
	Active      bool      `json:"active"`
	BrowserBin  string    `json:"browser_bin"`
	Source      Source    `json:"source"`
	Version     string    `json:"version"`
	ActivatedAt time.Time `json:"activated_at"`
	ExportDir   string    `json:"export_dir"`
}

// ActivateOpts carries the activation choices the user made in the
// Information-page dialog.
//
//   - BrowserBin: when set, skip probing and use this path directly.
//     When empty, the prober runs and the first candidate is picked.
//
// Formidable does not bundle or download Chrome — if no candidate
// is found, the user must install one (or point ROD_BROWSER_BIN at
// one) themselves.
type ActivateOpts struct {
	BrowserBin string `json:"browser_bin,omitempty"`
}

// ExportOpts shapes the per-call options ExportPDF accepts. Empty
// values fall back to the merged manifest + form-meta + global-config
// defaults.
//
//   - OutputPath: absolute or context-relative; default "<form>.pdf"
//     next to the form.
//   - Style: picoloom theme name or path to a custom CSS file.
//   - CoverTemplate: name of a cover from the on-disk library at
//     <AppRoot>/pdf/covers/. Empty = use whatever the merged
//     frontmatter / template manifest resolves to (which may itself
//     be empty → picoloom's default cover). Wired by the export
//     dialog's Cover picker for per-export overrides.
type ExportOpts struct {
	OutputPath    string `json:"output_path,omitempty"`
	Style         string `json:"style,omitempty"`
	CoverTemplate string `json:"cover_template,omitempty"`
}

// Result is the bound shape ExportPDF returns. The PDF bytes are not
// in the result — they go straight to disk via system.SaveFile. The
// frontend uses Path to surface an "Open" link in the success toast.
type Result struct {
	Path     string        `json:"path"`
	Bytes    int           `json:"bytes"`
	Duration time.Duration `json:"duration_ms"`
}

// ChromeCandidate is one entry returned by ProbeChrome — a Chrome/
// Chromium binary the activation flow can adopt. Version is best-
// effort (resolved by running `<path> --version`) and may be empty
// when the binary refuses to run or the call times out.
type ChromeCandidate struct {
	Path    string `json:"path"`
	Source  Source `json:"source"`
	Version string `json:"version,omitempty"`
}

// ProbeResult is what ProbeChrome returns to the activation dialog.
// Candidates is ordered: env-var override (if any), then system
// matches in the platform's standard search list, then managed-cache
// matches. Empty means no Chrome was found — the dialog should offer
// the managed-download path (Phase D).
type ProbeResult struct {
	Candidates []ChromeCandidate `json:"candidates"`
}
