package wiki

import (
	"context"
	_ "embed"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
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
// The real `*storage.Manager.OpenImageFile` satisfies it; tests pass
// in a stub. Returns nil bytes (not an error) when the file is
// missing — mirrors LoadForm's "missing isn't an error" semantics
// and lets the handler decide on the 404 status.
type Storage interface {
	// OpenImage returns the raw bytes + MIME type for an image, or
	// (nil, "", nil) when the file is missing.
	OpenImage(templateFilename, name string) ([]byte, string, error)
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
	return mux
}

// ── HTML view layer ──────────────────────────────────────────────────
//
// The chrome (page wrapper, sidebar, breadcrumb) is rendered with
// html/template — wiki owns this; the form *body* always comes from
// render.Manager via dataprovider.RenderForm. Keeping the chrome
// templates alongside the handler keeps view+route co-located.

//go:embed templates/index.html
var tplIndexSrc string

//go:embed templates/template.html
var tplTemplateSrc string

//go:embed templates/form.html
var tplFormSrc string

var (
	tplIndex    = template.Must(template.New("index").Parse(tplIndexSrc))
	tplTemplate = template.Must(template.New("template").Parse(tplTemplateSrc))
	tplForm     = template.Must(template.New("form").Funcs(template.FuncMap{
		// `safe` lets the form body bypass html/template's auto-escaping —
		// the body already came out of render.Manager, which produces
		// trusted (Goldmark-rendered) HTML. This is the one and only
		// `template.HTML` cast in the wiki module.
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}).Parse(tplFormSrc))
)

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

// templateView is what template.html binds against.
type templateView struct {
	Title    string
	Stem     string
	Name     string
	Forms    []templateFormRow
	BackHref string
}
type templateFormRow struct {
	Filename string
	Title    string
	Href     string
}

// formView is what form.html binds against.
type formView struct {
	Title    string
	Stem     string
	Filename string
	Body     string // raw HTML from the render pipeline
	BackHref string
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
	forms, err := h.dp.ListForms(r.Context(), filename, dataprovider.ListOpts{
		OrderBy: "updated_desc",
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
		})
	}
	writeHTML(w, tplTemplate, templateView{
		Title:    pickName(*t),
		Stem:     stem,
		Name:     pickName(*t),
		Forms:    rows,
		BackHref: "/",
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
		BackHref: "/template/" + stem,
	})
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
	raw, mime, err := h.st.OpenImage(templateFilename, name)
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
