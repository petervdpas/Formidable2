package wiki

import (
	"context"
	_ "embed"
	"embed"
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

// Provider is the narrow read-only surface the wiki handler needs
// from `*dataprovider.Manager`. Declared here as an interface so the
// HTTP unit tests can swap in a hand-rolled stub without spinning up
// SQLite + the full module graph. The composition root passes the
// real `*dataprovider.Manager`, which already satisfies this shape.
type Provider interface {
	ListTemplates(ctx context.Context) ([]dataprovider.TemplateSummary, error)
	GetTemplate(ctx context.Context, filename string) (*dataprovider.TemplateSummary, bool, error)
	ListForms(ctx context.Context, template string, opts dataprovider.ListOpts) ([]dataprovider.FormSummary, error)
	GetFormSummary(ctx context.Context, template, datafile string) (*dataprovider.FormSummary, bool, error)
	RenderForm(ctx context.Context, template, datafile string) (*dataprovider.RenderedPage, error)
}

// Storage is the bytes-side surface the wiki uses for `/storage/*`
// and per-form facet state on the template detail page.
//
//   - OpenImageFile: image bytes (nil/empty on missing) for /storage/*.
//   - LoadForm: per-form metadata (meta.Facets, meta.Tags) used by the
//     template page to render facet chips next to each row. Returns nil
//     when the form doesn't exist, mirroring the storage manager's own
//     contract.
//
// The real `*storage.Manager` satisfies both without an adapter; tests
// pass in stubs.
type Storage interface {
	OpenImageFile(templateFilename, name string) ([]byte, string, error)
	LoadForm(templateFilename, datafile string) *storage.Form
}

// Templates is the surface the wiki needs to read per-template facet
// definitions (template.Template.Facets). The real `*template.Manager`
// satisfies this directly; the wiki handler tolerates nil for backwards
// compatibility with old tests - facet chips simply don't render then.
type Templates interface {
	LoadTemplate(name string) (*tpl.Template, error)
}

// Expressioner is the sidebar-expression surface the wiki needs. The
// real `*expression.Manager` satisfies it directly; tests pass a stub.
// May be nil - the form list then falls back to the bare filename for
// every row.
type Expressioner interface {
	EvaluateList(templateName string) ([]expression.Result, error)
}

// EnabledTemplateFilter is the per-profile template curation surface
// the wiki consults to hide templates the user has marked as disabled
// in Settings → Templates. `*config.Manager` satisfies this directly;
// composition root wires it via SetEnabledFilter. Nil disables filtering
// - every template the dataprovider exposes is visible.
type EnabledTemplateFilter interface {
	IsTemplateEnabled(filename string) bool
	FilterEnabled(filenames []string) []string
}

// Handler owns the read-path routes. NewHandler returns *Handler (which
// satisfies http.Handler), keeping the router shape internal to this
// file so route signatures stay changeable without rippling out. The
// concrete return type also lets the composition root call optional
// setters (e.g. SetEnabledFilter) after construction.
type Handler struct {
	dp     Provider
	st     Storage
	expr   Expressioner
	tpl    Templates
	filter EnabledTemplateFilter
	mux    *http.ServeMux
}

// NewHandler builds the read-path handler. `expr` may be nil - wiki then
// renders the filename as subtitle. Filtering is off by default; call
// SetEnabledFilter to wire the per-profile template enablement. The
// Templates surface (per-template facet definitions) is installed later
// via SetTemplates so old call sites that don't pass it keep compiling;
// without it facet chips just don't render.
func NewHandler(dp Provider, st Storage, expr Expressioner) *Handler {
	h := &Handler{dp: dp, st: st, expr: expr}
	mux := http.NewServeMux()
	// Go 1.22+ typed patterns - method + path-segment captures, no
	// extra router dependency.
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /template/{tpl}", h.template)
	mux.HandleFunc("GET /template/{tpl}/form/{datafile}", h.form)
	mux.HandleFunc("GET /storage/{tpl}/images/{name}", h.image)
	// Embedded chrome (CSS / JS / images) the wiki templates reference.
	// `/_/css/formidable-prose.css` is a special pseudo-file: it streams
	// the bytes from render.ProseCSS() so the same stylesheet that
	// styles the in-app slideout body styles wiki form bodies - single
	// source of truth (see render/fulldoc.go and DRY commitment).
	mux.HandleFunc("GET /_/{path...}", h.static)
	h.mux = mux
	return h
}

// ServeHTTP makes *Handler satisfy http.Handler. Delegates to the
// internal mux built by NewHandler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// SetEnabledFilter installs (or clears, with nil) the per-profile
// template-enablement filter. The wiki's list and detail views consult
// it on every request - a profile switch in the running app takes
// effect on the next page load with no re-init.
func (h *Handler) SetEnabledFilter(f EnabledTemplateFilter) {
	h.filter = f
}

// SetTemplates installs the per-template facet-definition source. May
// be cleared with nil - facet pills/chips/filter then collapse to a
// no-op, but the rest of the wiki keeps rendering. The composition
// root passes `*template.Manager`; old tests that pre-date facets
// simply skip this call.
func (h *Handler) SetTemplates(t Templates) {
	h.tpl = t
}

// templateEnabled is the centralized gate the detail views use: missing
// filter or empty enabled list → everything passes; otherwise membership
// check. Empty filename is never enabled - same semantic as the config
// manager, kept consistent so 404s match the user's mental model.
func (h *Handler) templateEnabled(filename string) bool {
	if h.filter == nil {
		return true
	}
	return h.filter.IsTemplateEnabled(filename)
}

// staticFS holds the embedded `templates/static/` tree (CSS, JS, img).
// Served at /_/<path>. The fs.Sub strips the leading "templates/static/"
// so URLs map cleanly: /_/css/base.css → templates/static/css/base.css.
//
//go:embed templates/static
var staticEmbed embed.FS

var staticFS = func() fs.FS {
	sub, err := fs.Sub(staticEmbed, "templates/static")
	if err != nil {
		// embed.FS guarantees the dir exists at compile time, so this
		// branch is unreachable. Panic anyway so a future structural
		// rename surfaces immediately.
		panic("wiki: static fs setup: " + err.Error())
	}
	return sub
}()

// ── HTML view layer ──────────────────────────────────────────────────
//
// Each page template (`index`, `template`, `form`) overrides the
// `title`, `meta`, and `content` blocks defined in the shared
// `layout.html`. Layout owns the topbar (logo + breadcrumbs + search)
// and the <link>/<script> tags pointing at the embedded chrome assets.
// The form *body* always comes from render.Manager - wiki never
// invokes raymond/goldmark itself.

//go:embed templates/layout.html templates/index.html templates/template.html templates/form.html
var tplFiles embed.FS

// templateFuncs are the shared funcs every page template needs.
//
//   - safeHTML: lets a pre-rendered body string bypass html/template's
//     auto-escape - used only for `dataprovider.RenderedPage.HTML`,
//     which came out of goldmark and is therefore trusted.
//   - jsonString: emits a Go string as a JSON-quoted literal so the
//     `meta` block can produce valid JSON without ad-hoc escaping
//     (used by crumbs.js' window.__FORMIDABLE__).
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

// jsonString produces `"escaped"` for a string. Cheap hand-roll -
// pulling encoding/json just for this is overkill and would require
// trimming the leading/trailing newline anyway.
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

// indexView is what the index.html template binds against.
type indexView struct {
	Title     string
	Templates []indexTemplateRow
}
type indexTemplateRow struct {
	Stem string
	Name string
	Href string
	// Facets is the per-template facet contract projected for display.
	// Empty when the template declares none; the html/template treats
	// the surrounding block as a no-op in that case so a row without
	// facets renders no extra HTML.
	Facets []facetPill
}

// facetPill is the index-page projection of one Facet definition: the
// key, the FontAwesome icon class (rendered as a CSS hook even when the
// wiki layout itself doesn't load FA), and the option-colour tokens
// emitted as small swatches under the pill so the row reads like a
// quick filter contract.
type facetPill struct {
	Key      string
	Icon     string
	Swatches []string
}

// templateView is what template.html binds against. BackHref is gone
// - the topbar's history nav-buttons handle it now.
type templateView struct {
	Title string
	Stem  string
	Name  string
	Forms []templateFormRow
	// Filters lists the template's facet definitions for the filter
	// strip above the form list. Empty when the template has no facets
	// - the surrounding template block then renders nothing.
	Filters []facetFilter
}
type templateFormRow struct {
	Filename string
	Title    string
	Href     string
	// TagsAttr is the comma-joined tag list emitted into the
	// `data-tags="..."` attribute. filter.js reads this to drive the
	// live tag/text filter on the forms list.
	TagsAttr string
	// FacetsAttr is the comma-joined "key:label" list of facets that
	// are SET on this form (set=false entries are skipped). Emitted
	// into `data-facets="..."` for filter.js. Empty string when no
	// facets are set.
	FacetsAttr string
	// Chips are the visible per-row badges for SET facets. Empty when
	// the form has no facets set.
	Chips []facetChip
	// Subtitle is the per-row sub-label rendered under Title. Comes
	// from the template's sidebar_expression when configured; falls
	// back to Filename otherwise. SubtitleClasses + SubtitleColor
	// mirror the in-app sidebar's expression chip styling.
	Subtitle        string
	SubtitleClasses string
	SubtitleColor   string
}

// facetChip is the per-row projection of one set FacetState. Color is
// the option's colour token (looked up from the template's facet
// definition for the matching Selected label); falls back to empty
// string when the label isn't in the def (record drifted from spec) -
// the CSS class then collapses to a neutral chip.
type facetChip struct {
	Key      string
	Icon     string
	Selected string
	Color    string
}

// facetFilter is the strip-control projection of one Facet. Options
// are the user-visible labels; the corresponding colour tokens are
// emitted as data-color-<label> attributes on the <select> so the JS
// can paint the active selection without re-fetching the contract.
type facetFilter struct {
	Key     string
	Icon    string
	Options []facetFilterOption
}
type facetFilterOption struct {
	Label string
	Color string
}

// formView is what form.html binds against.
type formView struct {
	Title    string
	Stem     string
	Filename string
	Body     string // raw HTML from the render pipeline
}

// ── handlers ─────────────────────────────────────────────────────────

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	tps, err := h.dp.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.filter != nil {
		// Project to filenames, intersect with the enabled list, then
		// rehydrate. Cheaper than per-row IsTemplateEnabled when the
		// allowlist is small relative to the corpus, and lets the config
		// helper own the "empty list = all enabled" semantic.
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
		// 404 (not 403) on disabled templates: don't leak the existence
		// of a template the user disabled - same shape as "template not
		// found", same response to teammates following an old link.
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
	// Match the storage workspace's order - filesystem readdir is
	// alphabetical-by-filename, and that's what the original
	// Formidable wiki used too. Both views render the same template's
	// forms in the same order; the user's mental model stays stable.
	forms, err := h.dp.ListForms(r.Context(), filename, dataprovider.ListOpts{
		OrderBy: "filename_asc",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Sidebar expression - same evaluator the in-app storage workspace
	// uses, keyed by filename. Failures are logged-best-effort: if the
	// template has no expression (ErrNoExpression) or the engine isn't
	// wired, we just fall back to filename subtitles.
	subtitles := h.sidebarSubtitles(filename)

	// Facet contract for this template - drives both the per-row chips
	// (icon + colour lookup keyed by selected option) and the filter
	// strip above the list. Nil tpl or no facets ⇒ empty.
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
		// Only consult storage when the template actually declares
		// facets - saves N disk reads on the typical facet-less template.
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

// facetDefsFor returns the template's facet definitions, or nil when
// the Templates surface isn't wired / the load fails / the template
// declares none. Mirrors facetPillsFor but returns the raw Facet slice
// because the template page needs both icon + options.
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

// buildFacetColorLookup pre-computes a key→label→color map so the
// per-row chip projection doesn't re-scan the def slice for every form
// + selected option. Records with a Selected that's no longer in the
// def gracefully fall through to "" (rendered as a neutral chip).
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

// collectFormFacets reads one form via storage and projects its SET
// facets into (chips, attrAttr). set=false entries are skipped entirely
// - they're indistinguishable from "facet doesn't apply" for display
// purposes. Order follows the template's declared facet order so two
// forms with the same set look identical row-to-row.
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

// facetFiltersFromDefs projects the template's facets into the filter
// strip's binding shape: every option label + its colour token, so the
// JS can render a coloured marker next to the active selection.
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

// facetPillsFor projects the template's facets into the index-page
// display shape. Returns nil when the Templates surface isn't wired,
// when LoadTemplate fails, or when the template has no facets - the
// caller's `{{if .Facets}}` block then renders nothing extra.
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

// sidebarSubtitles returns filename → Result for the given
// template. Returns nil when no expression is configured, the
// evaluator wasn't wired, or evaluation failed at the source level -
// the caller falls back to filename subtitles in any of those cases.
// Per-row errors are surfaced as item.Error and still keyed in.
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

	// 404 fast on a missing template or missing form - render is
	// expensive; keep it gated on cheap SQLite checks first.
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
	// `page.HTML` already carries `/template/<stem>/form/<datafile>`
	// hrefs because the wiki's render.Manager was constructed with a
	// FormidableLinkURL strategy that rewrites at the source (see
	// internal/app/app.go's `wikiRender`). No post-process needed.
	writeHTML(w, tplForm, formView{
		Title:    title,
		Stem:     stem,
		Filename: datafile,
		Body:     page.HTML,
	})
}

// static serves the embedded chrome assets (CSS / JS / logo) at /_/.
// The path is captured wholesale via the {path...} pattern; we then
// guard against traversal (Go's mux already cleans, but explicit
// rejection is cheap and audit-friendly), special-case
// /_/css/formidable-prose.css to stream from render.ProseCSS, and
// otherwise serve the embedded byte slice with the right MIME.
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
	data, err := fs.ReadFile(staticFS, rel)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", staticMIME(rel))
	_, _ = w.Write(data)
}

// staticMIME maps the file extension to a MIME type. Limited to the
// types we actually ship; unknown extensions get the generic
// application/octet-stream so nothing crashes on a stray file.
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

// image serves a per-template image from storage. The wiki context
// uses regular HTTP image URLs so the browser can cache them and so
// the page HTML stays slim. The in-app slideout uses base64 data
// URLs (set by the composition root's render locator); both flow
// through the same render pipeline - only the image strategy
// differs.
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

// ── helpers ──────────────────────────────────────────────────────────

// validSegment guards against ../ and other oddities in path
// captures. Go's mux already strips traversal in the URL path, but
// the helper is explicit defense + makes the negative test scenarios
// readable.
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
		// Don't double-write - the header is already on the wire.
		// Log via a panic so the manager's serve goroutine surfaces it.
		// In practice this should never fire on the handcrafted templates.
		_, _ = w.Write([]byte("\n<!-- template error: " + err.Error() + " -->"))
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
