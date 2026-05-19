package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/api/swaggerui"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// defaultListLimit matches the original Formidable internalServer's
// behaviour: an unset/zero limit falls back to 100. Reflected in the
// response body so clients can paginate without re-reading their own
// query string.
const defaultListLimit = 100

// onlyGet returns true when the request is a GET and writes a 405
// Method Not Allowed otherwise. Used by handlers whose mux pattern
// doesn't carry a method prefix (we drop the prefix to avoid Go
// 1.22's strict-mux ambiguity warnings between literal and wildcard
// path segments).
func onlyGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", "GET")
	writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	return false
}

// listCollections answers GET /api/collections. Returns the templates
// that opt into collection mode (enable_collection=true AND a guid
// field is defined — both gates encoded in `IsCollectionEnabled`).
// Sorted by stem ASC for stability — the dataprovider already returns
// templates filename-sorted, so the slice we build here inherits that
// order.
func (h *Handler) listCollections(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	tps, err := h.dp.ListTemplates(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	rows := make([]TemplateRow, 0, len(tps))
	for _, t := range tps {
		// EnableCollection is the dataprovider's combined flag — true
		// only when the yaml says enable_collection AND the indexer
		// detected a guid field (see TemplateSummary docs). So we
		// don't need the runtime IsCollectionEnabled helper here.
		if !t.EnableCollection || t.GuidField == "" {
			continue
		}
		rows = append(rows, TemplateRow{
			ID:   t.Stem,
			Name: pickName(t),
			Href: "/api/collections/" + url.PathEscape(t.Stem),
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

// docsRedirect bounces /api/docs → /api/docs/ so the embedded shell's
// relative asset URLs resolve correctly.
func (h *Handler) docsRedirect(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	http.Redirect(w, r, "/api/docs/", http.StatusMovedPermanently)
}

// docs serves the swagger UI shell + its bundled assets, plus our
// custom back-link script. Each asset is embedded inside the
// swaggerui sub-package and addressed by basename — no traversal,
// no filesystem access.
func (h *Handler) docs(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	// strip the "/api/docs/" prefix to get the requested asset name.
	// "" → index.html (the shell). Trailing slash already handled by
	// the redirect above.
	name := strings.TrimPrefix(r.URL.Path, "/api/docs/")
	data, mime, ok := swaggerui.File(name)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}
	w.Header().Set("Content-Type", mime)
	// Cache the embedded assets — they only change on a binary
	// rebuild, so a long max-age is safe. The OpenAPI spec endpoint
	// is no-cache; this is just for the static UI shell.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// openapi answers GET /api/openapi.json. The spec is built per
// request from the live template set so Swagger UI consumers see
// schema changes the moment a template is saved — no regen step.
func (h *Handler) openapi(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	spec, err := buildOpenAPISpec(r.Context(), h.dp, h.tpl)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, spec)
}

// exportNDJSON answers GET /api/collections/{tpl}/export.ndjson. One
// line per addressable item; each line is a JSON object carrying the
// full meta + data plus the identity fields. Loaded lazily so a 304
// short-circuit on If-None-Match avoids any disk reads.
func (h *Handler) exportNDJSON(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem, tplFilename, ok := h.exportPath(w, r)
	if !ok {
		return
	}
	etag, ok := h.exportETag(w, r, tplFilename)
	if !ok {
		return
	}

	page, err := h.dp.ListCollection(r.Context(), tplFilename, dataprovider.CollectionListOpts{})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !page.Enabled {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("ETag", etag)
	// Streaming — flush after each line so curl-style consumers see
	// rows as they're produced. ResponseRecorder ignores the flusher,
	// which is fine for tests; the production server has one.
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	for _, ci := range page.Items {
		form := h.st.LoadForm(tplFilename, ci.Filename)
		row := ndjsonRow(stem, ci, form)
		if err := enc.Encode(row); err != nil {
			// Header is already on the wire; bail without re-writing
			// status. Logging happens at the http.Server level.
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// ndjsonRow builds the per-item NDJSON envelope. Mirrors the original
// Formidable internalServer shape exactly so existing consumers don't
// have to change.
func ndjsonRow(_ string, ci dataprovider.CollectionItem, form *storage.Form) map[string]any {
	row := map[string]any{
		"id":       ci.ID,
		"filename": ci.Filename,
		"title":    ci.Title,
		"meta":     map[string]any{},
		"data":     map[string]any{},
	}
	if form != nil {
		row["meta"] = formMetaAsMap(form.Meta)
		if form.Data != nil {
			row["data"] = form.Data
		}
	}
	return row
}

// exportCSV answers GET /api/collections/{tpl}/export.csv. Emits a
// utf-8 BOM (so Excel auto-detects the encoding) followed by a header
// row and one quoted row per item. Tags are joined with ';' inside
// their cell — matches the original; importers can split on it.
func (h *Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem, tplFilename, ok := h.exportPath(w, r)
	if !ok {
		return
	}
	etag, ok := h.exportETag(w, r, tplFilename)
	if !ok {
		return
	}

	page, err := h.dp.ListCollection(r.Context(), tplFilename, dataprovider.CollectionListOpts{})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !page.Enabled {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+stem+`-export.csv"`)
	w.Header().Set("ETag", etag)
	// UTF-8 BOM so spreadsheet apps detect the encoding without
	// requiring users to set the import option manually.
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	// encoding/csv handles RFC 4180 quoting/escaping — wrap a tiny
	// quote-everything writer around it so all cells are quoted (the
	// original always quoted). encoding/csv quotes only when needed
	// by default; we force-quote by prefixing/suffixing manually.
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "filename", "title", "tags"})
	for _, ci := range page.Items {
		tags := strings.Join(ci.Tags, ";")
		_ = cw.Write([]string{
			alwaysQuote(ci.ID),
			alwaysQuote(ci.Filename),
			alwaysQuote(ci.Title),
			alwaysQuote(tags),
		})
	}
	cw.Flush()
}

// alwaysQuote wraps a value in literal double-quote characters so the
// emitted CSV cell is always quoted (the original Formidable always
// quoted; consumers may rely on the shape). encoding/csv treats the
// embedded quotes as data and escapes them as ""…"" — combined with
// our wrap that yields the exact `"value"` rendering.
func alwaysQuote(s string) string {
	// Replace inner quotes with the CSV-escape pair, then wrap.
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// exportPath captures /{tpl}, validates it, and gates on
// IsCollectionEnabled. Returns ok=false after writing the appropriate
// 403/404 response — caller just bails.
func (h *Handler) exportPath(w http.ResponseWriter, r *http.Request) (stem, tplFilename string, ok bool) {
	stem = r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", "", false
	}
	tplFilename = stem + ".yaml"
	if !h.dp.IsCollectionEnabled(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return "", "", false
	}
	return stem, tplFilename, true
}

// exportETag computes the collection rev → ETag and short-circuits on
// matching If-None-Match. Returns the etag + ok=false (response
// already written) when the request was satisfied by 304 or when the
// rev lookup failed; ok=true means the caller should proceed and emit
// the body, then attach the returned etag to its Content-Type.
func (h *Handler) exportETag(w http.ResponseWriter, r *http.Request, tplFilename string) (string, bool) {
	rev, err := h.dp.CollectionRev(r.Context(), tplFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return "", false
	}
	etag := makeETag(rev)
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return etag, false
	}
	return etag, true
}

// design answers GET /api/collections/design/{tpl}. Returns the
// template's full design (metadata + field list with normalized
// options). Unlike the data endpoints, this distinguishes 404
// (template not found) from 403 (template found but collection
// disabled) — the design surface is for tooling that already knows
// the template id, so the existence-leak posture doesn't apply.
func (h *Handler) design(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusNotFound, "template-not-found")
		return
	}
	filename := stem + ".yaml"

	t, err := h.tpl.LoadTemplate(filename)
	if err != nil || t == nil {
		writeJSONError(w, http.StatusNotFound, "template-not-found")
		return
	}
	if !t.EnableCollection {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	fields, err := designFieldsFromTemplate(t.Fields)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}

	// Compute ETag from the collection rev. Same caveat as A2/A3:
	// it's coarser than per-file mtime, but the trade is acceptable
	// until per-template revs land in the index.
	rev, err := h.dp.CollectionRev(r.Context(), filename)
	if err == nil {
		w.Header().Set("ETag", makeETag(rev))
	}
	// no-store matches the original — design changes mid-session aren't
	// rare and a stale cached design would surprise tooling.
	w.Header().Set("Cache-Control", "no-store")

	name := t.Name
	if name == "" {
		name = stem
	}
	writeJSON(w, http.StatusOK, designResponse{
		Name:              name,
		Filename:          filename,
		ItemField:         t.ItemField,
		MarkdownTemplate:  t.MarkdownTemplate,
		SidebarExpression: t.SidebarExpression,
		EnableCollection:  t.EnableCollection,
		Fields:            fields,
		Facets:            projectFacets(t.Facets),
	})
}

// facets answers GET /api/collections/{tpl}/facets. Returns the
// template's filter contract — the keys, icons, and option labels
// consumers can pass on the list endpoint as `?facet.<key>=LABEL`.
// Separate from /design (which carries data-structure metadata); facet
// definitions are filter metadata. Same gating as the list endpoint:
// unknown/disabled templates are 403 (no existence leak); valid stem
// over a wrong template name returns 403 too.
func (h *Handler) facets(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	filename := stem + ".yaml"
	if !h.dp.IsCollectionEnabled(r.Context(), filename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	t, err := h.tpl.LoadTemplate(filename)
	if err != nil || t == nil {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	if rev, err := h.dp.CollectionRev(r.Context(), filename); err == nil {
		w.Header().Set("ETag", makeETag(rev))
	}
	w.Header().Set("Cache-Control", "no-store")

	out := facetsResponse{
		Template: stem,
		Facets:   projectFacets(t.Facets),
	}
	if out.Facets == nil {
		out.Facets = []facetEntry{}
	}
	writeJSON(w, http.StatusOK, out)
}

// projectFacets copies template.Facet into the wire shape. Nil/empty
// in → nil out (callers decide whether to substitute an empty slice
// for response uniformity).
func projectFacets(in []template.Facet) []facetEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]facetEntry, len(in))
	for i, f := range in {
		opts := make([]facetOptionEntry, len(f.Options))
		for j, o := range f.Options {
			opts[j] = facetOptionEntry{Label: o.Label, Color: o.Color}
		}
		out[i] = facetEntry{Key: f.Key, Icon: f.Icon, Options: opts}
	}
	return out
}

// designFieldsFromTemplate JSON-roundtrips each template.Field into a
// map so type-specific properties pass through, then overlays a
// normalized label (defaults to key) and a normalized options array.
func designFieldsFromTemplate(fields []template.Field) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		raw, err := json.Marshal(f)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		// Default label = key. JSON tag emits "label" even when empty,
		// so we always write into the existing slot.
		if s, _ := m["label"].(string); s == "" {
			m["label"] = f.Key
		}
		m["options"] = normalizeOptions(f.Options)
		out = append(out, m)
	}
	return out, nil
}

// normalizeOptions coerces template.Field.Options entries into a
// uniform [{value, label}] shape. YAML may store options as bare
// scalars (`["a", "b"]`) or maps (`[{value: a, label: A}]`), and the
// API surface presents only the latter so clients have one branch
// less to handle.
func normalizeOptions(opts []any) []designOption {
	out := make([]designOption, 0, len(opts))
	for _, o := range opts {
		switch v := o.(type) {
		case map[string]any:
			value := stringify(v["value"])
			label := value
			if l, ok := v["label"]; ok && l != nil {
				label = stringify(l)
			}
			out = append(out, designOption{Value: value, Label: label})
		default:
			s := stringify(o)
			out = append(out, designOption{Value: s, Label: s})
		}
	}
	return out
}

// stringify converts a YAML/JSON scalar to its display string. nil
// becomes empty so we don't surface "<nil>" in the API.
func stringify(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// itemAny dispatches /api/collections/{tpl}/{id} across all five
// item-level methods. Centralised so the route only needs one mux
// pattern (avoids the literal-vs-wildcard ambiguity Go 1.22's strict
// mux flags between /{id} and the peer literals /count, /design,
// /export.*, /batch).
func (h *Handler) itemAny(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.item(w, r)
	case http.MethodHead:
		h.itemHead(w, r)
	case http.MethodPut:
		h.itemPut(w, r)
	case http.MethodPatch:
		h.itemPatch(w, r)
	case http.MethodDelete:
		h.itemDelete(w, r)
	default:
		w.Header().Set("Allow", "GET, HEAD, PUT, PATCH, DELETE")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	}
}


// item answers GET /api/collections/{tpl}/{id}. Returns the full
// stored form (meta + data) plus the navigation links — same shape
// the original Formidable internalServer returned.
//
// Caching: an If-None-Match against the per-collection ETag short-
// circuits to 304 before any disk read. The ETag is derived from the
// collection's rev (currently the index-wide rev), which is coarser
// than per-file mtimes — the original used per-file stat. The trade
// is acceptable for a slice-A surface; we can refine to per-file rev
// when stat-of-form is exposed through storage.
func (h *Handler) item(w http.ResponseWriter, r *http.Request) {
	stem, id, ok := h.itemPath(w, r)
	if !ok {
		return
	}
	tplFilename := stem + ".yaml"

	rev, err := h.dp.CollectionRev(r.Context(), tplFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	etag := makeETag(rev)
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	citem, found, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not-found")
		return
	}

	// Storage returns nil when the file is missing — race between the
	// indexer and the disk. Treat as 404 so clients can simply retry.
	form := h.st.LoadForm(tplFilename, citem.Filename)
	meta := map[string]any{}
	data := map[string]any{}
	if form != nil {
		meta = formMetaAsMap(form.Meta)
		if form.Data != nil {
			data = form.Data
		}
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, itemResponse{
		Template: citem.Template,
		ID:       citem.ID,
		Filename: citem.Filename,
		Title:    citem.Title,
		Meta:     meta,
		Data:     data,
		Links: itemLinks{
			Self: citem.HrefSelf,
			HTML: citem.HrefHTML,
		},
		Rev: itemRev{ETag: etag},
	})
}

// itemHead answers HEAD /api/collections/{tpl}/{id}. Returns just the
// status + ETag — useful for clients that want to check freshness
// before pulling the full body. Mirrors the original's HEAD behaviour
// (no 304 short-circuit, since HEAD is itself the bandwidth-saving
// variant; clients use GET + If-None-Match for that).
func (h *Handler) itemHead(w http.ResponseWriter, r *http.Request) {
	stem, id, ok := h.itemPath(w, r)
	if !ok {
		return
	}
	tplFilename := stem + ".yaml"

	_, found, err := h.dp.ResolveCollectionByID(r.Context(), tplFilename, id)
	if err != nil {
		// 500 with empty body: HEAD must not return a JSON body even
		// on errors, so use writeHeaderOnly here instead of
		// writeJSONError.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	rev, err := h.dp.CollectionRev(r.Context(), tplFilename)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", makeETag(rev))
	w.WriteHeader(http.StatusOK)
}

// itemPath captures and validates {tpl} + {id}. On a bad stem or a
// disabled template it writes the 403 itself and returns ok=false so
// the caller just bails. HEAD callers want a body-less 403, so this
// helper writes status only — the GET path uses writeJSONError above
// as a separate branch.
func (h *Handler) itemPath(w http.ResponseWriter, r *http.Request) (stem, id string, ok bool) {
	stem = r.PathValue("tpl")
	id = r.PathValue("id")
	if !validStem(stem) || id == "" {
		// Match the body-vs-status posture to the request method:
		// GET → JSON error body; HEAD → header-only.
		writeStatusForMethod(w, r, http.StatusForbidden, "collection-disabled")
		return "", "", false
	}
	if !h.dp.IsCollectionEnabled(r.Context(), stem+".yaml") {
		writeStatusForMethod(w, r, http.StatusForbidden, "collection-disabled")
		return "", "", false
	}
	return stem, id, true
}

func writeStatusForMethod(w http.ResponseWriter, r *http.Request, status int, code string) {
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}
	writeJSONError(w, status, code)
}

// collectionAny dispatches /api/collections/{tpl} to GET (list) or
// POST (create). Other methods → 405. One pattern, one method-switch
// — same shape as itemAny so Go's mux stays unambiguous and the
// /count, /design, /export.* literals at the next path position
// still work.
func (h *Handler) collectionAny(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSONError(w, http.StatusMethodNotAllowed, "method-not-allowed")
	}
}


// list answers GET /api/collections/{tpl}. Supports limit / offset / q
// / tags as query params, and participates in HTTP caching via ETag +
// If-None-Match. Order of checks matters:
//   - Validate stem (defensive against router swaps).
//   - 403 if the template isn't collection-enabled. Same posture as
//     count: unknown == disabled, no existence-leak.
//   - Compute the rev → ETag and short-circuit on If-None-Match BEFORE
//     touching ListCollection. The whole point of ETag is to avoid the
//     work when the client already has fresh data.
//   - Run the list, set headers, write JSON.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	tplFilename := stem + ".yaml"
	if !h.dp.IsCollectionEnabled(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	rev, err := h.dp.CollectionRev(r.Context(), tplFilename)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	etag := makeETag(rev)
	if r.Header.Get("If-None-Match") == etag {
		// 304 must not include a body; setting the validator headers is
		// still useful so a downstream proxy can cache the bare 304.
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	opts := parseListOpts(r.URL.Query())
	if facets, errResp := h.parseFacetFilters(r, tplFilename); errResp != nil {
		writeJSON(w, errResp.status, errResp.body)
		return
	} else {
		opts.Facets = facets
	}
	page, err := h.dp.ListCollection(r.Context(), tplFilename, opts)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !page.Enabled {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}

	// Normalize the response's Limit so the body matches what the
	// handler actually applied — clients shouldn't have to know about
	// our default-fallback rule.
	if page.Limit == 0 {
		page.Limit = opts.Limit
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, page)
}

// count answers GET /api/collections/{tpl}/count. The path captures
// the stem (the URL-friendly form), not the .yaml filename — the API
// is consistently stem-keyed, matching the wiki's URL shape.
func (h *Handler) count(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	stem := r.PathValue("tpl")
	if !validStem(stem) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	tplFilename := stem + ".yaml"
	if !h.dp.IsCollectionEnabled(r.Context(), tplFilename) {
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	facets, errResp := h.parseFacetFilters(r, tplFilename)
	if errResp != nil {
		writeJSON(w, errResp.status, errResp.body)
		return
	}
	// Pull the smallest-possible page; we only need `total`. Limit=0
	// means "no slicing" — the underlying CollectionPage carries the
	// full filtered total either way.
	page, err := h.dp.ListCollection(r.Context(), tplFilename, dataprovider.CollectionListOpts{
		Limit:  1,
		Facets: facets,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal-error")
		return
	}
	if !page.Enabled {
		// Belt-and-braces — IsCollectionEnabled already returned true
		// above, so this branch is unreachable in practice. Kept so a
		// future change to ListCollection's gate doesn't silently
		// surface a 200 with a misleading total.
		writeJSONError(w, http.StatusForbidden, "collection-disabled")
		return
	}
	writeJSON(w, http.StatusOK, CountResponse{
		Template: stem,
		Total:    page.Total,
	})
}

// facetFilterError carries a parsed-failure response so the caller
// can write it through the same writeJSON path. body is shaped
// {error, key[, label]} so consumers can recognise unknown_facet vs
// unknown_facet_option uniformly.
type facetFilterError struct {
	status int
	body   map[string]any
}

// parseFacetFilters reads `facet.<key>=LABEL` query params, resolves
// them against the template's declared facets, and returns the map
// passed to dataprovider.ListCollection. Multiple params AND together.
// Unknown facet key → 400 {error:"unknown_facet", key}; label not in
// the facet's options → 400 {error:"unknown_facet_option", key, label};
// duplicate `facet.<key>=...` query params keep the first value (Go's
// url.Values returns []string in order — first wins via [0]). Empty
// label is treated as "no filter on this facet" (i.e. omitted from
// the map) so a stale URL param like ?facet.flag= doesn't shrink the
// result set unexpectedly.
func (h *Handler) parseFacetFilters(r *http.Request, tplFilename string) (map[string]string, *facetFilterError) {
	const prefix = "facet."
	q := r.URL.Query()
	var keys []string
	for k := range q {
		if strings.HasPrefix(k, prefix) && k != prefix {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	t, err := h.tpl.LoadTemplate(tplFilename)
	if err != nil || t == nil {
		return nil, &facetFilterError{
			status: http.StatusForbidden,
			body:   map[string]any{"error": "collection-disabled"},
		}
	}
	facetByKey := make(map[string]*template.Facet, len(t.Facets))
	for i := range t.Facets {
		facetByKey[t.Facets[i].Key] = &t.Facets[i]
	}
	out := make(map[string]string, len(keys))
	for _, full := range keys {
		key := strings.TrimPrefix(full, prefix)
		f, ok := facetByKey[key]
		if !ok {
			return nil, &facetFilterError{
				status: http.StatusBadRequest,
				body:   map[string]any{"error": "unknown_facet", "key": key},
			}
		}
		label := strings.TrimSpace(q.Get(full))
		if label == "" {
			continue
		}
		labelKnown := false
		for _, o := range f.Options {
			if o.Label == label {
				labelKnown = true
				break
			}
		}
		if !labelKnown {
			return nil, &facetFilterError{
				status: http.StatusBadRequest,
				body:   map[string]any{"error": "unknown_facet_option", "key": key, "label": label},
			}
		}
		out[key] = label
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// parseListOpts extracts the list-endpoint query params. Bad/empty
// numeric values fall through to the defaults — matches the original
// `Number(x) || default` semantics. Tags is comma-separated; empty
// strings between commas are dropped so `?tags=,wb,` still works.
func parseListOpts(q url.Values) dataprovider.CollectionListOpts {
	limit := atoiDefault(q.Get("limit"), defaultListLimit)
	offset := atoiDefault(q.Get("offset"), 0)
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}
	var tags []string
	if raw := strings.TrimSpace(q.Get("tags")); raw != "" {
		for t := range strings.SplitSeq(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	}
	return dataprovider.CollectionListOpts{
		Limit:  limit,
		Offset: offset,
		Q:      q.Get("q"),
		Tags:   tags,
	}
}

func atoiDefault(s string, dflt int) int {
	if s == "" {
		return dflt
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return dflt
	}
	return n
}

// formMetaAsMap projects storage.FormMeta into the on-disk JSON shape.
// Returning a map (rather than encoding the typed struct directly)
// avoids a json round-trip and keeps the wire format stable when
// FormMeta gains internal fields.
func formMetaAsMap(m storage.FormMeta) map[string]any {
	out := map[string]any{
		"id":       m.ID,
		"template": m.Template,
		"created":  auditEntryAsMap(m.Created),
		"updated":  auditEntryAsMap(m.Updated),
		"tags":     m.Tags,
	}
	if len(m.Facets) > 0 {
		out["facets"] = facetsAsMap(m.Facets)
	}
	return out
}

func facetsAsMap(in map[string]storage.FacetState) map[string]any {
	out := make(map[string]any, len(in))
	for key, state := range in {
		entry := map[string]any{"set": state.Set}
		if state.Selected != "" {
			entry["selected"] = state.Selected
		}
		out[key] = entry
	}
	return out
}

func auditEntryAsMap(a storage.AuditEntry) map[string]any {
	return map[string]any{
		"at":    a.At,
		"name":  a.Name,
		"email": a.Email,
	}
}

// makeETag formats the int64 rev as a weak ETag. Weak because the rev
// is index-wide and bumps on any write — even unrelated ones. The
// trade-off is acceptable: a few extra revalidations are cheaper than
// tracking per-collection revs in the index right now.
func makeETag(rev int64) string {
	return `W/"` + strconv.FormatInt(rev, 10) + `"`
}

// ── helpers ──────────────────────────────────────────────────────────

func pickName(t dataprovider.TemplateSummary) string {
	if t.Name != "" {
		return t.Name
	}
	return t.Stem
}

// validStem rejects empty or path-traversing segments. Go's mux
// already cleans `..` out of URL paths via 301 redirects, so this
// guard is mostly defensive against a future router swap. Mirrors
// wiki.validSegment.
func validStem(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errorBody{Error: code})
}
