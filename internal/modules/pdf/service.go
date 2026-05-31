package pdf

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/petervdpas/formidable2/internal/optrack"
)

// builtinThemes is picoloom v2's bundled style set. Picoloom exposes no
// registry, so this list is the single source of truth; ordering is the
// dropdown order. Keep in sync with picoloom.
//
// TODO: upstream a picoloom.ListStyles() so this can be a pass-through.
var builtinThemes = []ThemeDescriptor{
	{Name: "technical"},
	{Name: "academic"},
	{Name: "corporate"},
	{Name: "legal"},
	{Name: "invoice"},
	{Name: "manuscript"},
	{Name: "creative"},
}

// Service is the Wails-bound surface over Manager. There is no HTTP
// peer; PDF generation stays Wails-only.
type Service struct {
	m   *Manager
	ops *optrack.Registry
}

// AttachOps installs the shared op registry so ExportPDF registers its state
// (guarding "cannot run twice" and letting the frontend resume on reload).
func AttachOps(s *Service, ops *optrack.Registry) {
	if s == nil {
		return
	}
	s.ops = ops
}

// NewService wraps a Manager, panicking on nil so a composition-root
// bug surfaces at boot.
func NewService(m *Manager) *Service {
	if m == nil {
		panic("pdf: NewService called with nil manager")
	}
	return &Service{m: m}
}

// GetStatus returns the live engine state.
func (s *Service) GetStatus() Status { return s.m.Status() }

// AssetServerAddr returns the asset server's host:port, or "" when none
// is wired or it is closed.
func (s *Service) AssetServerAddr() string {
	if s == nil || s.m == nil {
		return ""
	}
	return s.m.AssetServer().Addr()
}

// ProbeChrome lists every Chrome/Chromium binary the activation flow
// could adopt; empty Candidates means none was found.
func (s *Service) ProbeChrome() ProbeResult { return s.m.Probe() }

// Activate adopts a Chrome binary for PDF export.
func (s *Service) Activate(opts ActivateOpts) (Status, error) {
	return s.m.Activate(opts)
}

// Deactivate releases the bound Chrome binary, keeping any managed download.
func (s *Service) Deactivate() error { return s.m.Deactivate() }

// ExportPDF renders the form to a PDF on disk; see Manager.Export. Tracked and
// guarded against a concurrent second export, which would race the same Chrome.
func (s *Service) ExportPDF(templateFilename, datafile string, opts ExportOpts) (Result, error) {
	_, release, err := optrack.Guard(s.ops, "pdf:export")
	if err != nil {
		return Result{}, err
	}
	defer release()
	return s.m.Export(templateFilename, datafile, opts)
}

// SetExportDir sets the per-machine export folder; see Manager.SetExportDir.
func (s *Service) SetExportDir(path string) (Status, error) {
	return s.m.SetExportDir(path)
}

// GetDirectivesDoc returns the directives reference markdown for
// locale, falling back to English.
func (s *Service) GetDirectivesDoc(locale string) (string, error) {
	return directivesDoc(locale)
}

// ListCovers returns descriptors for every cover under <AppRoot>/pdf/covers/.
func (s *Service) ListCovers() ([]CoverDescriptor, error) {
	return s.m.ListCovers()
}

// SaveCover validates and persists cover HTML; see Manager.SaveCover.
func (s *Service) SaveCover(name, html string) error {
	return s.m.SaveCover(name, html)
}

// ValidateCoverHTML dry-runs cover validation without touching disk.
func (s *Service) ValidateCoverHTML(html string) CoverValidation {
	return ValidateCover(html)
}

// InjectFrontmatter prepends the canonical scaffold to a markdown body,
// or returns ErrFrontmatterAlreadyPresent.
//
// Deprecated: prefer BuildFrontmatter(InjectConfig); this survives only
// for callers that want the full commented placeholder scaffold.
func (s *Service) InjectFrontmatter(markdown string) (string, error) {
	return InjectFrontmatter(markdown)
}

// BuildFrontmatter renders an InjectConfig into a YAML frontmatter block.
func (s *Service) BuildFrontmatter(cfg InjectConfig) (string, error) {
	return BuildFrontmatter(cfg)
}

// ListPageSizes returns picoloom's page-size set in dropdown order.
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

// MigrateFrontmatter rewrites an eisvogel/pandoc block into picoloom v2
// shape; see MigrateFrontmatter for the contract.
func (s *Service) MigrateFrontmatter(markdown string) (FrontmatterMigration, error) {
	return MigrateFrontmatter(markdown)
}

// ListThemes returns the picoloom bundled style set in display order.
func (s *Service) ListThemes() []ThemeDescriptor {
	out := make([]ThemeDescriptor, len(builtinThemes))
	copy(out, builtinThemes)
	return out
}

// ResolveExportDefaults previews the Theme + Cover Export would pick;
// see Manager.ResolveExportDefaults.
func (s *Service) ResolveExportDefaults(templateFilename, datafile string) (ResolvedExportDefaults, error) {
	return s.m.ResolveExportDefaults(templateFilename, datafile)
}

// LoadCover returns the raw cover HTML for editing (no validation).
func (s *Service) LoadCover(name string) (string, error) {
	return s.m.LoadCover(name)
}

// DeleteCover removes a cover; see Manager.DeleteCover for the seed
// reset behaviour.
func (s *Service) DeleteCover(name string) error {
	return s.m.DeleteCover(name)
}

// LastExport returns the most recent success + failure telemetry.
func (s *Service) LastExport() ExportTelemetrySnapshot {
	return s.m.LastExport()
}

// ExportCoverArchive zips a cover and its image refs into zipPath.
func (s *Service) ExportCoverArchive(name, zipPath string) (ExportCoverArchiveResult, error) {
	return s.m.ExportCoverArchive(name, zipPath)
}

// ImportCoverArchive materialises a cover archive from zipPath; refuses
// to clobber unless overwrite=true.
func (s *Service) ImportCoverArchive(zipPath string, overwrite bool) (ImportCoverArchiveResult, error) {
	return s.m.ImportCoverArchive(zipPath, overwrite)
}

// ListCoverImages returns descriptors for every image under
// <AppRoot>/pdf/covers/images/; seeds are flagged.
func (s *Service) ListCoverImages() ([]CoverImageDescriptor, error) {
	return s.m.ListCoverImages()
}

// SaveCoverImage decodes a base64 (or data-URI) body and persists it
// under <AppRoot>/pdf/covers/images/<name>.
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

// LoadCoverImage returns one image's bytes base64-encoded for the JSON
// Wails surface.
func (s *Service) LoadCoverImage(name string) (string, error) {
	raw, err := s.m.LoadCoverImage(name)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DeleteCoverImage removes an image; seeds reappear via the next boot's
// scaffold (phrase as "Reset" for those).
func (s *Service) DeleteCoverImage(name string) error {
	return s.m.DeleteCoverImage(name)
}
