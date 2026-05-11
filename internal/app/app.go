// Package app is Formidable's composition root.
//
// It constructs every domain module with its dependencies, exposes the
// Wails service list to main.go, and (later) registers the loopback
// HTTP routes for the modules that opt into them.
package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"context"
	"strings"

	applog "github.com/petervdpas/formidable2/internal/log"
	"github.com/petervdpas/formidable2/internal/modules/about"
	"github.com/petervdpas/formidable2/internal/modules/api"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/credential"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/git"
	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/csv"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/dialog"
	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/i18n"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/integrity"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/logging"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/monitor"
	"github.com/petervdpas/formidable2/internal/modules/nav"
	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
)

// EmitFunc bridges journal events to whatever transport the host
// app uses (Wails, stdout, …). main.go installs the Wails-backed
// implementation via App.SetEmit after the Wails app is built.
type EmitFunc func(name string, data any)

// emitterRelay is the journal.EventEmitter the journal manager holds
// for its lifetime. The actual transport (Wails) is installed later
// because the Wails app doesn't exist when journal.NewManager runs.
type emitterRelay struct {
	mu sync.RWMutex
	fn EmitFunc
}

func (e *emitterRelay) Emit(name string, data any) {
	e.mu.RLock()
	fn := e.fn
	e.mu.RUnlock()
	if fn != nil {
		fn(name, data)
	}
}

func (e *emitterRelay) set(fn EmitFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fn = fn
}

type Deps struct {
	AppRoot        string
	Logger         *slog.Logger
	LogBroadcaster *applog.Broadcaster
}

type App struct {
	About    *about.Service
	System   *system.Service
	Config   *config.Service
	Sfr      *sfr.Service
	Journal  *journal.Service
	Csv      *csv.Service
	Template *template.Service
	Storage  *storage.Service
	Form     *form.Service
	I18n     *i18n.Service
	Dialog   *dialog.Service
	Render       *render.Service
	Nav          *nav.Service
	Wiki         *wiki.Service
	Dataprovider *dataprovider.Service
	Plugin       *plugin.Service
	Git          *git.Service
	Credential   *credential.Service
	Monitor      *monitor.Service
	Expression   *expression.Service
	Integrity    *integrity.Service
	Logging      *logging.Service

	templateManager *template.Manager
	storageManager  *storage.Manager
	formManager     *form.Manager
	renderManager   *render.Manager
	navManager      *nav.Manager
	journalManager  *journal.Manager
	indexManager    *index.Manager
	indexEvents     *index.EventHandler
	dataProvider    *dataprovider.Manager
	wikiManager     *wiki.Manager
	pluginManager     *plugin.Manager
	gitManager        *git.Manager
	credentialManager *credential.Manager
	apiHandler      http.Handler
	emitter         *emitterRelay
	logBroadcaster  *applog.Broadcaster
	deps            Deps
}

func New(d Deps) (*App, error) {
	if d.AppRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			d.AppRoot = cwd
		}
	}
	if d.Logger == nil {
		logger, bc := applog.New(applog.Options{AppRoot: d.AppRoot})
		d.Logger = logger
		d.LogBroadcaster = bc
	}

	sysM := system.NewManager(d.AppRoot, d.Logger)

	cfgM, err := config.NewManager(sysM, d.Logger)
	if err != nil {
		return nil, err
	}

	sfrM := sfr.NewManager(sysM, d.Logger)
	csvM := csv.NewManager(sysM, d.Logger)

	// Template manager — rooted at the active context's templates path
	// (absolute, from config's VFS). On profile/context switch the
	// composition root is rebuilt; that's outside the scope of this story.
	templatesPath, err := cfgM.GetContextTemplatesPath()
	if err != nil {
		return nil, err
	}
	tplM := template.NewManager(sysM, templatesPath, d.Logger)
	// Stamp every saved template with the active profile's author
	// identity when the caller didn't set it. Mirrors how record
	// .meta.json files carry meta.author_name / meta.author_email so
	// PullWithStash can name "who last touched this file" without
	// walking git log.
	tplM.SetAuthorReader(template.AuthorFunc(func() (string, string) {
		cfg, err := cfgM.LoadUserConfig()
		if err != nil || cfg == nil {
			return "", ""
		}
		return cfg.AuthorName, cfg.AuthorEmail
	}))
	tplStorageLocator := func(name string) string {
		if info := cfgM.GetTemplateStorageInfo(name); info != nil {
			return info.Path
		}
		return ""
	}

	// Storage manager — rooted at the active context's storage path.
	storagePath, err := cfgM.GetContextStoragePath()
	if err != nil {
		return nil, err
	}
	stoM := storage.NewManager(sysM, sfrM, tplM, storagePath, d.Logger)

	// Form manager — orchestrates template + storage + config defaults
	// for the Storage workspace's per-form view. configAdapter is a
	// thin shim so config doesn't have to depend on form's types.
	formM := form.NewManager(tplM, stoM, &configAdapter{cfg: cfgM}, d.Logger)

	// Render — two managers, one per transport target. The render
	// pipeline is identical (Handlebars → Markdown → HTML); only the
	// URL strategies differ. Each consumer instantiates its own
	// (image, link) pair so future export targets (Azure DevOps wiki,
	// GitHub wiki, …) just plug in their own strategies without
	// teaching the render module about transports.
	//
	//   slideoutRender — Storage workspace preview slideout + the
	//     Wails Render service. Images come back as `data:` URLs (the
	//     Wails webview blocks file://); formidable:// URLs pass
	//     through, the Vue click interceptor in StorageWorkspace
	//     resolves them via the Nav service.
	//
	//   wikiRender — wiki HTTP server (and dataprovider, which the
	//     wiki consumes). Images come back as `/storage/<tpl>/images/
	//     <name>` so the browser caches them; formidable:// URLs are
	//     rewritten to `/template/<stem>/form/<datafile>` at the
	//     source so links work natively as plain HTML anchors.
	// The slideout's <img src=…> reaches /api/images/<stem>/<file>
	// through Wails' AssetMiddleware (see APIAssetMiddleware) — the
	// markdown stays free of inlined base64 and the same URL works
	// from external HTTP callers when the wiki/api server is on.
	slideoutImageURL := func(templateFilename, name string) string {
		stem := strings.TrimSuffix(templateFilename, ".yaml")
		// PathEscape on the filename segment only — spaces, parens, etc.
		// in the on-disk name would otherwise produce a markdown URL
		// goldmark refuses to parse as an image destination (link
		// destinations may not contain unescaped spaces). The template
		// stem is slug-shaped and safe to pass through verbatim.
		return "/api/images/" + stem + "/" + url.PathEscape(name)
	}
	// Inline-image mode for the generator's "inline" choice — reads
	// the bytes via storage and returns the data URL (which
	// LoadImageFile already produces). Wired only on the slideout
	// manager; the wiki manager keeps url-only output.
	slideoutImageBase64 := func(templateFilename, name string) string {
		dataURL, err := stoM.LoadImageFile(templateFilename, name)
		if err != nil {
			return ""
		}
		return dataURL
	}
	slideoutRender := render.NewManager(tplM, stoM, slideoutImageURL, nil /*linkURL*/, d.Logger)
	slideoutRender.SetImageBase64URL(slideoutImageBase64)

	wikiImageURL := func(templateFilename, name string) string {
		stem := strings.TrimSuffix(templateFilename, ".yaml")
		// See slideoutImageURL above: PathEscape so spaces and other
		// markdown-hostile chars in the filename round-trip through the
		// rendered URL as %-encoded bytes.
		return "/storage/" + stem + "/images/" + url.PathEscape(name)
	}
	wikiLinkURL := func(templateFilename, datafile string) string {
		stem := strings.TrimSuffix(templateFilename, ".yaml")
		return "/template/" + stem + "/form/" + datafile
	}
	wikiRender := render.NewManager(tplM, stoM, wikiImageURL, wikiLinkURL, d.Logger)

	// `renderM` is the slideout-context manager; the Render Wails
	// service binds to it. Most code below references `renderM`.
	renderM := slideoutRender

	i18nM, err := i18n.NewManager(d.Logger)
	if err != nil {
		return nil, fmt.Errorf("init i18n: %w", err)
	}

	emitter := &emitterRelay{}
	jrnM := journal.NewManager(sysM, d.Logger, emitter)

	// Nav manager — owns formidable:// URL resolution. Validates the
	// (template, datafile) pair against the same managers the rest of
	// the app uses, persists the selection to config, and emits a
	// nav:changed event so the frontend's global listener can flip the
	// active workspace.
	navM := nav.NewManager(tplM, stoM, &configWriterAdapter{cfg: cfgM}, emitter, d.Logger)

	// Wire journal as the emitter for system FS mutations and as the
	// configurer that listens to context-folder/backend changes from config.
	sysM.SetJournal(jrnM)
	cfgM.SetJournal(jrnM)

	// Trigger an initial Configure so the journal picks up the freshly
	// loaded config without waiting for the next save.
	if cfg, err := cfgM.LoadUserConfig(); err == nil {
		_ = jrnM.Configure(cfg.ContextFolder, cfg.RemoteBackend)
		// Best-effort baseline seed; harmless if log already exists.
		_ = jrnM.Init()
	}

	// Index — per-profile SQLite cache that backs the future wiki/API.
	// Lives at <AppRoot>/index/<profile-stem>.db. Read-side never
	// touches disk; writes go through the manager hooks below and via
	// RescanAll on startup (catches sync/external edits we missed).
	contextRoot, err := cfgM.GetContextPath()
	if err != nil {
		return nil, fmt.Errorf("init index: resolve context: %w", err)
	}
	profileStem := strings.TrimSuffix(cfgM.CurrentProfileFilename(), filepath.Ext(cfgM.CurrentProfileFilename()))
	if profileStem == "" {
		profileStem = "default"
	}
	indexDBPath := filepath.Join(d.AppRoot, "index", profileStem+".db")
	idxM, err := index.NewManager(indexDBPath)
	if err != nil {
		return nil, fmt.Errorf("init index: open %q: %w", indexDBPath, err)
	}

	// EventHandler bridges template/storage save+delete events into the
	// index. The loader adapter wraps the existing managers with the
	// stat() call the index needs for mtime/size tracking.
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(contextRoot)
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)

	// First-boot reconcile — picks up anything that landed on disk
	// while the app was off (gigot pull, manual edits, etc.). Logged-
	// best-effort: the index is a derived view, app boots regardless.
	if err := ehM.RescanAll(context.Background()); err != nil {
		d.Logger.Warn("index initial RescanAll failed", "err", err)
	}

	// Dataprovider — read-only facade over the index + render. The
	// wiki HTTP server consumes this and gets `wikiRender` so its
	// rendered output already carries `/template/.../form/...` and
	// `/storage/.../images/...` URLs (no post-process regex needed).
	// Vue continues to call the per-module Wails services directly,
	// which use `slideoutRender` (formidable:// + data: URLs).
	dpM := dataprovider.NewManager(idxM, wikiRender, stoM)

	// Expression engine — sandboxed evaluator for sidebar sub-labels
	// (and future field-default / plugin-command callers). Built before
	// the wiki handler so the wiki form list can show expression
	// subtitles using the same engine the in-app sidebar uses.
	expressionM := expression.NewManager(
		expressionTemplateAdapter{tpl: tplM},
		expressionStorageAdapter{sto: stoM},
	)

	// Integrity — analyzes stored forms against the template's current
	// field declarations (Utilities → Cleanup Storage). Phase 1 was
	// analyze-only; phase 2 adds Fix, which mutates meta + data and
	// commits via storage.SaveFormExact so meta mutations (mint UUID,
	// re-stamp timestamps) land on disk without the SaveForm
	// "preserve prev meta" path overriding them.
	integrityM := integrity.NewManager(tplM, stoM)
	integrityM.SetWriter(integrityStorageAdapter{sto: stoM})

	// Wiki — runtime-controllable HTTP server that serves rendered
	// templates+forms from dataprovider and images from storage. The
	// in-app About workspace toggles it on/off via Wiki service. The
	// window-opener hook is installed by main.go after the Wails
	// application exists; until then OpenInternalWiki returns an error.
	wikiM := wiki.NewManager(d.Logger)
	wikiHandler := wiki.NewHandler(dpM, stoM, expressionM)

	// REST API peer surface — `/api/...` routes (collections CRUD-read,
	// design, exports, OpenAPI spec, Swagger UI). Mounted alongside the
	// wiki HTML chrome on the same loopback listener; Go's mux routes
	// `/api/*` to the api handler and everything else to wiki by
	// longest-prefix match.
	// stoM appears twice — once as Storage (LoadForm), once as Writer
	// (SaveForm/DeleteForm). Same instance, narrow per-concern interfaces.
	apiHandler := api.NewHandler(dpM, stoM, stoM, tplM)

	// Monitor module — generic observation surface over Formidable's
	// internal event streams. JournalSource is the only registered
	// source for now; future LogSource / RequestSource plug into the
	// same Manager. Wails service for the in-app Monitoring page,
	// HTTP handler at /api/monitor/* for external consumers.
	monitorM := monitor.NewManager()
	monitorM.Register(monitor.NewJournalSource(jrnM, sysM))
	monitorHandler := monitor.NewHandler(monitorM)

	top := http.NewServeMux()
	// Longest-prefix wins: /api/monitor/ takes precedence over /api/.
	top.Handle("/api/monitor/", monitorHandler)
	top.Handle("/api/", apiHandler)
	top.Handle("/", wikiHandler)
	wikiM.SetHandler(top)
	wikiSvc := wiki.NewService(wikiM,
		func() int {
			cfg, err := cfgM.LoadUserConfig()
			if err != nil || cfg.InternalServerPort == 0 {
				return 8383
			}
			return cfg.InternalServerPort
		},
		openInDefaultBrowser,
		nil, // window opener installed via App.SetWindowOpener
	)

	// Auto-start when the user's config asks for it. Best-effort:
	// failure is logged so the rest of the app still boots.
	if cfg, err := cfgM.LoadUserConfig(); err == nil && cfg.EnableInternalServer {
		if err := wikiSvc.StartServer(); err != nil {
			d.Logger.Warn("wiki: auto-start failed", "err", err)
		}
	}

	// Plugin module — Lua-scripted on-demand commands. Lives at
	// <AppRoot>/plugins/<id>/{plugin.json,main.lua}; per-plugin K/V
	// at <AppRoot>/plugins/.kv/<id>.json. Discovery runs once at
	// boot; the workspace's Refresh button re-scans at runtime.
	pluginsDir := filepath.Join(d.AppRoot, "plugins")
	pluginKV := plugin.NewKV(sysM, filepath.Join(pluginsDir, ".kv"))
	pluginM := plugin.NewManager(plugin.ManagerDeps{
		PluginsDir: pluginsDir,
		Logger:     d.Logger,
		KV:         pluginKV,
		// Editor uses *system.Manager so plugin.json + main.lua get
		// the same atomic+fsync write semantics every other write in
		// the codebase enjoys.
		Editor:     sysM,
		Template:   pluginTemplateAdapter{dp: dpM, tpl: tplM},
		Collection: pluginCollectionAdapter{dp: dpM},
		Form:       pluginFormAdapter{sto: stoM},
		Render:     pluginRenderAdapter{rdr: renderM},
		FS:         plugin.OSFS{},
		Exec:       plugin.OSExec{},
		// HTTPClient is satisfied by a wiki+system adapter — plugins
		// that flag requires_internal_server in their manifest get
		// formidable.api.fetch wired against the running wiki server.
		API:        pluginHTTPAdapter{wiki: wikiM, sys: sysM},
	})
	if err := pluginM.Refresh(); err != nil {
		d.Logger.Warn("plugin: initial refresh failed", "err", err)
	}

	// Collaboration → Git. Stateless read-only manager backed by
	// pure go-git — no system git binary or credential helper
	// required. Network/auth ops arrive in a later pass.
	gitM := git.NewManager()

	// Collaboration → Credentials. Thin wrapper over the OS
	// keychain (zalando/go-keyring). Used by the Clone form's
	// "Save token for sync" opt-in and (later) by sync ops that
	// need to read the stored PAT to talk to the remote.
	credentialM := credential.NewManager()

	d.Logger.Info("formidable starting", "appRoot", d.AppRoot)

	return &App{
		About:           about.NewService(),
		System:          system.NewService(sysM),
		Config:          config.NewService(cfgM),
		Sfr:             sfr.NewService(sfrM),
		Journal:         journal.NewService(jrnM),
		Csv:             csv.NewService(csvM),
		Template:        template.NewService(tplM, tplStorageLocator),
		Storage:         storage.NewService(stoM),
		Form:            form.NewService(formM),
		I18n:            i18n.NewService(i18nM),
		Dialog:          dialog.NewService(),
		Render:          render.NewService(renderM),
		Nav:             nav.NewService(navM),
		Wiki:            wikiSvc,
		Dataprovider:    dataprovider.NewService(dpM),
		Plugin:          plugin.NewService(pluginM),
		Git:             git.NewService(gitM, credentialM, cfgM, jrnM),
		Credential:      credential.NewService(credentialM),
		Monitor:         monitor.NewService(monitorM),
		Expression:      expression.NewService(expressionM),
		Integrity:       integrity.NewService(integrityM),
		Logging:         logging.NewService(logging.NewManager(d.LogBroadcaster, applog.LogPath(applog.Options{AppRoot: d.AppRoot}))),
		templateManager: tplM,
		storageManager:  stoM,
		formManager:     formM,
		renderManager:   renderM,
		navManager:      navM,
		journalManager:  jrnM,
		indexManager:    idxM,
		indexEvents:     ehM,
		dataProvider:    dpM,
		wikiManager:     wikiM,
		pluginManager:   pluginM,
		gitManager:        gitM,
		credentialManager: credentialM,
		apiHandler:        apiHandler,
		emitter:         emitter,
		logBroadcaster:  d.LogBroadcaster,
		deps:            d,
	}, nil
}

// APIHandler returns the api module's http.Handler. main.go feeds this
// into the Wails AssetServer middleware so /api/* requests from the
// in-app webview reach the api handler even when the optional wiki/api
// HTTP server is OFF (the same handler also runs behind the loopback
// HTTP server when the user enables it via the Information workspace).
func (a *App) APIHandler() http.Handler {
	if a == nil {
		return nil
	}
	return a.apiHandler
}

// SetWindowOpener installs the Wails-aware function used by
// Wiki.OpenInternalWiki to spawn an in-app webview window. main.go
// calls this after the Wails application is built (the application
// pointer doesn't exist when New() runs). Pass nil to disable.
func (a *App) SetWindowOpener(fn func(url string) error) {
	if a == nil || a.Wiki == nil {
		return
	}
	wiki.InstallWindowOpener(a.Wiki, fn)
}

// SetEmit installs the transport that journal events (and the log
// broadcaster) flow through. main.go calls this after building the
// Wails application; once installed, every slog record also reaches
// the frontend as a "log:entry" event.
func (a *App) SetEmit(fn EmitFunc) {
	if a == nil || a.emitter == nil {
		return
	}
	a.emitter.set(fn)
	if a.logBroadcaster != nil {
		a.logBroadcaster.SetEmitter(func(e applog.Entry) {
			fn("log:entry", e)
		})
	}
}

func (a *App) Logger() *slog.Logger { return a.deps.Logger }
