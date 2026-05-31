package api

import (
	"context"
	"net/http"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/query"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Provider is the read surface the API needs from the dataprovider.
type Provider interface {
	ListTemplates(ctx context.Context) ([]dataprovider.TemplateSummary, error)
	IsCollectionEnabled(ctx context.Context, template string) bool
	ListCollection(ctx context.Context, template string, opts dataprovider.CollectionListOpts) (*dataprovider.CollectionPage, error)
	CollectionRev(ctx context.Context, template string) (int64, error)
	ResolveCollectionByID(ctx context.Context, template, id string) (*dataprovider.CollectionItem, bool, error)
}

// Storage is the bytes-side read surface: a form's meta+data and raw image
// bytes. OpenImageFile returns nil for a missing file so the handler maps it
// to 404 without sniffing the error.
type Storage interface {
	LoadForm(templateFilename, datafile string) *storage.Form
	OpenImageFile(templateFilename, name string) ([]byte, string, error)
}

// Writer is the write side (POST/PUT/PATCH/DELETE), split from Storage so an
// audit can grep "Writer" to find every write path.
type Writer interface {
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) storage.SaveResult
	DeleteForm(templateFilename, datafile string) error
}

// Templates loads a parsed template, for the design endpoint.
type Templates interface {
	LoadTemplate(name string) (*template.Template, error)
}

// Stats is the read surface for /api/statistics/*: a template's named
// objects and their evaluated grids.
type Stats interface {
	ListObjects(template string) ([]stat.StatObject, error)
	EvaluateObject(template, name string) (*stat.Grid, error)
	EvaluateComposite(template, name string) (*stat.CompositeGrid, error)
	EvaluateDSL(template, dsl string) (*stat.Grid, error)
}

// Query runs a constrained SELECT (FDRM) over a template's values.
type Query interface {
	Run(spec query.Spec) (query.Result, error)
}

// Handler exposes the /api/* routes; the mux uses fully-qualified paths so no StripPrefix is needed.
type Handler struct {
	dp    Provider
	st    Storage
	wr    Writer
	tpl   Templates
	stats Stats
	qry   Query
}

func NewHandler(dp Provider, st Storage, wr Writer, tpl Templates, stats Stats, qry Query) http.Handler {
	h := &Handler{dp: dp, st: st, wr: wr, tpl: tpl, stats: stats, qry: qry}
	mux := http.NewServeMux()
	// Go 1.22+ typed patterns, registered without a method prefix: each handler branches on
	// r.Method itself, which avoids the strict-mux ambiguity between HEAD /{tpl}/{id} and GET
	// /{tpl}/count. Literal segments outrank params, so /count matches ahead of /{id}.
	mux.HandleFunc("/api/openapi.json", h.openapi)
	mux.HandleFunc("/api/guid", h.guid)
	mux.HandleFunc("/api/docs", h.docsRedirect)
	mux.HandleFunc("/api/docs/", h.docs)
	mux.HandleFunc("/api/collections", h.listCollections)
	mux.HandleFunc("/api/collections/{tpl}", h.collectionAny)
	mux.HandleFunc("/api/collections/{tpl}/count", h.count)
	mux.HandleFunc("/api/collections/{tpl}/batch", h.batch)
	mux.HandleFunc("/api/collections/{tpl}/{id}/field/{key}", h.fieldPatch)
	// Design lives at /{tpl}/design (peer to /count), not /design/{tpl}: same path position as
	// count avoids the Go 1.22 ambiguity where /design/{tpl} and /{tpl}/count both match equally.
	mux.HandleFunc("/api/collections/{tpl}/design", h.design)
	mux.HandleFunc("/api/collections/{tpl}/facets", h.facets)
	mux.HandleFunc("/api/collections/{tpl}/query", h.query)
	mux.HandleFunc("/api/collections/{tpl}/export.ndjson", h.exportNDJSON)
	mux.HandleFunc("/api/collections/{tpl}/export.csv", h.exportCSV)
	mux.HandleFunc("/api/collections/{tpl}/{id}", h.itemAny)
	mux.HandleFunc("/api/images/{tpl}/{filename}", h.imageBytes)
	// Statistics get their own prefix so {name} can't collide with /collections/{tpl}/{id}/... under strict mux.
	mux.HandleFunc("/api/statistics/{tpl}", h.statistics)
	mux.HandleFunc("/api/statistics/{tpl}/{name}", h.statistic)
	return mux
}
