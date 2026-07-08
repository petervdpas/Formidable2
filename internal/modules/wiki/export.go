package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"html/template"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/render"
)

// ExportResult is the outcome of an offline-wiki bundle export: the zip bytes
// plus the stems that were skipped (a template that could not be loaded, or a
// presentation with the deck provider unwired).
type ExportResult struct {
	Zip     []byte
	Skipped []string
}

// exportEntry is one file destined for the zip.
type exportEntry struct {
	name string
	data []byte
}

// exportDeck is one slideset selected for export: its value ("" for a single
// deck presentation), display label, and ordered slide datafiles.
type exportDeck struct {
	Value     string
	Label     string
	Datafiles []string
}

// ExportBundle renders the given templates into a self-contained offline copy of
// the wiki. selections maps a template filename to the deck values to include
// (only meaningful for a presentation; an empty slice means all of its decks; it
// is ignored for a document template). The zip holds an index.html home, one
// template-<stem>.html + form-<stem>-<datafile>.html per document, one
// deck-<stem>[-<deck>].html per exported slideset, and a shared _/ assets folder.
// Every absolute server URL is rewritten to a relative path and images are
// inlined as data URIs, so the zip opens from disk with no server.
func (h *Handler) ExportBundle(ctx context.Context, selections map[string][]string) (ExportResult, error) {
	// Pull in related templates so the bundle is self-contained: a link a reader
	// clicks must resolve to a page inside the zip. This runs regardless of what
	// the caller sent, so the guarantee does not depend on the frontend.
	selections = h.expandSelections(selections)

	filenames := make([]string, 0, len(selections))
	for fn := range selections {
		if fn != "" {
			filenames = append(filenames, fn)
		}
	}
	sort.Strings(filenames) // deterministic output regardless of map order

	var entries []exportEntry
	var docRows []indexTemplateRow
	var presoRows []deckLink
	var skipped []string
	pages := 0
	usedDecks := false

	for _, fn := range filenames {
		stem := strings.TrimSuffix(fn, ".yaml")
		if h.isPresentation(fn) {
			n, err := h.exportPresentation(ctx, fn, stem, selections[fn], &entries, &presoRows)
			if err != nil {
				return ExportResult{}, err
			}
			if n == 0 {
				skipped = append(skipped, stem)
				continue
			}
			usedDecks = true
			pages += n
			continue
		}

		n, err := h.exportDocument(ctx, fn, stem, &entries, &docRows)
		if err != nil {
			return ExportResult{}, err
		}
		if n == 0 {
			skipped = append(skipped, stem)
			continue
		}
		pages += n
	}

	if pages == 0 {
		return ExportResult{}, errors.New("wiki: nothing to export")
	}

	ip, err := renderPage(tplIndex, indexView{Title: "Wiki", Templates: docRows, Presentations: presoRows})
	if err != nil {
		return ExportResult{}, err
	}
	entries = append(entries, exportEntry{name: "index.html", data: []byte(rewritePage(string(ip)))})

	assets, err := offlineAssets(usedDecks)
	if err != nil {
		return ExportResult{}, err
	}
	entries = append(entries, assets...)

	// Pack the queryable data (a SQLite image + its OpenAPI spec) for the
	// collection templates in the bundle, so the Viewer can expose an agent API
	// over it. The packer ignores non-collections; empty parts add nothing.
	if h.data != nil {
		pack, err := h.data.BuildDataPack(ctx, filenames)
		if err != nil {
			return ExportResult{}, err
		}
		if len(pack.DB) > 0 {
			entries = append(entries, exportEntry{name: "_/data.db", data: pack.DB})
		}
		if len(pack.OpenAPI) > 0 {
			entries = append(entries, exportEntry{name: "_/openapi.json", data: pack.OpenAPI})
		}
		if len(pack.Context) > 0 {
			entries = append(entries, exportEntry{name: "_/context.md", data: pack.Context})
		}
	}

	zipBytes, err := zipEntries(entries)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Zip: zipBytes, Skipped: skipped}, nil
}

// ExportPack renders the selections into the offline-wiki zip and wraps it as a
// branded .bundle: a cleartext manifest plus the payload. A non-empty password
// seals the payload (Argon2id + AES-256-GCM); an empty password stores it
// plainly but still branded, so a bundle is always a Viewer artifact rather than
// a loose zip. Returns the packed bytes and the skipped stems.
func (h *Handler) ExportPack(ctx context.Context, selections map[string][]string, password string, meta bundle.Manifest) ([]byte, []string, error) {
	res, err := h.ExportBundle(ctx, selections)
	if err != nil {
		return nil, nil, err
	}
	packed, err := bundle.Pack(meta, res.Zip, password)
	if err != nil {
		return nil, nil, err
	}
	return packed, res.Skipped, nil
}

// exportDocument writes a document template's collection page + record pages and
// appends its index row. Returns the number of pages written (0 = the template
// vanished).
func (h *Handler) exportDocument(ctx context.Context, filename, stem string, entries *[]exportEntry, rows *[]indexTemplateRow) (int, error) {
	view, ok, err := h.templateViewFor(ctx, filename, stem)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}

	page, err := renderPage(tplTemplate, view)
	if err != nil {
		return 0, err
	}
	*entries = append(*entries, exportEntry{
		name: collectionFile(stem),
		data: []byte(rewritePage(h.inlineImages(string(page)))),
	})
	*rows = append(*rows, indexTemplateRow{
		Stem:   stem,
		Name:   view.Name,
		Href:   "/template/" + stem,
		Facets: h.facetPillsFor(filename),
	})
	n := 1

	for _, row := range view.Forms {
		fv, ok, err := h.formViewFor(ctx, filename, stem, row.Filename)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		rp, err := renderPage(tplForm, fv)
		if err != nil {
			return 0, err
		}
		*entries = append(*entries, exportEntry{
			name: recordFile(stem, row.Filename),
			data: []byte(rewritePage(h.inlineImages(string(rp)))),
		})
		n++
	}
	return n, nil
}

// exportPresentation writes one deck page per selected slideset and appends each
// to the index presentations list. Returns the number of deck pages written.
func (h *Handler) exportPresentation(ctx context.Context, filename, stem string, wantDecks []string, entries *[]exportEntry, rows *[]deckLink) (int, error) {
	decks, err := h.decksToExport(filename, wantDecks)
	if err != nil {
		return 0, err
	}
	if len(decks) == 0 {
		return 0, nil
	}

	name := stem
	if t, ok, _ := h.dp.GetTemplate(ctx, filename); ok && t != nil {
		name = pickName(*t)
	}

	n := 0
	for _, ed := range decks {
		built, err := h.decks.BuildDeck(filename, ed.Datafiles)
		if err != nil {
			return 0, err
		}
		title, indexLabel := name, name
		href := "/template/" + stem + "/slides"
		if ed.Value != "" {
			title = name + " - " + ed.Label
			indexLabel = name + " : " + ed.Label
			href += "/" + ed.Value
		}
		page, err := renderPage(tplDeck, deckView{
			Title:    title,
			Body:     built.HTML,
			Width:    built.Width,
			Height:   built.Height,
			Assets:   render.DeckAssetsHash(),
			Accent:   built.Accent,
			Progress: built.Progress,
		})
		if err != nil {
			return 0, err
		}
		*entries = append(*entries, exportEntry{
			name: deckFile(stem, ed.Value),
			data: []byte(rewritePage(h.inlineImages(string(page)))),
		})
		*rows = append(*rows, deckLink{Label: indexLabel, Href: href})
		n++
	}
	return n, nil
}

// decksToExport resolves a presentation's slidesets to export, honoring want
// (empty = every deck that has slides). A single-deck presentation (no slideset
// field) yields one deck with an empty value.
func (h *Handler) decksToExport(filename string, want []string) ([]exportDeck, error) {
	if h.decks == nil {
		return nil, nil
	}
	wantSet := map[string]bool{}
	for _, v := range want {
		if v != "" {
			wantSet[v] = true
		}
	}
	decks, err := h.decks.Decks(filename)
	if err != nil {
		return nil, err
	}
	if len(decks) == 0 {
		order, err := h.decks.SequenceOrder(filename)
		if err != nil {
			return nil, err
		}
		if len(order) == 0 {
			return nil, nil
		}
		return []exportDeck{{Value: "", Datafiles: order}}, nil
	}

	var out []exportDeck
	for _, d := range decks {
		if len(wantSet) > 0 && !wantSet[d.Value] {
			continue
		}
		order, err := h.decks.DeckOrder(filename, d.Value)
		if err != nil || len(order) == 0 {
			continue
		}
		out = append(out, exportDeck{Value: d.Value, Label: d.Label, Datafiles: order})
	}
	return out, nil
}

// renderPage executes a parsed wiki page template (whose root is layout.html,
// or deck.html for the standalone deck) into a byte slice, the same entry point
// writeHTML uses for the live server.
func renderPage(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- zip-path naming (rewriter and writers must agree) -------------------

// slugRe matches runs of characters that are unsafe in a bundle filename.
var slugRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

// slug turns a datafile/deck identifier into a filename-safe token; the same
// function names the file and rewrites its href so they always match.
func slug(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(s, "-"), "-")
}

func collectionFile(stem string) string       { return "template-" + stem + ".html" }
func recordFile(stem, datafile string) string { return "form-" + stem + "-" + slug(datafile) + ".html" }

// RecordPageName is the bundle's HTML page filename for a record, matching what
// ExportBundle writes. Exported so the data packer can link each graph node to
// its page for the Viewer's detail panel.
func RecordPageName(stem, datafile string) string { return recordFile(stem, datafile) }

func deckFile(stem, deck string) string {
	if deck == "" {
		return "deck-" + stem + ".html"
	}
	return "deck-" + stem + "-" + slug(deck) + ".html"
}

// --- URL rewriting -------------------------------------------------------

var (
	reImg       = regexp.MustCompile(`/storage/([A-Za-z0-9._-]+)/images/([^"'\s)>]+)`)
	reSlideDeck = regexp.MustCompile(`/template/([A-Za-z0-9._-]+)/slides/([^"'\s)>]+)`)
	reSlides    = regexp.MustCompile(`/template/([A-Za-z0-9._-]+)/slides`)
	reForm      = regexp.MustCompile(`/template/([A-Za-z0-9._-]+)/form/([^"'\s)>]+)`)
	reTpl       = regexp.MustCompile(`/template/([A-Za-z0-9._-]+)`)
	// reAssetVer strips the ?v=<hash> cache-buster the deck page appends to its
	// asset URLs (a query breaks a file:// load).
	reAssetVer = regexp.MustCompile(`(_/[^"'?]+\.(?:css|js))\?v=[0-9a-fA-F]+`)
)

// inlineImages replaces every /storage/<stem>/images/<name> reference in a
// rendered page with a base64 data URI, so pages carry their images with no
// server. A missing image is left as-is (a broken img is better than a crash).
func (h *Handler) inlineImages(page string) string {
	return reImg.ReplaceAllStringFunc(page, func(m string) string {
		sub := reImg.FindStringSubmatch(m)
		name, err := url.PathUnescape(sub[2])
		if err != nil {
			name = sub[2]
		}
		raw, mime, err := h.st.OpenImageFile(sub[1]+".yaml", name)
		if err != nil || raw == nil {
			return m
		}
		if mime == "" {
			mime = "application/octet-stream"
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
	})
}

// rewritePage maps a rendered page's absolute server URLs to the bundle's
// relative files. Order matters: the /slides/ and /form/ rules must run before
// the bare /template/ rule (which would otherwise swallow their prefix). Every
// page sits at the zip root, so /_/x becomes _/x directly and the deck home
// link "/" becomes index.html.
func rewritePage(page string) string {
	page = reSlideDeck.ReplaceAllStringFunc(page, func(m string) string {
		sub := reSlideDeck.FindStringSubmatch(m)
		return deckFile(sub[1], sub[2])
	})
	page = reSlides.ReplaceAllStringFunc(page, func(m string) string {
		sub := reSlides.FindStringSubmatch(m)
		return deckFile(sub[1], "")
	})
	page = reForm.ReplaceAllStringFunc(page, func(m string) string {
		sub := reForm.FindStringSubmatch(m)
		return recordFile(sub[1], sub[2])
	})
	page = reTpl.ReplaceAllStringFunc(page, func(m string) string {
		sub := reTpl.FindStringSubmatch(m)
		return collectionFile(sub[1])
	})
	page = strings.ReplaceAll(page, `href="/"`, `href="index.html"`)
	page = strings.ReplaceAll(page, "/_/", "_/")
	return reAssetVer.ReplaceAllString(page, "$1")
}

// rewriteCSS maps /_/ references inside a stylesheet (which lives in _/css/) to
// paths relative to that folder: /_/img/logo.png -> ../img/logo.png.
func rewriteCSS(css string) string {
	return strings.ReplaceAll(css, "/_/", "../")
}

// --- assets --------------------------------------------------------------

// offlineCrumbsJS is the bundle's replacement for crumbs.js: it builds the
// breadcrumb from window.__FORMIDABLE__ with relative bundle links (the wiki's
// crumbs.js emits absolute /template/... paths that would break from disk).
const offlineCrumbsJS = `(function () {
  var el = document.getElementById('crumbs');
  if (!el) return;
  var meta = window.__FORMIDABLE__ || {};
  var esc = function (s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;');
  };
  var items = [{ href: 'index.html', label: 'Formidable', cls: 'root' }];
  if (meta.templateId) {
    items.push({ href: 'template-' + meta.templateId + '.html', label: meta.templateName || meta.templateId });
  }
  if (meta.formFile) {
    items.push({ href: null, label: meta.formTitle || meta.formFile });
  }
  el.innerHTML = items.map(function (p, i) {
    return (i ? '<span class="sep">/</span>' : '') +
      (p.href
        ? '<a ' + (p.cls ? 'class="' + p.cls + '"' : '') + ' href="' + p.href + '">' + esc(p.label) + '</a>'
        : '<span class="current">' + esc(p.label) + '</span>');
  }).join('');
  var q = document.getElementById('q');
  document.addEventListener('keydown', function (e) {
    if (e.key === '/' && q && !q.disabled && document.activeElement !== q) { e.preventDefault(); q.focus(); }
  });
  // Tell the Viewer shell which bundle page is showing, so it can root the
  // relations graph at the record you are on (harmless outside the Viewer).
  try {
    if (window.parent && window.parent !== window) {
      window.parent.postMessage({ formidablePage: location.pathname.replace(/^\/+/, '') }, '*');
    }
  } catch (e) {}
})();`

// offlineAssets gathers the chrome assets into the _/ folder: the wiki CSS (with
// /_/ refs rewritten relative), the render-module prose stylesheet and mermaid
// lib, the offline crumbs.js, the unchanged filter/lightbox/mermaid-init JS, and
// the logo. When withDeck is set, the reveal/katex/deck client assets are added
// too (only decks need them).
func offlineAssets(withDeck bool) ([]exportEntry, error) {
	var out []exportEntry

	for _, name := range []string{"base.css", "header.css", "content.css", "facets.css"} {
		b, err := fs.ReadFile(staticFS, "css/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, exportEntry{name: "_/css/" + name, data: []byte(rewriteCSS(string(b)))})
	}
	out = append(out, exportEntry{name: "_/css/formidable-prose.css", data: []byte(rewriteCSS(render.ProseCSS()))})

	for _, name := range []string{"filter.js", "lightbox.js", "mermaid-init.js"} {
		b, err := fs.ReadFile(staticFS, "js/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, exportEntry{name: "_/js/" + name, data: b})
	}
	out = append(out, exportEntry{name: "_/js/crumbs.js", data: []byte(offlineCrumbsJS)})
	out = append(out, exportEntry{name: "_/js/mermaid.min.js", data: render.MermaidJS()})

	logo, err := fs.ReadFile(staticFS, "img/logo.png")
	if err != nil {
		return nil, err
	}
	out = append(out, exportEntry{name: "_/img/logo.png", data: logo})

	if withDeck {
		deck, err := deckAssets()
		if err != nil {
			return nil, err
		}
		out = append(out, deck...)
	}
	return out, nil
}

// deckAssets gathers the reveal.js + KaTeX + deck client assets for a bundle
// that includes at least one slideset. The KaTeX dist keeps its directory shape
// under _/katex/ so katex.min.css's relative fonts/ URLs still resolve.
func deckAssets() ([]exportEntry, error) {
	var out []exportEntry

	deckPage, err := fs.ReadFile(staticFS, "css/deck-page.css")
	if err != nil {
		return nil, err
	}
	out = append(out, exportEntry{name: "_/css/deck-page.css", data: []byte(rewriteCSS(string(deckPage)))})
	out = append(out, exportEntry{name: "_/css/reveal.css", data: []byte(rewriteCSS(render.RevealCSS()))})
	out = append(out, exportEntry{name: "_/css/deck.css", data: []byte(rewriteCSS(render.DeckCSS()))})
	out = append(out, exportEntry{name: "_/js/reveal.js", data: render.RevealJS()})
	out = append(out, exportEntry{name: "_/js/deck-init.js", data: render.DeckInitJS()})

	kfs := render.KatexFS()
	err = fs.WalkDir(kfs, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(kfs, p)
		if err != nil {
			return err
		}
		out = append(out, exportEntry{name: "_/katex/" + p, data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// zipEntries writes the entries into a zip archive in the given order. No
// timestamps are set, so identical input yields identical bytes.
func zipEntries(entries []exportEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
