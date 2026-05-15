package pdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	picoloom "github.com/alnah/picoloom/v2"
)

// exportTimeout caps how long Chrome can spend on a single document.
// Picoloom's own default is 30s; we give a more generous ceiling
// because cold-start Chrome on slow disks can take a few seconds
// before rendering even begins. Settable later via ExportOpts if a
// user-facing knob is needed.
const exportTimeout = 60 * time.Second

// renderer is the slice of render.Manager the pdf module needs.
// Satisfied by *render.Manager in production; tests inject a stub.
type renderer interface {
	RenderMarkdown(templateFilename, datafile string) (string, error)
}

// storageFS is the slice of storage.Manager the pdf module needs:
// the absolute filesystem location where a given template's forms +
// assets live. Used both for SourceDir resolution (so picoloom can
// load relative-path images authored in markdown) and as the
// default output directory ("next to the form") when neither
// ExportOpts.OutputPath nor Status.ExportDir is set.
//
// Satisfied by *storage.Manager.
type storageFS interface {
	TemplateStorageDir(templateFilename string) string
}

// converter is the slice of picoloom.Converter we exercise.
// *picoloom.Converter satisfies this directly; tests inject a stub
// so the unit suite never boots Chrome.
type converter interface {
	Convert(ctx context.Context, input picoloom.Input) (*picoloom.ConvertResult, error)
	Close() error
}

// converterFactory builds a converter sized for one export call.
// browserBin and style are read off the merged frontmatter + opts
// at call time; the production factory wires them onto picoloom via
// the ROD_BROWSER_BIN env var and the WithStyle option (picoloom
// does not expose a browser-bin option, so env is the only hook).
type converterFactory func(browserBin, style string) (converter, error)

// realConverterFactory is the production converterFactory. It sets
// ROD_BROWSER_BIN if the active browser path is non-empty (Stage 2's
// activation gate guarantees this for any successful Export call),
// then builds a picoloom converter. The caller owns Close().
//
// Setting ROD_BROWSER_BIN per call is intentional rather than
// once at Activate time: external processes can clear the env, and
// the cost is negligible compared to Chrome boot.
func realConverterFactory(browserBin, style string) (converter, error) {
	if browserBin != "" {
		_ = os.Setenv("ROD_BROWSER_BIN", browserBin)
	}
	opts := []picoloom.Option{picoloom.WithTimeout(exportTimeout)}
	if style != "" {
		opts = append(opts, picoloom.WithStyle(style))
	}
	c, err := picoloom.NewConverter(opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Export renders the (templateFilename, datafile) form to a PDF on
// disk. The pipeline is:
//
//  1. Refuse the call if the engine is inactive.
//  2. Serialize concurrent calls for the same form (formMu); distinct
//     forms render in parallel.
//  3. Render markdown via the injected render.Manager (Handlebars
//     stage; frontmatter survives for picoloom to read).
//  4. Parse + merge frontmatter. Stage 4 only carries the doc layer;
//     form-meta / manifest / global layers wire in at Stage 6+.
//  5. Build the picoloom.Input, defaulting SourceDir to the template's
//     storage directory so relative-path images resolve.
//  6. Build a converter (sets ROD_BROWSER_BIN, applies WithStyle).
//  7. Convert, close the converter, atomically write the PDF bytes.
//
// Style precedence: ExportOpts.Style > merged frontmatter Style > "".
// Output path precedence: ExportOpts.OutputPath > Status.ExportDir
// + basename > template storage dir + basename.
//
// All error returns wrap the underlying cause; the typed gate for
// "engine off" stays errors.Is-compatible with ErrPDFNotActivated.
func (m *Manager) Export(templateFilename, datafile string, opts ExportOpts) (Result, error) {
	started := m.nowFn()
	m.log.Debug("pdf: export",
		"template", templateFilename, "datafile", datafile,
		"output_path", opts.OutputPath, "style", opts.Style)

	status := m.Status()
	if !status.Active {
		return Result{}, ErrPDFNotActivated
	}

	formKey := templateFilename + "|" + datafile
	unlock := m.formMu.Lock(formKey)
	defer unlock()

	rendered, err := m.renderer.RenderMarkdown(templateFilename, datafile)
	if err != nil {
		return Result{}, fmt.Errorf("pdf: render markdown: %w", err)
	}

	docFM, body, parseErr := ParseFrontmatter(rendered)
	if parseErr != nil {
		m.log.Warn("pdf: frontmatter parse failed; using defaults",
			"template", templateFilename, "datafile", datafile, "err", parseErr)
		// docFM is the zero value here; body is the input verbatim so
		// the malformed frontmatter still gets shipped to picoloom,
		// which will strip it the same way. Render proceeds.
	}

	merged := Merge(docFM)

	input := BuildInput(merged, body)
	if input.SourceDir == "" && m.storage != nil {
		input.SourceDir = m.storage.TemplateStorageDir(templateFilename)
	}

	style := opts.Style
	if style == "" {
		style = merged.Style
	}

	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	defer cancel()

	conv, err := m.convertFn(status.BrowserBin, style)
	if err != nil {
		return Result{}, fmt.Errorf("pdf: build converter: %w", err)
	}
	defer func() { _ = conv.Close() }()

	res, err := conv.Convert(ctx, input)
	if err != nil {
		return Result{}, fmt.Errorf("pdf: convert: %w", err)
	}
	if res == nil || len(res.PDF) == 0 {
		return Result{}, errors.New("pdf: converter returned empty PDF")
	}

	outPath := m.resolveOutputPath(templateFilename, datafile, opts, status.ExportDir)
	if err := m.store.fs.SaveFile(outPath, string(res.PDF)); err != nil {
		return Result{}, fmt.Errorf("pdf: save: %w", err)
	}

	duration := m.nowFn().Sub(started)
	m.log.Info("pdf: exported",
		"path", outPath, "bytes", len(res.PDF), "duration_ms", duration.Milliseconds())

	return Result{
		Path:     outPath,
		Bytes:    len(res.PDF),
		Duration: duration,
	}, nil
}

// resolveOutputPath chooses where the PDF lands:
//
//   - ExportOpts.OutputPath wins outright. Absolute is used as-is;
//     relative is resolved against the ExportDir (or storage dir).
//   - Empty OutputPath + non-empty ExportDir → ExportDir/<basename>.pdf.
//   - Otherwise → <template storage dir>/<basename>.pdf ("next to
//     the form", per design doc Stage 4 default).
//
// The basename strips `.meta.json` then any remaining extension, so
// `adapter-eum.meta.json` → `adapter-eum.pdf`.
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

// pdfBasename derives the bare PDF filename from a form datafile.
// Strips the `.meta.json` envelope and any residual extension, so a
// form named `adapter-eum.meta.json` exports as `adapter-eum.pdf`.
// Falls back to `export.pdf` when the input is empty or extension-only.
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
