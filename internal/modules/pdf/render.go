package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	picoloom "github.com/alnah/picoloom/v2"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// exportTimeout caps Chrome per document. More generous than picoloom's
// 30s default because cold-start Chrome on slow disks can take seconds
// before rendering begins.
const exportTimeout = 60 * time.Second

// renderer is the slice of render.Manager the pdf module needs.
type renderer interface {
	RenderMarkdown(templateFilename, datafile string) (string, error)
}

// storageFS yields a template's storage dir: used for SourceDir
// resolution (relative-path images) and as the default output directory.
type storageFS interface {
	TemplateStorageDir(templateFilename string) string
}

// templateLoader reads per-template PDF defaults (style + cover) for the
// manifest merge layer. May be nil; Export falls through to doc-frontmatter only.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// converter is the slice of picoloom.Converter we exercise.
type converter interface {
	Convert(ctx context.Context, input picoloom.Input) (*picoloom.ConvertResult, error)
	Close() error
}

// converterFactory builds a converter for one export call. browserBin
// is the ROD_BROWSER_BIN snapshot (picoloom has no Bin opt); coverTS
// nil means picoloom's default cover.
type converterFactory func(browserBin, style string, coverTS *picoloom.TemplateSet) (converter, error)

// realConverterFactory builds a picoloom converter. It sets
// ROD_BROWSER_BIN per call (not once at Activate) because external
// processes can clear the env and the cost is negligible vs Chrome boot.
func realConverterFactory(browserBin, style string, coverTS *picoloom.TemplateSet) (converter, error) {
	if browserBin != "" {
		_ = os.Setenv("ROD_BROWSER_BIN", browserBin)
	}
	opts := []picoloom.Option{picoloom.WithTimeout(exportTimeout)}
	if style != "" {
		opts = append(opts, picoloom.WithStyle(style))
	}
	if coverTS != nil {
		opts = append(opts, picoloom.WithTemplateSet(coverTS))
	}
	c, err := picoloom.NewConverter(opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Export renders the (templateFilename, datafile) form to a PDF on
// disk: render markdown, parse + merge frontmatter, build the
// picoloom.Input (SourceDir defaults to the template storage dir),
// convert, then atomically write. Refuses when inactive
// (ErrPDFNotActivated); concurrent calls for the same form serialize
// via formMu while distinct forms render in parallel.
//
// Style precedence: opts.Style > merged Style > "". Output path:
// opts.OutputPath > ExportDir/basename > storage dir/basename.
func (m *Manager) Export(templateFilename, datafile string, opts ExportOpts) (Result, error) {
	started := m.nowFn()
	m.log.Debug("pdf: export",
		"template", templateFilename, "datafile", datafile,
		"output_path", opts.OutputPath, "style", opts.Style)

	status := m.Status()
	if !status.Active {
		return m.failExport(started, templateFilename, datafile, "engine_gate", ErrPDFNotActivated)
	}

	formKey := templateFilename + "|" + datafile
	unlock := m.formMu.Lock(formKey)
	defer unlock()

	rendered, err := m.renderer.RenderMarkdown(templateFilename, datafile)
	if err != nil {
		return m.failExport(started, templateFilename, datafile, "render_markdown", &ExportError{
			Code:    CodeRenderFailed,
			Message: "pdf: render markdown: " + err.Error(),
			Hint:    "Check the template's handlebars/markdown for syntax errors.",
			Cause:   err,
		})
	}

	docFM, body, parseErr := ParseFrontmatter(rendered)
	if parseErr != nil {
		m.log.Warn("pdf: frontmatter parse failed; using defaults",
			"template", templateFilename, "datafile", datafile, "err", parseErr)
		// body is verbatim so picoloom strips the malformed frontmatter
		// the same way; render proceeds.
	}

	// Bake ```mermaid fences to inline SVG before picoloom (which can't wait
	// for client-side JS). Best-effort: leaves fences on failure.
	body = m.bakeMermaidSVG(body, status.BrowserBin)

	manifestFM := m.loadManifestFrontmatter(templateFilename)
	merged := Merge(docFM, manifestFM)

	sourceDir := ""
	if m.storage != nil {
		sourceDir = m.storage.TemplateStorageDir(templateFilename)
	}

	input := BuildInput(merged, body)
	if input.SourceDir == "" {
		input.SourceDir = sourceDir
	}

	// Resolve cover.logo to a string picoloom + Chrome can load
	// cross-platform. The asset server (when wired) feeds central-library
	// logos an http:// URL, needed on Windows because Chrome in a file://
	// document can't load a bare `C:/...` <img src>. See BuildCoverLogoSrc.
	if input.Cover != nil {
		input.Cover.Logo = BuildCoverLogoSrc(input.Cover.Logo, input.SourceDir, m.store.fs, m.AssetServer())
	}

	// Theme precedence: DisableTheme forces empty over everything; else
	// opts.Style, then merged.Style.
	var style string
	if !opts.DisableTheme {
		style = opts.Style
		if style == "" {
			style = merged.Style
		}
	}

	// Cover precedence: DisableCover > CoverTemplate (dialog pick) >
	// merged Enabled=false (frontmatter turned it off; BuildInput drops
	// the data, we drop the template to match) > merged.Cover > nil.
	var coverFM *CoverFM
	switch {
	case opts.DisableCover:
		coverFM = nil
	case opts.CoverTemplate != "":
		if merged.Cover == nil {
			coverFM = &CoverFM{}
		} else {
			cp := *merged.Cover
			coverFM = &cp
		}
		coverFM.Template = opts.CoverTemplate
		coverFM.TemplatePath = ""
		// Explicit dialog pick re-enables the cover even if the
		// merged frontmatter had Enabled=false.
		on := true
		coverFM.Enabled = &on
	case merged.Cover != nil && merged.Cover.Enabled != nil && !*merged.Cover.Enabled:
		coverFM = nil
	default:
		coverFM = merged.Cover
	}

	coverTS, err := ResolveCoverTemplateSet(coverFM, sourceDir, m.store.fs)
	if err != nil {
		return m.failExport(started, templateFilename, datafile, "resolve_cover",
			fmt.Errorf("pdf: resolve cover: %w", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	defer cancel()

	conv, err := m.convertFn(status.BrowserBin, style, coverTS)
	if err != nil {
		return m.failExport(started, templateFilename, datafile, "build_converter",
			fmt.Errorf("pdf: build converter: %w", err))
	}
	defer func() { _ = conv.Close() }()

	res, err := conv.Convert(ctx, input)
	if err != nil {
		return m.failExport(started, templateFilename, datafile, "convert",
			fmt.Errorf("pdf: convert: %w", err))
	}
	if res == nil || len(res.PDF) == 0 {
		return m.failExport(started, templateFilename, datafile, "convert", errEmptyPDF)
	}

	pdfBytes := res.PDF
	md := buildPDFMetadata(merged)
	if md.HasContent() {
		injected, mErr := InjectPDFMetadata(pdfBytes, md)
		if mErr != nil {
			m.log.Warn("pdf: metadata injection failed; saving unmodified PDF",
				"template", templateFilename, "datafile", datafile, "err", mErr)
		} else {
			pdfBytes = injected
		}
	}

	outPath := m.resolveOutputPath(templateFilename, datafile, opts, status.ExportDir)
	if err := m.store.fs.SaveFile(outPath, string(pdfBytes)); err != nil {
		return m.failExport(started, templateFilename, datafile, "save",
			fmt.Errorf("%w: %v", errSaveFailed, err))
	}

	finishedAt := m.nowFn()
	duration := finishedAt.Sub(started)
	m.log.Info("pdf: exported",
		"template", templateFilename,
		"datafile", datafile,
		"path", outPath,
		"bytes", len(pdfBytes),
		"duration_ms", duration.Milliseconds(),
		"theme", style,
		"cover", coverNameForLog(coverFM),
		"has_cover", coverTS != nil,
	)
	m.recordSuccess(&ExportTelemetry{
		At:         finishedAt,
		Template:   templateFilename,
		Datafile:   datafile,
		DurationMs: duration.Milliseconds(),
		Theme:      style,
		Cover:      coverNameForLog(coverFM),
		HasCover:   coverTS != nil,
		Path:       outPath,
		Bytes:      len(pdfBytes),
	})

	return Result{
		Path:     outPath,
		Bytes:    len(pdfBytes),
		Duration: duration,
	}, nil
}

// ResolveExportDefaults previews the Theme + CoverTemplate Export would
// compute with no opts override, via the same Merge pipeline. Read-only:
// not gated on activation, no formMu. Renderer failures bubble up;
// malformed frontmatter is tolerated.
func (m *Manager) ResolveExportDefaults(templateFilename, datafile string) (ResolvedExportDefaults, error) {
	rendered, err := m.renderer.RenderMarkdown(templateFilename, datafile)
	if err != nil {
		return ResolvedExportDefaults{}, fmt.Errorf("pdf: resolve defaults: %w", err)
	}
	docFM, _, _ := ParseFrontmatter(rendered)
	manifestFM := m.loadManifestFrontmatter(templateFilename)
	merged := Merge(docFM, manifestFM)
	out := ResolvedExportDefaults{Theme: merged.Style}
	if merged.Cover != nil {
		if merged.Cover.Enabled != nil && !*merged.Cover.Enabled {
			out.CoverDisabled = true
		} else {
			out.CoverTemplate = merged.Cover.Template
		}
	}
	return out, nil
}

// failExport maps err to a typed ExportError, logs a "pdf: export
// failed" event, and returns the zero Result. The stage strings are
// stable (consumed by the PDF doctor): keep them lowercase snake_case.
func (m *Manager) failExport(started time.Time, templateFilename, datafile, stage string, err error) (Result, error) {
	mapped := MapExportError(err)
	finishedAt := m.nowFn()
	duration := finishedAt.Sub(started)
	code := ""
	if mapped != nil {
		code = string(mapped.Code)
	}
	m.log.Error("pdf: export failed",
		"template", templateFilename,
		"datafile", datafile,
		"code", code,
		"stage", stage,
		"duration_ms", duration.Milliseconds(),
		"err", err.Error(),
	)
	m.recordFailure(&ExportTelemetry{
		At:         finishedAt,
		Template:   templateFilename,
		Datafile:   datafile,
		DurationMs: duration.Milliseconds(),
		Code:       code,
		Stage:      stage,
		Err:        err.Error(),
	})
	return Result{}, mapped
}

// coverNameForLog returns a stable cover identifier, preferring the
// more-specific TemplatePath over the library name.
func coverNameForLog(fm *CoverFM) string {
	if fm == nil {
		return ""
	}
	if fm.TemplatePath != "" {
		return fm.TemplatePath
	}
	return fm.Template
}

// loadManifestFrontmatter projects per-template PDF defaults into the
// Merge "manifest" layer. Returns the zero value on any miss; manifest
// defaults are best-effort and must never block a render.
func (m *Manager) loadManifestFrontmatter(templateFilename string) Frontmatter {
	if m.templates == nil {
		return Frontmatter{}
	}
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil || tpl == nil || tpl.PDF == nil {
		return Frontmatter{}
	}
	out := Frontmatter{Style: tpl.PDF.Style}
	if tpl.PDF.Cover != nil {
		out.Cover = projectTemplateCover(tpl.PDF.Cover)
	}
	return out
}

// projectTemplateCover copies template.PDFCoverConfig into a CoverFM.
func projectTemplateCover(c *template.PDFCoverConfig) *CoverFM {
	if c == nil {
		return nil
	}
	out := &CoverFM{
		Template:     c.Template,
		TemplatePath: c.TemplatePath,
		Title:        c.Title,
		Subtitle:     c.Subtitle,
		Logo:         c.Logo,
		Author:       c.Author,
		AuthorTitle:  c.AuthorTitle,
		Organization: c.Organization,
		Date:         c.Date,
		Version:      c.Version,
		ClientName:   c.ClientName,
		ProjectName:  c.ProjectName,
		DocumentType: c.DocumentType,
		DocumentID:   c.DocumentID,
		Description:  c.Description,
		Department:   c.Department,
	}
	if c.Enabled != nil {
		v := *c.Enabled
		out.Enabled = &v
	}
	return out
}

// resolveOutputPath chooses where the PDF lands: opts.OutputPath
// (absolute as-is, relative against ExportDir/storage) > ExportDir >
// template storage dir, with the datafile basename.
func (m *Manager) resolveOutputPath(templateFilename, datafile string, opts ExportOpts, exportDir string) string {
	if opts.OutputPath != "" {
		if filepath.IsAbs(opts.OutputPath) {
			return filepath.Clean(opts.OutputPath)
		}
		base := exportDir
		if base == "" && m.storage != nil {
			base = m.storage.TemplateStorageDir(templateFilename)
		}
		return filepath.Clean(filepath.Join(base, opts.OutputPath))
	}
	dir := exportDir
	if dir == "" && m.storage != nil {
		dir = m.storage.TemplateStorageDir(templateFilename)
	}
	return filepath.Clean(filepath.Join(dir, pdfBasename(datafile)))
}

// pdfBasename derives the PDF filename, stripping `.meta.json` and any
// residual extension (`adapter-eum.meta.json` -> `adapter-eum.pdf`).
// Falls back to `export.pdf`.
func pdfBasename(datafile string) string {
	name := strings.TrimSuffix(datafile, ".meta.json")
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	if name == "" {
		name = "export"
	}
	return name + ".pdf"
}
