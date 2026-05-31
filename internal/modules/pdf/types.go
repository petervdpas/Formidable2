// Package pdf owns Formidable's PDF export pipeline, built on picoloom
// v2 (headless Chrome via go-rod). See design/pdf-export.md.
//
// The module is opt-in: until activated, every action returns
// ErrPDFNotActivated. Activation state persists per-machine in
// <AppRoot>/config/.pdf-state.json, NOT in user.json, because
// browser_bin is machine-specific and would break under gigot/git sync.
package pdf

import (
	"errors"
	"time"
)

// ErrPDFNotActivated is returned while the engine is inactive; the
// frontend treats it as "re-open the Information page" routing signal.
var ErrPDFNotActivated = errors.New("pdf: not activated")

// Source describes where the active Chrome binary came from. Values are
// stable and persisted; do not rename without a migration.
type Source string

const (
	SourceUnset   Source = "unset"
	SourceSystem  Source = "system"
	SourceManaged Source = "managed"
)

// Status is the live PDF-engine snapshot; zero value means inactive.
// ExportDir is independent of activation (Deactivate does not wipe it);
// empty means renders land next to the form.
type Status struct {
	Active      bool      `json:"active"`
	BrowserBin  string    `json:"browser_bin"`
	Source      Source    `json:"source"`
	Version     string    `json:"version"`
	ActivatedAt time.Time `json:"activated_at"`
	ExportDir   string    `json:"export_dir"`
}

// ActivateOpts carries the activation choices. BrowserBin, when set,
// skips probing. Formidable does not bundle or download Chrome; with no
// candidate the user must install one or set ROD_BROWSER_BIN.
type ActivateOpts struct {
	BrowserBin string `json:"browser_bin,omitempty"`
}

// ExportOpts shapes ExportPDF's per-call options; empty values fall
// back to the merged frontmatter + manifest defaults.
type ExportOpts struct {
	OutputPath    string `json:"output_path,omitempty"`
	Style         string `json:"style,omitempty"`
	CoverTemplate string `json:"cover_template,omitempty"`

	// DisableCover forces no cover, winning over CoverTemplate.
	DisableCover bool `json:"disable_cover,omitempty"`

	// DisableTheme forces picoloom's default CSS, winning over Style.
	DisableTheme bool `json:"disable_theme,omitempty"`
}

// Result is what ExportPDF returns. The PDF bytes are NOT here; they go
// straight to disk. Path drives the "Open" link in the success toast.
type Result struct {
	Path     string        `json:"path"`
	Bytes    int           `json:"bytes"`
	Duration time.Duration `json:"duration_ms"`
}

// ChromeCandidate is one ProbeChrome entry. Version is best-effort and
// may be empty when the binary refuses --version or times out.
type ChromeCandidate struct {
	Path    string `json:"path"`
	Source  Source `json:"source"`
	Version string `json:"version,omitempty"`
}

// ProbeResult holds ProbeChrome's ordered Candidates (env override,
// system paths, managed cache). Empty means no Chrome was found.
type ProbeResult struct {
	Candidates []ChromeCandidate `json:"candidates"`
}

// ExportTelemetry is the in-memory record of one Export call. Success
// records carry Path + Bytes, failures Code + Stage + Err; the rest
// apply to both. Stable shape: JSON tags are part of the Wails contract.
type ExportTelemetry struct {
	At         time.Time `json:"at"`
	Template   string    `json:"template"`
	Datafile   string    `json:"datafile"`
	DurationMs int64     `json:"duration_ms"`
	Theme      string    `json:"theme,omitempty"`
	Cover      string    `json:"cover,omitempty"`
	HasCover   bool      `json:"has_cover"`

	// Success-only.
	Path  string `json:"path,omitempty"`
	Bytes int    `json:"bytes,omitempty"`

	// Failure-only.
	Code  string `json:"code,omitempty"`
	Stage string `json:"stage,omitempty"`
	Err   string `json:"err,omitempty"`
}

// ExportTelemetrySnapshot is the doctor's recent-activity read; either
// field may be nil before that kind of export has happened.
type ExportTelemetrySnapshot struct {
	LastSuccess *ExportTelemetry `json:"last_success,omitempty"`
	LastFailure *ExportTelemetry `json:"last_failure,omitempty"`
}

// ThemeDescriptor is one Theme-dropdown entry; Name is the
// picoloom.WithStyle key, labelled via frontend i18n. The canonical
// list lives in builtinThemes (service.go); keep it in sync with picoloom.
type ThemeDescriptor struct {
	Name string `json:"name"`
}

// ResolvedExportDefaults reveals the Theme + Cover Export would pick
// under default options, so the dialog can label its default entry.
// Empty strings mean no override (picoloom built-in); CoverDisabled
// distinguishes that from an explicit cover.enabled: false.
type ResolvedExportDefaults struct {
	Theme         string `json:"theme"`
	CoverTemplate string `json:"cover_template"`
	CoverDisabled bool   `json:"cover_disabled,omitempty"`
}
