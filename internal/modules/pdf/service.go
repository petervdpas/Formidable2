package pdf

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// builtinThemes is picoloom v2's canonical bundled style set. Picoloom
// does not expose a registry method, so this list IS the single source
// of truth on the Go side — the frontend reads it via ListThemes() and
// never hardcodes its own copy. Keep ordering stable: it's the order
// the dropdown will display.
//
// TODO: upstream a picoloom.ListStyles() or Asset registry method so
// this can become a pass-through instead of a duplicated list.
var builtinThemes = []ThemeDescriptor{
	{Name: "technical"},
	{Name: "academic"},
	{Name: "corporate"},
	{Name: "legal"},
	{Name: "invoice"},
	{Name: "manuscript"},
	{Name: "creative"},
}

// Service is the Wails-bound surface over Manager. The Information
// workspace activation panel and any "Export as PDF" trigger call
// these methods directly — there is no HTTP handler peer, PDF
// generation stays Wails-only (see types.go).
type Service struct{ m *Manager }

// NewService wraps a Manager. Panics on nil — that's a composition-
// root bug and must surface at boot, not later in a rare branch.
func NewService(m *Manager) *Service {
	if m == nil {
		panic("pdf: NewService called with nil manager")
	}
	return &Service{m: m}
}

// GetStatus returns the live engine state. Cheap; safe to poll from
// the Information page status row.
func (s *Service) GetStatus() Status { return s.m.Status() }

// AssetServerAddr returns the host:port the loopback asset server
// is bound to (e.g. "127.0.0.1:54321"), or "" when no server is
// wired or it has been closed. Surfaced in the Information → PDF
// Export panel as a diagnostic row so users can see + probe the URL
// that picoloom hands cover-library logos to Chrome via.
func (s *Service) AssetServerAddr() string {
	if s == nil || s.m == nil {
		return ""
	}
	return s.m.AssetServer().Addr()
}

// ProbeChrome lists every Chrome/Chromium binary the activation
// flow could adopt — env-var override, then system paths in their
// platform's conventional order, then the latest managed-cache
// pick. Empty Candidates means no Chrome was found; the dialog
// should offer the managed-download path (Phase D).
func (s *Service) ProbeChrome() ProbeResult { return s.m.Probe() }

// Activate is the user's "yes, set up PDF export" action. Stage 1
// always returns ErrPDFNotActivated; Stage 2 wires probing +
// managed-download flow.
func (s *Service) Activate(opts ActivateOpts) (Status, error) {
	return s.m.Activate(opts)
}

// Deactivate releases the bound Chrome binary without deleting any
// managed download. Stage 1 always returns ErrPDFNotActivated.
func (s *Service) Deactivate() error { return s.m.Deactivate() }

// ExportPDF renders the (templateFilename, datafile) form to a PDF
// on disk. Pipeline: render markdown → parse + merge frontmatter →
// build picoloom.Input → convert → atomic write. Returns
// ErrPDFNotActivated when the engine is inactive; otherwise wraps
// any downstream error with context. See pdf.Manager.Export for the
// full contract.
func (s *Service) ExportPDF(templateFilename, datafile string, opts ExportOpts) (Result, error) {
	return s.m.Export(templateFilename, datafile, opts)
}

// SetExportDir adopts a per-machine "where PDFs land" preference.
// Empty path clears the preference. Non-empty paths must be
// absolute + existent + a directory — otherwise the service
// returns ErrInvalidExportDir, which the frontend should surface
// as a user-correctable error (typically a re-pick via the native
// folder picker). Independent of activation state.
func (s *Service) SetExportDir(path string) (Status, error) {
	return s.m.SetExportDir(path)
}

// GetDirectivesDoc returns the embedded markdown reference for the
// picoloom frontmatter directives in the requested locale. Unknown
// locale falls back to English. Static content — safe to call any
// time, cheap, no I/O beyond the embedded FS.
func (s *Service) GetDirectivesDoc(locale string) (string, error) {
	return directivesDoc(locale)
}

// ListCovers returns descriptors for every cover discovered under
// <AppRoot>/pdf/covers/. Powers the cover-picker dropdown in the
// export dialog: scanned live on every call so user-added .html
// files appear without restart.
func (s *Service) ListCovers() ([]CoverDescriptor, error) {
	return s.m.ListCovers()
}

// SaveCover persists user-authored cover HTML. Validates first; on
// any error-severity issue, refuses to write and returns
// ErrCoverInvalid wrapped with structured issue codes. On success
// the cover becomes discoverable via ListCovers immediately.
func (s *Service) SaveCover(name, html string) error {
	return s.m.SaveCover(name, html)
}

// ValidateCoverHTML lets the frontend dry-run validation (e.g. on
// every keystroke in a cover editor) without writing to disk. Pure
// function — no I/O, no side effects.
func (s *Service) ValidateCoverHTML(html string) CoverValidation {
	return ValidateCover(html)
}

// InjectFrontmatter prepends the canonical picoloom v2 scaffold to a
// markdown body. Refuses with ErrFrontmatterAlreadyPresent when an
// existing `---` block is detected — the user should run
// MigrateFrontmatter in that case. The frontend inserts the result
// into the template editor; the user saves the template manually.
//
// Deprecated for new callers — prefer BuildFrontmatter(InjectConfig)
// + your own prepend logic for full control over which blocks/values
// land in the output. This helper survives because some callers want
// the full canonical fully-commented placeholder scaffold.
func (s *Service) InjectFrontmatter(markdown string) (string, error) {
	return InjectFrontmatter(markdown)
}

// BuildFrontmatter renders a typed InjectConfig (collected by the
// Inject dialog's toggles + dropdowns + text inputs) into a YAML
// frontmatter block. Each block the user enabled becomes a sub-block
// in the output, in canonical order, with empty fields skipped. The
// frontend prepends the result to its markdown_template draft.
func (s *Service) BuildFrontmatter(cfg InjectConfig) (string, error) {
	return BuildFrontmatter(cfg)
}

// ListPageSizes returns picoloom's canonical page-size set in
// dropdown order. Frontend reads this via the same pattern as
// ListThemes / ListLocales — never hardcoded.
func (s *Service) ListPageSizes() []PageSizeDescriptor {
	out := make([]PageSizeDescriptor, len(builtinPageSizes))
	copy(out, builtinPageSizes)
	return out
}

// ListPageOrientations returns picoloom's canonical orientation set.
func (s *Service) ListPageOrientations() []OrientationDescriptor {
	out := make([]OrientationDescriptor, len(builtinOrientations))
	copy(out, builtinOrientations)
	return out
}

// ListFooterPositions returns picoloom's canonical footer-position set.
func (s *Service) ListFooterPositions() []FooterPositionDescriptor {
	out := make([]FooterPositionDescriptor, len(builtinFooterPositions))
	copy(out, builtinFooterPositions)
	return out
}

// MigrateFrontmatter rewrites an existing eisvogel/pandoc-style
// frontmatter block into picoloom v2 shape. Returns the rewritten
// markdown alongside structured metadata (which keys were mapped,
// which landed in the legacy block, warnings) so the frontend can
// render a preview before applying.
func (s *Service) MigrateFrontmatter(markdown string) (FrontmatterMigration, error) {
	return MigrateFrontmatter(markdown)
}

// ListThemes returns the canonical picoloom bundled style set in
// stable display order. The frontend uses this to populate the Theme
// dropdown — it must NOT keep its own hardcoded list. Pure function,
// safe regardless of activation state.
func (s *Service) ListThemes() []ThemeDescriptor {
	out := make([]ThemeDescriptor, len(builtinThemes))
	copy(out, builtinThemes)
	return out
}

// ResolveExportDefaults previews the Theme + Cover that Manager.Export
// would pick for (template, datafile) under the dialog's default
// options. The dialog reads this on open and labels its default
// dropdown entry with the concrete value — so users know whether
// the template's frontmatter actually supplies a theme/cover or whether
// the picoloom built-in defaults kick in. Read-only; safe regardless
// of activation state.
func (s *Service) ResolveExportDefaults(templateFilename, datafile string) (ResolvedExportDefaults, error) {
	return s.m.ResolveExportDefaults(templateFilename, datafile)
}

// LoadCover returns the raw HTML for an existing cover. Skips
// validation so the editor can load a broken file and let the user
// fix it; the frontend should call ValidateCoverHTML on the loaded
// content to surface issues. Reserved names return ErrCoverNotFound.
func (s *Service) LoadCover(name string) (string, error) {
	return s.m.LoadCover(name)
}

// DeleteCover removes a user-added or seed cover from disk. Seed
// covers (classic/banner/corporate) reappear at next boot via the
// scaffold — the frontend should phrase this as "Reset" rather than
// "Delete" for those entries. Refuses reserved names with
// ErrCoverNotFound. Missing files are not an error.
func (s *Service) DeleteCover(name string) error {
	return s.m.DeleteCover(name)
}

// LastExport returns the most recent success + failure ExportTelemetry
// records held in memory by the Manager. Both fields may be nil when
// the process is fresh. Powers the PDF doctor sub-panel on the
// Information page.
func (s *Service) LastExport() ExportTelemetrySnapshot {
	return s.m.LastExport()
}

// ExportCoverArchive zips a cover .html plus every image referenced
// from its <img src=…> and CSS url(…) into the user-picked zipPath.
// Used by the cover-sharing flow on the Information → PDF Covers panel.
func (s *Service) ExportCoverArchive(name, zipPath string) (ExportCoverArchiveResult, error) {
	return s.m.ExportCoverArchive(name, zipPath)
}

// ImportCoverArchive materialises a cover archive zip from zipPath
// back into <AppRoot>/pdf/covers/. overwrite=false (default) refuses
// to replace an existing cover so the frontend can confirm with the
// user before retrying with overwrite=true.
func (s *Service) ImportCoverArchive(zipPath string, overwrite bool) (ImportCoverArchiveResult, error) {
	return s.m.ImportCoverArchive(zipPath, overwrite)
}

// ListCoverImages returns descriptors for every image discovered
// under <AppRoot>/pdf/covers/images/. Seed images (e.g. formidable.svg)
// are flagged so the frontend can offer Reset-to-default instead of
// permanent delete.
func (s *Service) ListCoverImages() ([]CoverImageDescriptor, error) {
	return s.m.ListCoverImages()
}

// SaveCoverImage persists a user-uploaded image under
// <AppRoot>/pdf/covers/images/<name>. data is the base64-encoded
// file body — the frontend reads the upload via FileReader and
// passes the result here, keeping the Wails JSON boundary clean.
// The filename and extension are validated before any bytes hit
// disk; on any rejection the function returns ErrCoverImageInvalid.
func (s *Service) SaveCoverImage(name, base64Data string) error {
	if idx := strings.Index(base64Data, ","); idx >= 0 && strings.HasPrefix(base64Data, "data:") {
		base64Data = base64Data[idx+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(base64Data)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCoverImageInvalid, err)
	}
	return s.m.SaveCoverImage(name, raw)
}

// LoadCoverImage returns the raw bytes for one image as a base64
// string so it can ride the JSON Wails surface. The frontend uses
// this to render image previews next to the file metadata.
func (s *Service) LoadCoverImage(name string) (string, error) {
	raw, err := s.m.LoadCoverImage(name)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DeleteCoverImage removes a user-uploaded or seed image. Seed
// images reappear on the next boot via the scaffold (delete-to-
// reset, matching the cover .html flow), so the frontend should
// phrase the action as "Reset" for those entries.
func (s *Service) DeleteCoverImage(name string) error {
	return s.m.DeleteCoverImage(name)
}
