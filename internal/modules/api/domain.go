package api

import (
	"context"
	"net/http"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Provider is the narrow read surface the API handler needs from the
// dataprovider. Declared as an interface so unit tests can supply a
// hand-rolled stub without standing up SQLite + the real index. The
// real *dataprovider.Manager satisfies this without an adapter.
type Provider interface {
	ListTemplates(ctx context.Context) ([]dataprovider.TemplateSummary, error)
	IsCollectionEnabled(ctx context.Context, template string) bool
	ListCollection(ctx context.Context, template string, opts dataprovider.CollectionListOpts) (*dataprovider.CollectionPage, error)
	CollectionRev(ctx context.Context, template string) (int64, error)
	ResolveCollectionByID(ctx context.Context, template, id string) (*dataprovider.CollectionItem, bool, error)
}

// Storage is the bytes-side surface the API needs:
//   - LoadForm: meta + data block for /api/collections/{tpl}/{id}
//   - OpenImageFile: raw image bytes + MIME for /api/images/{tpl}/{filename}
//
// The real *storage.Manager satisfies this without an adapter.
// OpenImageFile returns (nil, "", nil) for a missing file so the api
// handler can map that to 404 without sniffing the error string.
type Storage interface {
	LoadForm(templateFilename, datafile string) *storage.Form
	OpenImageFile(templateFilename, name string) ([]byte, string, error)
}

// Writer is the write-side surface used by the POST/PUT/PATCH/DELETE
// endpoints. Kept separate from Storage so a future security audit
// can grep "Writer" to find every write path. The real *storage.Manager
// satisfies both - the indexer hook attached to SaveForm/DeleteForm
// keeps SQLite consistent without the api module having to know about
// the index at all.
type Writer interface {
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) storage.SaveResult
	DeleteForm(templateFilename, datafile string) error
}

// Templates is the surface the API needs to answer the design
// endpoint: the full parsed template (fields + metadata). The real
// *template.Manager already exposes LoadTemplate with this signature.
type Templates interface {
	LoadTemplate(name string) (*template.Template, error)
}

// Stats is the read surface for the /api/statistics/* endpoints: the
// template's named statistical objects and their evaluated grids. The
// real *stat.Service satisfies it; the engine owns the aggregation, the
// API just serves the JSON so external consumers (R, scripts) can fetch
// the same presentation-free grids the UI renders.
type Stats interface {
	ListObjects(template string) ([]stat.StatObject, error)
	EvaluateObject(template, name string) (*stat.Grid, error)
	EvaluateComposite(template, name string) (*stat.CompositeGrid, error)
	EvaluateDSL(template, dsl string) (*stat.Grid, error)
}

// Handler exposes the /api/* routes as an http.Handler. The composition
// root mounts the returned handler at the root mux's "/api/" prefix -
// the api mux itself uses fully-qualified paths so no StripPrefix is
// needed.
type Handler struct {
	dp    Provider
	st    Storage
	wr    Writer
	tpl   Templates
	stats Stats
}

// NewHandler builds the API handler. Returns the underlying mux as
// http.Handler so callers compose it through the standard interface;
// route shapes stay private to this file and can be evolved without
// rippling out.
func NewHandler(dp Provider, st Storage, wr Writer, tpl Templates, stats Stats) http.Handler {
	h := &Handler{dp: dp, st: st, wr: wr, tpl: tpl, stats: stats}
	mux := http.NewServeMux()
	// Go 1.22+ typed patterns. Full paths (incl. "/api") so the
	// composition root can mount this at the root mux without
	// StripPrefix. Literal segments take precedence over param
	// segments, so /count is matched before /{id}.
	// Patterns are registered without method prefix so HEAD on
	// /{tpl}/{id} doesn't theoretically overlap with GET on
	// /{tpl}/count under Go's strict mux ambiguity check. Each handler
	// branches on r.Method itself and returns 405 for unsupported
	// methods.
	mux.HandleFunc("/api/openapi.json", h.openapi)
	// Server-minted UUID v4. Lets clients hit one endpoint to obtain a
	// fresh GUID for create flows instead of bundling a UUID library.
	// POST /api/collections/{tpl} also auto-mints when the body's
	// data[guidKey] is empty, so this endpoint is a convenience, not
	// a requirement.
	mux.HandleFunc("/api/guid", h.guid)
	// Swagger UI - `/api/docs` redirects to the trailing-slash form so
	// the embedded HTML's relative `/api/docs/<asset>` script tags work
	// (mux uses literal pattern matching on the directory boundary).
	mux.HandleFunc("/api/docs", h.docsRedirect)
	mux.HandleFunc("/api/docs/", h.docs)
	mux.HandleFunc("/api/collections", h.listCollections)
	mux.HandleFunc("/api/collections/{tpl}", h.collectionAny)
	mux.HandleFunc("/api/collections/{tpl}/count", h.count)
	mux.HandleFunc("/api/collections/{tpl}/batch", h.batch)
	mux.HandleFunc("/api/collections/{tpl}/{id}/field/{key}", h.fieldPatch)
	// Design lives at /{tpl}/design (peer to /count) rather than the
	// original Formidable's /design/{tpl}. Putting "design" and "count"
	// at the same path position avoids Go 1.22's strict-mux conflict
	// - `/design/{tpl}` and `/{tpl}/count` would otherwise both match
	// `/collections/design/count` with equal precedence. Tooling that
	// previously hit /design/<id> just swaps to /<id>/design.
	mux.HandleFunc("/api/collections/{tpl}/design", h.design)
	mux.HandleFunc("/api/collections/{tpl}/facets", h.facets)
	mux.HandleFunc("/api/collections/{tpl}/export.ndjson", h.exportNDJSON)
	mux.HandleFunc("/api/collections/{tpl}/export.csv", h.exportCSV)
	mux.HandleFunc("/api/collections/{tpl}/{id}", h.itemAny)
	// Image bytes (or data-URL string with ?format=url). Reused by the
	// slideout's <img src=…> via Wails AssetMiddleware so the markdown
	// stays free of inlined base64.
	mux.HandleFunc("/api/images/{tpl}/{filename}", h.imageBytes)
	// Statistics live under their own prefix (not the collections subtree)
	// so the {name} segment can't collide with /collections/{tpl}/{id}/...
	// under Go's strict-mux ambiguity check. GET lists the named objects;
	// GET /{name} evaluates one to its JSON grid (or composite grid); POST
	// evaluates an ad-hoc DSL.
	mux.HandleFunc("/api/statistics/{tpl}", h.statistics)
	mux.HandleFunc("/api/statistics/{tpl}/{name}", h.statistic)
	return mux
}
