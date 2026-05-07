package render

import (
	"fmt"
	"log/slog"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateLoader — what the render Manager needs from the template
// module. Satisfied by *template.Manager.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// formStore — what the render Manager needs from the storage module.
// Satisfied by *storage.Manager.
type formStore interface {
	LoadForm(templateFilename, datafile string) *storage.Form
}

// ImageURLFunc resolves an image filename (under the template's
// images/ folder) to a URL. The composition root supplies this:
// desktop returns `file:///<abs>/storage/<tpl>/images/<file>`, the
// future internal HTTP server returns `/storage/<tpl>/images/<file>`.
// May be nil; the renderer falls back to a relative `images/<file>`
// path so static markdown export still works.
type ImageURLFunc func(templateFilename, name string) string

// ImageBase64URLFunc resolves an image filename to a
// `data:<mime>;base64,<bytes>` URL. Wired separately from ImageURLFunc
// so a single render.Manager can serve both `<img src="/api/images/…">`
// (via ImageURL) and the generator's inline-image mode (via
// ImageBase64URL). May be nil — inline-mode markdown then renders the
// {{imageBase64}} helper as an empty string.
type ImageBase64URLFunc func(templateFilename, name string) string

// FormidableLinkURLFunc rewrites `formidable://<template>:<datafile>`
// hrefs into transport-specific URLs. Each export target plugs its
// own:
//
//   - in-app slideout: nil → keep formidable://, Vue interceptor handles
//   - internal wiki: "/template/<stem>/form/<datafile>"
//   - Azure DevOps wiki: relative `<page>.md` slug
//   - GitHub wiki: wiki page slug
//
// Empty string return = fall back to the original formidable:// URL
// (lets the rewriter punt on a malformed input without dropping it).
type FormidableLinkURLFunc func(templateFilename, datafile string) string

// Manager is the entry point Vue + the wiki HTTP server use to render
// a (template, datafile) pair to markdown + HTML. Per-target URL
// strategies are configured at construction; one Manager per target.
type Manager struct {
	templates         templateLoader
	storage           formStore
	imageURL          ImageURLFunc
	imageBase64URL    ImageBase64URLFunc
	formidableLinkURL FormidableLinkURLFunc
	log               *slog.Logger
}

// NewManager constructs a render Manager. log may be nil. Pass nil
// for either URL strategy to get the unrewritten passthrough.
//
// imageBase64URL is wired separately via SetImageBase64URL to keep the
// constructor signature stable across consumers that don't need
// inline-image mode (the wiki HTTP server, plain MD export, …).
func NewManager(t templateLoader, s formStore, imgURL ImageURLFunc, linkURL FormidableLinkURLFunc, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		templates:         t,
		storage:           s,
		imageURL:          imgURL,
		formidableLinkURL: linkURL,
		log:               log,
	}
}

// SetImageBase64URL wires the data-URL image strategy after construction.
// Use this only on Managers whose consumers actually want inline-mode
// (today: the slideout's render Manager, supplied by the composition
// root). Pass nil to disable.
func (m *Manager) SetImageBase64URL(fn ImageBase64URLFunc) {
	if m == nil {
		return
	}
	m.imageBase64URL = fn
}

// RenderForm returns both the Handlebars-rendered markdown and the
// goldmark+chroma-rendered HTML in one call. Empty datafile renders
// against the template's default values (which Vue rarely needs but
// keeps the path uniform with form.Manager.BuildView).
func (m *Manager) RenderForm(templateName, datafile string) (*Result, error) {
	md, err := m.RenderMarkdown(templateName, datafile)
	if err != nil {
		return nil, err
	}
	html, err := RenderHTML(md)
	if err != nil {
		return nil, err
	}
	return &Result{Markdown: md, HTML: html}, nil
}

// RenderMarkdown loads the template + form, then runs the Handlebars
// stage. The HTML stage isn't called.
func (m *Manager) RenderMarkdown(templateName, datafile string) (string, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return "", fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	values := map[string]any{}
	if datafile != "" {
		if loaded := m.storage.LoadForm(templateName, datafile); loaded != nil {
			values = loaded.Data
		}
	}
	opts := m.optionsFor(templateName)
	return RenderMarkdown(values, tpl, opts)
}

// RenderHTMLOnly is exposed as a Wails method so Vue can re-render
// markdown that the user edited in-place (e.g. preview pane) without
// going through the form pipeline.
func (m *Manager) RenderHTMLOnly(markdown string) (string, error) {
	return RenderHTML(markdown)
}

// optionsFor builds the per-template Options bundle. Captures the
// template filename in closures so the emitters don't need to thread
// it through. Both URL strategies fall back to nil-passthrough when
// the manager wasn't given them.
func (m *Manager) optionsFor(templateName string) *Options {
	opts := &Options{}
	if m.imageURL != nil {
		opts.ImageURL = func(name string) string {
			return m.imageURL(templateName, name)
		}
	}
	if m.imageBase64URL != nil {
		opts.ImageBase64URL = func(name string) string {
			return m.imageBase64URL(templateName, name)
		}
	}
	if m.formidableLinkURL != nil {
		// Note: this is invoked by resolveLinkHref AFTER it parses the
		// formidable:// URL, so the closure receives the template+
		// datafile pair from the URL itself, not the renderer's
		// `templateName` (a link can point cross-template).
		opts.FormidableLinkURL = m.formidableLinkURL
	}
	return opts
}
