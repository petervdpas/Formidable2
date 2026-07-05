package render

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateLoader is what the render Manager needs from the template module.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// formStore is what the render Manager needs from the storage module.
type formStore interface {
	LoadForm(templateFilename, datafile string) *storage.Form
}

// ImageURLFunc resolves an image filename to a URL; nil falls back to a
// relative images/<file> path so static export still works.
type ImageURLFunc func(templateFilename, name string) string

// ImageBase64URLFunc resolves an image to a data: URL for inline-image
// mode, wired separately from ImageURLFunc; nil renders {{imageBase64}} as "".
type ImageBase64URLFunc func(templateFilename, name string) string

// FormidableLinkURLFunc rewrites a formidable://<template>:<datafile> href
// per target; nil keeps the formidable:// URL, empty-string return falls
// back to it (punt on malformed input without dropping the link).
type FormidableLinkURLFunc func(templateFilename, datafile string) string

// ReferenceResolverFunc projects one target-collection record (by id) into a row
// keyed by columnKeys, read live for api-field rendering. Nil renders api fields
// empty rather than stale.
type ReferenceResolverFunc func(targetTemplate, id string, columnKeys []string) map[string]any

// ReferenceLinkResolverFunc resolves a target-collection record id to its
// formidable://<template>:<datafile> deep link for api-field card anchors.
type ReferenceLinkResolverFunc func(targetTemplate, id string) string

// Manager renders a (template, datafile) pair to markdown + HTML. URL
// strategies are set at construction; one Manager per target.
type Manager struct {
	templates         templateLoader
	storage           formStore
	imageURL          ImageURLFunc
	imageBase64URL    ImageBase64URLFunc
	formidableLinkURL FormidableLinkURLFunc
	referenceResolver ReferenceResolverFunc
	referenceLink     ReferenceLinkResolverFunc
	log               *slog.Logger

	// Deck cache: BuildDeck output memoized by (template, datafiles), keyed on a
	// collection revision so any write invalidates it (invariant, not a TTL). Off
	// until SetRevFunc wires a revision source. Per-Manager, since each target's
	// image/link URLs make the deck HTML differ.
	revFn     RevFunc
	deckMu    sync.Mutex
	deckCache map[string]cachedDeck

	// fontFace supplies @font-face rules for user-uploaded fonts; nil = none.
	fontFace func() (string, error)
}

// SetFontFaceProvider wires the source of @font-face CSS for user fonts (the
// fonts module). Nil leaves decks without extra font faces.
func (m *Manager) SetFontFaceProvider(fn func() (string, error)) { m.fontFace = fn }

// RevFunc returns the current collection revision (from the index). A change in
// the returned value invalidates cached decks. Nil disables deck caching.
type RevFunc func() (int64, error)

// cachedDeck is one memoized BuildDeck result plus the revision it was built at.
type cachedDeck struct {
	rev  int64
	deck RevealDeck
}

// NewManager constructs a render Manager; log may be nil, nil URL
// strategies pass through. imageBase64URL is wired separately via
// SetImageBase64URL to keep the signature stable for non-inline consumers.
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

// SetImageBase64URL wires the data-URL image strategy (inline-image mode)
// after construction; nil disables it.
func (m *Manager) SetImageBase64URL(fn ImageBase64URLFunc) {
	if m == nil {
		return
	}
	m.imageBase64URL = fn
}

// SetRevFunc wires the collection-revision source that keys the deck cache
// (nil disables caching). Set to the index Rev so any write invalidates decks.
func (m *Manager) SetRevFunc(fn RevFunc) {
	if m == nil {
		return
	}
	m.revFn = fn
}

// SetReferenceResolver wires the live api-field resolver after construction; nil
// renders api fields empty. Set after the dataprovider exists at composition.
func (m *Manager) SetReferenceResolver(fn ReferenceResolverFunc) {
	if m == nil {
		return
	}
	m.referenceResolver = fn
}

// SetReferenceLinkResolver wires the formidable:// deep-link resolver for
// api-field cards after construction; nil renders cards without the record link.
func (m *Manager) SetReferenceLinkResolver(fn ReferenceLinkResolverFunc) {
	if m == nil {
		return
	}
	m.referenceLink = fn
}

// RenderForm returns both markdown and HTML in one call. Empty datafile
// renders against the template's default values.
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

// RenderMarkdown loads the template + form and runs only the Handlebars stage.
func (m *Manager) RenderMarkdown(templateName, datafile string) (string, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return "", fmt.Errorf("render: load template %q: %w", templateName, err)
	}
	values := map[string]any{}
	var facets map[string]string
	if datafile != "" {
		if loaded := m.storage.LoadForm(templateName, datafile); loaded != nil {
			values = loaded.Data
			facets = flattenFacets(loaded.Meta.Facets)
		}
	}
	opts := m.optionsFor(templateName, datafile)
	opts.Facets = facets
	return RenderMarkdown(values, tpl, opts)
}

// flattenFacets reduces FacetState to {facetKey: selectedLabel}. Unset
// entries drop out so {{virtual-field}} surfaces empty, not a stale Selected.
func flattenFacets(in map[string]storage.FacetState) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if !v.Set {
			continue
		}
		out[k] = v.Selected
	}
	return out
}

// RenderHTMLOnly re-renders user-edited markdown (e.g. preview pane)
// without going through the form pipeline.
func (m *Manager) RenderHTMLOnly(markdown string) (string, error) {
	return RenderHTML(markdown)
}

// optionsFor builds the per-template Options bundle, capturing templateName
// in closures so the emitters don't thread it through.
func (m *Manager) optionsFor(templateName, datafile string) *Options {
	opts := &Options{
		TemplateFilename: templateName,
		Datafile:         datafile,
	}
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
		// Receives the (template, datafile) from the parsed URL, which may
		// be cross-template, not the renderer's templateName.
		opts.FormidableLinkURL = m.formidableLinkURL
	}
	if m.templates != nil {
		opts.LoadTemplate = func(name string) *template.Template {
			t, err := m.templates.LoadTemplate(name)
			if err != nil {
				return nil
			}
			return t
		}
	}
	if m.referenceResolver != nil {
		opts.ResolveReference = m.referenceResolver
	}
	if m.referenceLink != nil {
		opts.ResolveReferenceLink = m.referenceLink
	}
	return opts
}
