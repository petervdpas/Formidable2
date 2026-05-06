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
	"github.com/petervdpas/formidable2/internal/modules/render"
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

// Storage is the bytes-side surface the wiki uses for `/storage/*`.
// The method shape mirrors `*storage.Manager.OpenImageFile` so the
// real manager satisfies it without an adapter; tests pass in a
// stub. Returns nil bytes (not an error) when the file is missing —
// mirrors LoadForm's "missing isn't an error" semantics and lets
// the handler decide on the 404 status.
type Storage interface {
	OpenImageFile(templateFilename, name string) ([]byte, string, error)
}

// Handler owns the read-path routes. NewHandler returns an
// http.Handler the wiki Manager can SetHandler with — keeping the
// router shape internal to this file so route signatures stay
// changeable without rippling out.
type Handler struct {
	dp Provider
	st Storage
}

// NewHandler builds the read-path handler. Returns an http.Handler
// (the underlying *http.ServeMux), so callers compose it through the
// standard handler interface — no wiki-specific glue at the seam.
func NewHandler(dp Provider, st Storage) http.Handler {
	h := &Handler{dp: dp, st: st}
	mux := http.NewServeMux()
	// Go 1.22+ typed patterns — method + path-segment captures, no
	// extra router dependency.
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /template/{tpl}", h.template)
	mux.HandleFunc("GET /template/{tpl}/form/{datafile}", h.form)
	mux.HandleFunc("GET /storage/{tpl}/images/{name}", h.image)
	// Embedded chrome (CSS / JS / images) the wiki templates reference.
	// `/_/css/formidable-prose.css` is a special pseudo-file: it streams
	// the bytes from render.ProseCSS() so the same stylesheet that
	// styles the in-app slideout body styles wiki form bodies — single
	// source of truth (see render/fulldoc.go and DRY commitment).
	mux.HandleFunc("GET /_/{path...}", h.static)
	return mux
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
// The form *body* always comes from render.Manager — wiki never
// invokes raymond/goldmark itself.

//go:embed templates/layout.html templates/index.html templates/template.html templates/form.html
var tplFiles embed.FS

// templateFuncs are the shared funcs every page template needs.
//
//   - safeHTML: lets a pre-rendered body string bypass html/template's
//     auto-escape — used only for `dataprovider.RenderedPage.HTML`,
//     which came out of goldmark and is therefore trusted.
//   - jsonString: emits a Go string as a JSON-quoted literal so the
//     `meta` block can produce valid JSON without ad-hoc escaping
//     (used by crumbs.js' window.__FORMIDABLE__).
var templateFuncs = template.FuncMap{
	"safeHTML":   func(s string) template.HTML { return template.HTML(s) },
	"jsonString": jsonString,
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

// jsonString produces `"escaped"` for a string. Cheap hand-roll —
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
}

// templateView is what template.html binds against. BackHref is gone
// — the topbar's history nav-buttons handle it now.
type templateView struct {
	Title string
	Stem  string
	Name  string
	Forms []templateFormRow
}
type templateFormRow struct {
	Filename string
	Title    string
	Href     string
	// TagsAttr is the comma-joined tag list emitted into the
	// `data-tags="..."` attribute. filter.js reads this to drive the
	// live tag/text filter on the forms list.
	TagsAttr string
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
	rows := make([]indexTemplateRow, 0, len(tps))
	for _, t := range tps {
		rows = append(rows, indexTemplateRow{
			Stem: t.Stem,
			Name: pickName(t),
			Href: "/template/" + t.Stem,
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
	t, ok, err := h.dp.GetTemplate(r.Context(), filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok || t == nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	// Match the storage workspace's order — filesystem readdir is
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
	rows := make([]templateFormRow, 0, len(forms))
	for _, f := range forms {
		rows = append(rows, templateFormRow{
			Filename: f.Filename,
			Title:    pickFormTitle(f),
			Href:     "/template/" + stem + "/form/" + f.Filename,
			TagsAttr: strings.Join(f.Tags, ","),
		})
	}
	writeHTML(w, tplTemplate, templateView{
		Title: pickName(*t),
		Stem:  stem,
		Name:  pickName(*t),
		Forms: rows,
	})
}

func (h *Handler) form(w http.ResponseWriter, r *http.Request) {
	stem := r.PathValue("tpl")
	datafile := r.PathValue("datafile")
	if !validSegment(stem) || !validSegment(datafile) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	filename := stem + ".yaml"

	// 404 fast on a missing template or missing form — render is
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
// through the same render pipeline — only the image strategy
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
		// Don't double-write — the header is already on the wire.
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
