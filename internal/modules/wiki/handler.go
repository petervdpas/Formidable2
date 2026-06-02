package wiki

import (
	"context"
	"embed"
	_ "embed"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

// Provider is the read surface the wiki needs from the dataprovider.
type Provider interface {
	ListTemplates(ctx context.Context) ([]dataprovider.TemplateSummary, error)
	GetTemplate(ctx context.Context, filename string) (*dataprovider.TemplateSummary, bool, error)
	ListForms(ctx context.Context, template string, opts dataprovider.ListOpts) ([]dataprovider.FormSummary, error)
	GetFormSummary(ctx context.Context, template, datafile string) (*dataprovider.FormSummary, bool, error)
	RenderForm(ctx context.Context, template, datafile string) (*dataprovider.RenderedPage, error)
}

// Storage is the bytes side: image bytes for /storage/*, and LoadForm for the
// per-form facet/tag state the template page shows as chips (both nil on a
// missing file).
type Storage interface {
	OpenImageFile(templateFilename, name string) ([]byte, string, error)
	LoadForm(templateFilename, datafile string) *storage.Form
}

// Templates reads per-template facet definitions. Nil is tolerated: facet
// chips just don't render.
type Templates interface {
	LoadTemplate(name string) (*tpl.Template, error)
}

// Expressioner computes each row's sub-label. Nil falls back to the filename.
type Expressioner interface {
	EvaluateList(templateName string) ([]expression.Result, error)
}

// EnabledTemplateFilter hides templates disabled per-profile in Settings.
// Nil shows every template.
type EnabledTemplateFilter interface {
	IsTemplateEnabled(filename string) bool
	FilterEnabled(filenames []string) []string
}

// Handler owns the read-path routes; the concrete return type lets the composition root call optional setters.
type Handler struct {
	dp     Provider
	st     Storage
	expr   Expressioner
	tpl    Templates
	filter EnabledTemplateFilter
	mux    *http.ServeMux
}

// NewHandler builds the read-path handler; expr may be nil (filename used as subtitle).
func NewHandler(dp Provider, st Storage, expr Expressioner) *Handler {
	h := &Handler{dp: dp, st: st, expr: expr}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /template/{tpl}", h.template)
	mux.HandleFunc("GET /template/{tpl}/form/{datafile}", h.form)
	mux.HandleFunc("GET /storage/{tpl}/images/{name}", h.image)
	// /_/css/formidable-prose.css streams render.ProseCSS() so wiki bodies and the in-app
	// slideout share one stylesheet (single source of truth; see render/fulldoc.go).
	mux.HandleFunc("GET /_/{path...}", h.static)
	h.mux = mux
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// SetEnabledFilter installs (or clears with nil) the per-profile template-enablement filter.
func (h *Handler) SetEnabledFilter(f EnabledTemplateFilter) {
	h.filter = f
}

// SetTemplates installs the per-template facet-definition source (nil collapses facet UI to a no-op).
func (h *Handler) SetTemplates(t Templates) {
	h.tpl = t
}

// templateEnabled gates the detail views; missing filter or empty enabled list passes everything.
func (h *Handler) templateEnabled(filename string) bool {
	if h.filter == nil {
		return true
	}
	return h.filter.IsTemplateEnabled(filename)
}

//go:embed templates/static
var staticEmbed embed.FS

var staticFS = func() fs.FS {
	sub, err := fs.Sub(staticEmbed, "templates/static")
	if err != nil {
		// Unreachable (embed guarantees the dir); panic so a future rename surfaces immediately.
		panic("wiki: static fs setup: " + err.Error())
	}
	return sub
}()

//go:embed templates/layout.html templates/index.html templates/template.html templates/form.html
var tplFiles embed.FS

// templateFuncs: safeHTML trusts goldmark-rendered bodies past auto-escape; jsonString emits a JSON-quoted literal.
var templateFuncs = template.FuncMap{
	"safeHTML":   func(s string) template.HTML { return template.HTML(s) },
	"jsonString": jsonString,
	"facetIcon":  facetIconSVG,
}

func parsePage(name string) *template.Template {
	t := template.Must(template.New("layout.html").Funcs(templateFuncs).
		ParseFS(tplFiles, "templates/layout.html", "templates/"+name+".html"))
	return t
}

var (
	tplIndex    = parsePage("index")
	tplTemplate = parsePage("template")
	tplForm     = parsePage("form")
)

// jsonString produces a JSON-quoted string literal (hand-rolled to avoid encoding/json's trailing newline).
func jsonString(s string) template.JS {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[r>>4])
				b.WriteByte(hex[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return template.JS(b.String())
}

type indexView struct {
	Title     string
	Templates []indexTemplateRow
}
type indexTemplateRow struct {
	Stem   string
	Name   string
	Href   string
	Facets []facetPill
}

type facetPill struct {
	Key      string
	Icon     string
	Swatches []string
}

type templateView struct {
	Title   string
	Stem    string
	Name    string
	Forms   []templateFormRow
	Filters []facetFilter
}
type templateFormRow struct {
	Filename string
	Title    string
	Href     string
	TagsAttr string // data-tags="..." for filter.js
	// FacetsAttr: comma-joined "key:label" of SET facets, for data-facets="..." (filter.js).
	FacetsAttr string
	Chips      []facetChip
	// Subtitle comes from sidebar_expression when configured, else Filename.
	Subtitle        string
	SubtitleClasses string
	SubtitleColor   string
}

// facetChip is the per-row projection of one set FacetState; Color falls back to "" (neutral chip) on spec drift.
type facetChip struct {
	Key      string
	Icon     string
	Selected string
	Color    string
}

type facetFilter struct {
	Key     string
	Icon    string
	Options []facetFilterOption
}
type facetFilterOption struct {
	Label string
	Color string
}

type formView struct {
	Title    string
	Stem     string
	Filename string
	Body     string // raw HTML from the render pipeline
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	tps, err := h.dp.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.filter != nil {
		// Intersect with the enabled list via FilterEnabled so config owns the "empty list = all" semantic.
		names := make([]string, len(tps))
		for i := range tps {
			names[i] = tps[i].Filename
		}
		allowed := make(map[string]struct{}, len(names))
		for _, n := range h.filter.FilterEnabled(names) {
			allowed[n] = struct{}{}
		}
		kept := tps[:0]
		for _, t := range tps {
			if _, ok := allowed[t.Filename]; ok {
				kept = append(kept, t)
			}
		}
		tps = kept
	}
	rows := make([]indexTemplateRow, 0, len(tps))
	for _, t := range tps {
		rows = append(rows, indexTemplateRow{
			Stem:   t.Stem,
			Name:   pickName(t),
			Href:   "/template/" + t.Stem,
			Facets: h.facetPillsFor(t.Filename),
		})
	}
	writeHTML(w, tplIndex, indexView{
		Title:     "Wiki",
		Templates: rows,
	})
}

func (h *Handler) template(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	if !validSegment(stem) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	filename := stem + ".yaml"
	if !h.templateEnabled(filename) {
		// 404 not 403: don't leak the existence of a disabled template.
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	t, ok, err := h.dp.GetTemplate(r.Context(), filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok || t == nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	// filename_asc to match the storage workspace order (both views stay consistent).
	forms, err := h.dp.ListForms(r.Context(), filename, dataprovider.ListOpts{
		OrderBy: "filename_asc",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	subtitles := h.sidebarSubtitles(filename)

	facetDefs := h.facetDefsFor(filename)
	colorLookup := buildFacetColorLookup(facetDefs)

	rows := make([]templateFormRow, 0, len(forms))
	for _, f := range forms {
		row := templateFormRow{
			Filename: f.Filename,
			Title:    pickFormTitle(f),
			Href:     "/template/" + stem + "/form/" + f.Filename,
			TagsAttr: strings.Join(f.Tags, ","),
			Subtitle: f.Filename,
		}
		if item, ok := subtitles[f.Filename]; ok {
			if item.Text != "" {
				row.Subtitle = item.Text
			}
			row.SubtitleClasses = strings.Join(item.Classes, " ")
			row.SubtitleColor = item.Color
		}
		// Only hit storage when facets are declared, saving N disk reads on facet-less templates.
		if len(facetDefs) > 0 {
			row.Chips, row.FacetsAttr = h.collectFormFacets(filename, f.Filename, facetDefs, colorLookup)
		}
		rows = append(rows, row)
	}
	writeHTML(w, tplTemplate, templateView{
		Title:   pickName(*t),
		Stem:    stem,
		Name:    pickName(*t),
		Forms:   rows,
		Filters: facetFiltersFromDefs(facetDefs),
	})
}

// facetDefsFor returns the template's facet definitions, or nil when unwired / load fails / none declared.
func (h *Handler) facetDefsFor(filename string) []tpl.Facet {
	if h.tpl == nil {
		return nil
	}
	t, err := h.tpl.LoadTemplate(filename)
	if err != nil || t == nil {
		return nil
	}
	return t.Facets
}

// buildFacetColorLookup precomputes a key->label->color map so chip projection doesn't rescan per form.
func buildFacetColorLookup(defs []tpl.Facet) map[string]map[string]string {
	if len(defs) == 0 {
		return nil
	}
	out := make(map[string]map[string]string, len(defs))
	for _, f := range defs {
		inner := make(map[string]string, len(f.Options))
		for _, o := range f.Options {
			inner[o.Label] = o.Color
		}
		out[f.Key] = inner
	}
	return out
}

// collectFormFacets projects a form's SET facets into (chips, attr); set=false entries are skipped.
// Order follows the declared facet order so equal sets render identically row-to-row.
func (h *Handler) collectFormFacets(
	templateFilename, datafile string,
	defs []tpl.Facet,
	colors map[string]map[string]string,
) ([]facetChip, string) {
	if h.st == nil {
		return nil, ""
	}
	form := h.st.LoadForm(templateFilename, datafile)
	if form == nil || len(form.Meta.Facets) == 0 {
		return nil, ""
	}
	chips := make([]facetChip, 0, len(defs))
	attrs := make([]string, 0, len(defs))
	for _, f := range defs {
		state, ok := form.Meta.Facets[f.Key]
		if !ok || !state.Set {
			continue
		}
		color := ""
		if inner, ok := colors[f.Key]; ok {
			color = inner[state.Selected]
		}
		chips = append(chips, facetChip{
			Key:      f.Key,
			Icon:     f.Icon,
			Selected: state.Selected,
			Color:    color,
		})
		attrs = append(attrs, f.Key+":"+state.Selected)
	}
	return chips, strings.Join(attrs, ",")
}

// facetFiltersFromDefs projects facets into the filter-strip shape (label + colour token per option).
func facetFiltersFromDefs(defs []tpl.Facet) []facetFilter {
	if len(defs) == 0 {
		return nil
	}
	out := make([]facetFilter, 0, len(defs))
	for _, f := range defs {
		opts := make([]facetFilterOption, 0, len(f.Options))
		for _, o := range f.Options {
			opts = append(opts, facetFilterOption{Label: o.Label, Color: o.Color})
		}
		out = append(out, facetFilter{Key: f.Key, Icon: f.Icon, Options: opts})
	}
	return out
}

// facetPillsFor projects facets into the index-page display shape; nil when unwired / load fails / none.
func (h *Handler) facetPillsFor(filename string) []facetPill {
	if h.tpl == nil {
		return nil
	}
	t, err := h.tpl.LoadTemplate(filename)
	if err != nil || t == nil || len(t.Facets) == 0 {
		return nil
	}
	out := make([]facetPill, 0, len(t.Facets))
	for _, f := range t.Facets {
		sw := make([]string, 0, len(f.Options))
		for _, o := range f.Options {
			sw = append(sw, o.Color)
		}
		out = append(out, facetPill{Key: f.Key, Icon: f.Icon, Swatches: sw})
	}
	return out
}

// sidebarSubtitles returns filename->Result, or nil when unconfigured/unwired/failed (caller falls back to filename).
func (h *Handler) sidebarSubtitles(templateFilename string) map[string]expression.Result {
	if h.expr == nil {
		return nil
	}
	items, err := h.expr.EvaluateList(templateFilename)
	if err != nil {
		return nil
	}
	out := make(map[string]expression.Result, len(items))
	for _, it := range items {
		if it.Filename == "" {
			continue
		}
		out[it.Filename] = it
	}
	return out
}

func (h *Handler) form(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	datafile := r.PathValue("datafile")
	if !validSegment(stem) || !validSegment(datafile) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	filename := stem + ".yaml"

	if !h.templateEnabled(filename) {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}

	// 404 fast on missing template/form: render is expensive, gate on cheap SQLite checks first.
	if _, ok, err := h.dp.GetTemplate(r.Context(), filename); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	if _, ok, err := h.dp.GetFormSummary(r.Context(), filename, datafile); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "form not found")
		return
	}

	page, err := h.dp.RenderForm(r.Context(), filename, datafile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	title := page.Title
	if title == "" {
		title = datafile
	}
	// page.HTML already carries wiki hrefs (the render.Manager's FormidableLinkURL rewrites at the source).
	writeHTML(w, tplForm, formView{
		Title:    title,
		Stem:     stem,
		Filename: datafile,
		Body:     page.HTML,
	})
}

// static serves the embedded chrome assets at /_/, special-casing /_/css/formidable-prose.css to stream render.ProseCSS.
func (h *Handler) static(w http.ResponseWriter, r *http.Request) {
	rel := r.PathValue("path")
	if rel == "" || strings.Contains(rel, "..") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if rel == "css/formidable-prose.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write([]byte(render.ProseCSS()))
		return
	}
	if rel == "js/mermaid.min.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(render.MermaidJS())
		return
	}
	data, err := fs.ReadFile(staticFS, rel)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", staticMIME(rel))
	_, _ = w.Write(data)
}

// staticMIME maps a file extension to a MIME type; unknown extensions fall to application/octet-stream.
func staticMIME(rel string) string {
	switch path.Ext(rel) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// image serves a per-template image from storage (wiki uses HTTP URLs; the slideout uses base64 data URLs).
func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	name := r.PathValue("name")
	if !validSegment(stem) || !validSegment(name) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	templateFilename := stem + ".yaml"
	raw, mime, err := h.st.OpenImageFile(templateFilename, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if raw == nil {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	w.Header().Set("Content-Type", mime)
	_, _ = w.Write(raw)
}

// validSegment guards path captures against traversal (explicit defense; Go's mux already cleans).
func validSegment(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return false
	}
	return true
}

func pickName(t dataprovider.TemplateSummary) string {
	if t.Name != "" {
		return t.Name
	}
	return t.Stem
}

func pickFormTitle(f dataprovider.FormSummary) string {
	if f.Title != "" {
		return f.Title
	}
	if f.FmTitle != "" {
		return f.FmTitle
	}
	return f.Filename
}

func writeHTML(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil && !errors.Is(err, http.ErrAbortHandler) {
		// Header is already on the wire; append a comment rather than re-writing status.
		_, _ = w.Write([]byte("\n<!-- template error: " + err.Error() + " -->"))
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
