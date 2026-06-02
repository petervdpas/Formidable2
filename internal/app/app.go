// Package app is Formidable's composition root. It constructs every
// domain module, exposes the Wails service list to main.go, and
// registers the loopback HTTP routes for modules that opt in.
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
	"github.com/petervdpas/formidable2/internal/modules/auth"
	"github.com/petervdpas/formidable2/internal/modules/codeformatter"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/credential"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/gigot"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/git"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/git/sysgit"
	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/csv"
	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/dialog"
	"github.com/petervdpas/formidable2/internal/modules/expression"
	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/history"
	"github.com/petervdpas/formidable2/internal/modules/i18n"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/integrity"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/logging"
	"github.com/petervdpas/formidable2/internal/modules/manual"
	"github.com/petervdpas/formidable2/internal/modules/monitor"
	"github.com/petervdpas/formidable2/internal/modules/nav"
	"github.com/petervdpas/formidable2/internal/modules/pdf"
	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/query"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
	"github.com/petervdpas/formidable2/internal/modules/updatecheck"
	"github.com/petervdpas/formidable2/internal/modules/wiki"
	"github.com/petervdpas/formidable2/internal/optrack"
	"github.com/petervdpas/formidable2/internal/server/godoc"
	"github.com/petervdpas/formidable2/internal/statengine"
)

// EmitFunc bridges journal events to the host transport. main.go
// installs the Wails-backed implementation via App.SetEmit after the
// Wails app is built.
type EmitFunc func(name string, data any)

// emitterRelay is the journal.EventEmitter held for the manager's
// lifetime; the real transport is installed later because the Wails app
// doesn't exist when journal.NewManager runs.
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
	About         *about.Service
	System        *system.Service
	Config        *config.Service
	Sfr           *sfr.Service
	Journal       *journal.Service
	Csv           *csv.Service
	Template      *template.Service
	Storage       *storage.Service
	Form          *form.Service
	I18n          *i18n.Service
	Dialog        *dialog.Service
	Render        *render.Service
	Nav           *nav.Service
	Wiki          *wiki.Service
	Dataprovider  *dataprovider.Service
	Plugin        *plugin.Service
	Git           *git.Service
	Gigot         *gigot.Service
	OpTrack       *optrack.Service
	Credential    *credential.Service
	Monitor       *monitor.Service
	Stat          *stat.Service
	Query         *query.Service
	Datacore      *datacore.Service
	Expression    *expression.Service
	Formula       *FormulaService
	History       *history.Service
	Integrity     *integrity.Service
	Logging       *logging.Service
	PDF           *pdf.Service
	Manual        *manual.Service
	CodeFormatter *codeformatter.Service
	UpdateCheck   *updatecheck.Service
	Index         *index.Service

	templateManager   *template.Manager
	storageManager    *storage.Manager
	formManager       *form.Manager
	renderManager     *render.Manager
	navManager        *nav.Manager
	journalManager    *journal.Manager
	indexManager      *index.Manager
	indexEvents       *index.EventHandler
	statManager       *stat.Manager
	dataProvider      *dataprovider.Manager
	wikiManager       *wiki.Manager
	pluginManager     *plugin.Manager
	gitManager        *git.Manager
	gigotManager      *gigot.Manager
	credentialManager *credential.Manager
	apiHandler        http.Handler
	emitter           *emitterRelay
	logBroadcaster    *applog.Broadcaster
	deps              Deps
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

	// Rooted at the active context's templates path (absolute, from
	// config's VFS). Profile/context switch rebuilds the composition root.
	templatesPath, err := cfgM.GetContextTemplatesPath()
	if err != nil {
		return nil, err
	}
	tplM := template.NewManager(sysM, templatesPath, d.Logger)
	// Stamp saved templates with the active profile's author when unset,
	// so PullWithStash can name who last touched a file without walking
	// git log (mirrors record .meta.json author_name/author_email).
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

	storagePath, err := cfgM.GetContextStoragePath()
	if err != nil {
		return nil, err
	}
	stoM := storage.NewManager(sysM, sfrM, tplM, storagePath, d.Logger)

	// SaveForm stamps the Updated block (and Created on first save) with
	// the returned identity. Re-read per save so a mid-session profile
	// switch takes effect without restart.
	stoM.SetAuthorProvider(func() (string, string) {
		c, err := cfgM.LoadUserConfig()
		if err != nil || c == nil {
			return "", ""
		}
		return c.AuthorName, c.AuthorEmail
	})

	// Export needs to walk every form for the active template; storage
	// satisfies csv.formsSource via a small adapter. Import uses only csvM.
	csvM.SetForms(&csvFormsAdapter{sto: stoM})
	csvM.SetTemplate(&csvTemplateAdapter{tpl: tplM})

	// configAdapter is a thin shim so config needn't depend on form's types.
	formM := form.NewManager(tplM, stoM, &configAdapter{cfg: cfgM}, d.Logger)

	// One render.Manager per transport target. Pipeline is identical;
	// only the (image, link) URL strategies differ, so a new export
	// target plugs in its own pair without teaching render about
	// transports.
	//
	//   slideoutRender: Storage preview slideout + Wails Render service.
	//     Images as data: URLs (the Wails webview blocks file://);
	//     formidable:// URLs pass through to the Vue click interceptor.
	//   wikiRender: wiki HTTP server + dataprovider. Images as
	//     /storage/<tpl>/images/<name> so the browser caches them;
	//     formidable:// rewritten to /template/<stem>/form/<datafile> at
	//     the source so links work as plain HTML anchors.
	//
	// The slideout's <img src> reaches /api/images/<stem>/<file> via
	// Wails' AssetMiddleware, keeping markdown free of inlined base64 and
	// reusing the same URL for external HTTP callers.
	slideoutImageURL := func(templateFilename, name string) string {
		stem := strings.TrimSuffix(templateFilename, ".yaml")
		// PathEscape the filename segment: spaces/parens in the on-disk
		// name would otherwise yield a destination goldmark refuses to
		// parse as an image. The stem is slug-shaped and safe verbatim.
		return "/api/images/" + stem + "/" + url.PathEscape(name)
	}
	// Generator "inline" mode: data URL via storage (LoadImageFile
	// already produces it). Wired only on slideout; wiki stays url-only.
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
		// PathEscape, see slideoutImageURL above.
		return "/storage/" + stem + "/images/" + url.PathEscape(name)
	}
	wikiLinkURL := func(templateFilename, datafile string) string {
		stem := strings.TrimSuffix(templateFilename, ".yaml")
		return "/template/" + stem + "/form/" + datafile
	}
	wikiRender := render.NewManager(tplM, stoM, wikiImageURL, wikiLinkURL, d.Logger)

	// Third per-target render.Manager. Images as inline base64 because
	// picoloom drives Chrome via page.SetDocumentContent (an about:blank
	// document) and Chrome refuses file:// loads from that origin; Stage 0
	// PoC confirmed base64 works and file:// did not. formidable:// links
	// stay as-is so downstream consumers can still follow them.
	pdfImageDataURL := func(templateFilename, name string) string {
		dataURL, err := stoM.LoadImageFile(templateFilename, name)
		if err != nil {
			return ""
		}
		return dataURL
	}
	pdfRender := render.NewManager(tplM, stoM, pdfImageDataURL, nil /*linkURL*/, d.Logger)
	// {{imageBase64}} shares the function so ImgMode=inline matches
	// {{imageURL}} on the PDF target.
	pdfRender.SetImageBase64URL(pdfImageDataURL)

	// renderM is the slideout-context manager the Render Wails service
	// binds to; most code below references it.
	renderM := slideoutRender

	i18nM, err := i18n.NewManager(d.Logger)
	if err != nil {
		return nil, fmt.Errorf("init i18n: %w", err)
	}

	emitter := &emitterRelay{}
	opsRegistry := optrack.NewRegistry()
	opsRegistry.SetEmitter(emitter)
	jrnM := journal.NewManager(sysM, d.Logger, emitter)

	// History: back/forward stack over formidable:// hrefs. Manager is
	// pure stack data; Controller holds nav replay + emitter + persister;
	// Service exposes only Back/Forward/State to keep SetNavigator/Push/
	// Broadcast off the bound surface.
	bootCfg, _ := cfgM.LoadUserConfig()
	historyM := history.NewManager(bootCfg.History.MaxSize)
	historyM.Restore(bootCfg.History.Stack, bootCfg.History.Index)
	historyCtl := history.NewController(
		historyM,
		nil, // navigator wired below after navM exists
		emitter,
		&historyPersistAdapter{cfg: cfgM},
		d.Logger,
	)
	historySvc := history.NewService(historyCtl)

	// Nav resolves formidable:// URLs: validates the (template, datafile)
	// pair, persists the selection, emits nav:changed for the frontend's
	// workspace switch, and push-hooks history so each link click extends
	// the back/forward stack.
	navM := nav.NewManager(tplM, stoM, &configWriterAdapter{cfg: cfgM}, emitter, historyCtl, d.Logger)
	historyCtl.SetNavigator(&navReplayAdapter{m: navM})

	sysM.SetJournal(jrnM)
	cfgM.SetJournal(jrnM)

	// Initial Configure so the journal picks up the loaded config without
	// waiting for the next save.
	if cfg, err := cfgM.LoadUserConfig(); err == nil {
		_ = jrnM.Configure(cfg.ContextFolder, cfg.RemoteBackend)
		_ = jrnM.Init() // baseline seed, harmless if log exists
	}

	// Index: per-profile SQLite cache at <AppRoot>/index/<stem>.db. Reads
	// never touch disk; writes go through the manager hooks below and via
	// startup RescanAll (catches sync/external edits we missed).
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

	// EventHandler bridges template/storage save+delete into the index;
	// the loader adapter adds the stat() call the index needs for mtime/
	// size tracking.
	loaderAdapter := newIndexLoaderAdapter(tplM, stoM)
	ehM := index.NewEventHandler(idxM, loaderAdapter, loaderAdapter)
	ehM.SetRoot(contextRoot)
	tplM.SetIndexer(ehM)
	stoM.SetIndexer(ehM)
	stoM.SetReader(newIndexFormReader(idxM))

	// Sandboxed evaluator for sidebar sub-labels and formula fields. Built
	// before datacore so the loader can compute formula cells through it, and
	// before the wiki handler so the form list can show subtitles.
	expressionM := expression.NewManager(
		expressionTemplateAdapter{tpl: tplM},
		expressionStorageAdapter{sto: stoM},
	)
	// The index harvest folds each form's formula values into the expression
	// context, so a sidebar expression can read F["formula"] like any field.
	// The same calculator backs the storage disk-read fallback, so a sidebar
	// label still gets its formula when the index isn't the source.
	ehM.SetFormulaEvaluator(formulaHarvester{ev: expressionM})
	stoM.SetFormulaFiller(formulaHarvester{ev: expressionM})
	// Scaling factors travel the same harvest as formulas, exposed under the
	// S["name"] namespace for both the index and the disk-read fallback.
	ehM.SetScaleEvaluator(formulaHarvester{ev: expressionM})
	stoM.SetScaleFiller(formulaHarvester{ev: expressionM})

	// Datacore: read-only perspectives over a tensor built from the
	// template's live forms. Built before stat because stat computes
	// through it. The loader adapter evaluates the template's formula fields
	// per record via expressionM, so formulas are ordinary datacore fields.
	datacoreSvc := datacore.NewServiceWithPlanner(func(tpl string) datacore.Loader {
		return newDatacoreLoaderAdapter(tplM, stoM, expressionM, tpl)
	}, newDatacoreIndexPlanner(idxM))

	// Chart-neutral statistics computed on the datacore tensor. The index's
	// aggregate methods survive only as the parity-test oracle; runtime no
	// longer routes statistics through them.
	statM := stat.NewManager(statengine.New(datacoreSvc, statengine.TemplateColumnNamer{Tpl: tplM}))
	statM.SetSourceOptions(statSourceOptions{tpl: tplM})
	statM.SetColumnResolver(statColumnResolver{tpl: tplM})
	statSvc := stat.NewService(statM, statTemplateSource{tpl: tplM})

	// Query: read-only SELECT (FDRM) over an in-memory matrix. Reads forms
	// directly, not the index, so any field is queryable and table rows
	// stay row-aligned.
	queryM := query.NewManager(newQueryLoaderAdapter(tplM, stoM))
	querySvc := query.NewService(queryM)

	// EnabledTemplates self-healing: a deleted template must drop from the
	// profile's list so downstream pickers (storage, wiki, api) reflect
	// reality. Reconcile is a no-op on an empty list ("all enabled"
	// default), so opted-out users pay no I/O.
	cfgM.SetTemplateLister(tplM)
	// First-run seed: a never-configured (or legacy key-absent) profile
	// scopes to every template; an explicitly-emptied profile ([]) stays
	// "none".
	if err := cfgM.SeedEnabledTemplatesIfUnset(); err != nil {
		d.Logger.Warn("seed enabled templates", "err", err)
	}
	tplM.AddObserver(template.ObserverFunc(func(_ string) error {
		_, err := cfgM.ReconcileEnabledTemplates()
		return err
	}))
	// Auto-enable freshly-created templates so Create-from-Editor shows the
	// new template without a Settings toggle. No-op when curation is off.
	tplM.AddCreationObserver(template.CreationObserverFunc(func(filename string) error {
		return cfgM.AutoEnableNewTemplate(filename)
	}))

	// First-boot reconcile picks up disk changes from while the app was
	// off (gigot pull, manual edits). Best-effort: the index is derived,
	// the app boots regardless.
	if err := ehM.RescanAll(context.Background()); err != nil {
		d.Logger.Warn("index initial RescanAll failed", "err", err)
	}

	// Read-only facade over index + render. Gets wikiRender so the wiki
	// server's output already carries /template/.../form/... and
	// /storage/.../images/... URLs (no post-process regex). Vue calls the
	// per-module Wails services directly, which use slideoutRender.
	dpM := dataprovider.NewManager(idxM, wikiRender, stoM)

	// Integrity analyzes stored forms against the template's current field
	// declarations. Fix commits via storage.SaveFormExact so meta mutations
	// (mint UUID, re-stamp timestamps) land without the SaveForm "preserve
	// prev meta" path overriding them.
	integrityM := integrity.NewManager(tplM, stoM)
	integrityM.SetWriter(integrityStorageAdapter{sto: stoM})

	// Runtime-toggled HTTP server for rendered templates+forms. The
	// window-opener hook is installed by main.go after the Wails app
	// exists; until then OpenInternalWiki returns an error.
	wikiM := wiki.NewManager(d.Logger)
	wikiHandler := wiki.NewHandler(dpM, stoM, expressionM)
	// Hide Settings-disabled templates from the list and detail pages (404).
	wikiHandler.SetEnabledFilter(cfgM)
	// Per-template facet definitions drive the index pills and template
	// filter strip.
	wikiHandler.SetTemplates(tplM)

	// stoM appears twice: Storage (LoadForm) and Writer (SaveForm/
	// DeleteForm). Same instance, narrow per-concern interfaces.
	apiHandlerBare := api.NewHandler(dpM, stoM, stoM, tplM, statSvc, querySvc)

	// Desktop-mode auth covering the two transports the api rides on:
	//
	//   apiHandlerNetwork: full chain (LoopbackOnly + RequireOrigin +
	//     ResolveIdentity). Mounted on the wiki mux the optional loopback
	//     server binds, where real TCP clients and browser tabs reach.
	//   apiHandlerInProcess: ResolveIdentity only. Served via Wails'
	//     AssetMiddleware to in-webview asset loads, which are
	//     process-local (empty RemoteAddr, no cross-origin browser), so the
	//     network defenses don't apply and would only false-positive here.
	//
	// Both run ResolveIdentity so SaveForm's audit stamping is ctx-scoped
	// on every transport.
	desktopResolver := auth.NewDesktopResolver(func() (string, string, string) {
		c, err := cfgM.LoadUserConfig()
		if err != nil || c == nil {
			return "", "", ""
		}
		return c.ProfileName, c.AuthorName, c.AuthorEmail
	})
	apiOriginAllowlist := buildAPIOriginAllowlist(cfgM)
	apiHandlerInProcess := auth.ResolveIdentity(desktopResolver)(apiHandlerBare)
	apiHandlerNetwork := auth.LoopbackOnly(
		auth.RequireOrigin(apiOriginAllowlist)(apiHandlerInProcess),
	)

	// Observation surface over internal event streams. JournalSource is the
	// only source for now; future sources plug into the same Manager.
	monitorM := monitor.NewManager()
	monitorM.Register(monitor.NewJournalSource(jrnM, sysM))
	monitorHandler := monitor.NewHandler(monitorM)

	top := http.NewServeMux()
	// Longest-prefix wins: /api/monitor/ takes precedence over /api/.
	top.Handle("/api/monitor/", monitorHandler)
	top.Handle("/api/", apiHandlerNetwork)
	top.Handle("/godoc/", godoc.Handler())
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

	// Auto-start when config asks. Best-effort: failure is logged so the
	// app still boots.
	if cfg, err := cfgM.LoadUserConfig(); err == nil && cfg.EnableInternalServer {
		if err := wikiSvc.StartServer(); err != nil {
			d.Logger.Warn("wiki: auto-start failed", "err", err)
		}
	}

	// Opt-in release probe. Reads update_check live on every Refresh, so
	// the toggle governs the feature without restart and a disabled probe
	// never touches the network. The startup probe is fired once by the
	// frontend via CheckNow; here we only construct the manager.
	updateCheckM := updatecheck.NewManager(about.Version, func() bool {
		cfg, err := cfgM.LoadUserConfig()
		return err == nil && cfg.UpdateCheck
	})

	// Lua-scripted on-demand commands at <AppRoot>/plugins/<id>/, per-plugin
	// K/V at <AppRoot>/plugins/.kv/<id>.json. Boot-time discovery;
	// Refresh re-scans at runtime.
	pluginsDir := filepath.Join(d.AppRoot, "plugins")
	pluginKV := plugin.NewKV(sysM, filepath.Join(pluginsDir, ".kv"))
	pluginM := plugin.NewManager(plugin.ManagerDeps{
		PluginsDir: pluginsDir,
		Logger:     d.Logger,
		KV:         pluginKV,
		// *system.Manager so plugin.json + main.lua get the same
		// atomic+fsync write semantics as every other write.
		Editor:     sysM,
		Template:   pluginTemplateAdapter{dp: dpM, tpl: tplM},
		Collection: pluginCollectionAdapter{dp: dpM},
		Form:       pluginFormAdapter{sto: stoM},
		Render:     pluginRenderAdapter{rdr: renderM},
		FM:         pluginFMAdapter{},
		FS:         plugin.OSFS{},
		Storage:    pluginStorageAdapter{sto: stoM},
		// Bridge formidable.run.* to Wails events that progressbar/
		// statusmessage widgets subscribe to. The emitter is late-bound
		// (main.go calls SetEmit after Wails comes up), so startup-time
		// calls no-op rather than panic.
		RunBarOut: func(evt plugin.RunBarEvent) {
			emitter.Emit("plugin:run:bar", evt)
		},
		RunStatOut: func(evt plugin.RunStatusEvent) {
			emitter.Emit("plugin:run:status", evt)
		},
		RunChartOut: func(evt plugin.RunChartEvent) {
			emitter.Emit("plugin:run:chart", evt)
		},
		RunOptionsOut: func(evt plugin.RunOptionsEvent) {
			emitter.Emit("plugin:run:options", evt)
		},
		Exec: plugin.OSExec{},
		// wiki+system adapter: plugins flagging requires_internal_server get
		// formidable.api.fetch against the running wiki server.
		API: pluginHTTPAdapter{wiki: wikiM, sys: sysM},
		// One adapter for both formidable.stats.* and formidable.facets.*,
		// reading index-backed aggregates.
		Stats:  pluginStatsAdapter{st: statM},
		Facets: pluginStatsAdapter{st: statM},
		// formidable.statistical(tpl, name): named statistical object into a
		// rank-N grid for plugins.
		StatObject: pluginStatObjectAdapter{svc: statSvc},
		Locale:     pluginLocaleAdapter{cfg: cfgM},
	})
	// Materialize the embedded plugin library to <pluginsDir>. Ships the
	// seed inside the binary so every distribution gets the same starter
	// set without per-distro file-copy plumbing. A missing seed isn't fatal.
	if err := plugin.ScaffoldPlugins(sysM, pluginsDir, d.Logger); err != nil {
		d.Logger.Warn("plugin: scaffold failed; library may be incomplete", "err", err)
	}
	if err := pluginM.Refresh(); err != nil {
		d.Logger.Warn("plugin: initial refresh failed", "err", err)
	}

	// Stateless read-only git manager backed by pure go-git, no system
	// binary or credential helper required.
	gitM := git.NewManager().WithLogger(d.Logger)
	sysgitR := sysgit.NewRunner(d.Logger)

	// OS-keychain wrapper for the Clone form's "Save token for sync"
	// opt-in and sync ops that read the stored PAT.
	credentialM := credential.NewManager()

	// JSON-over-HTTP sync to a GiGot server. Writes go through sysM
	// (atomic). The subscription bearer is resolved per-call from the
	// keychain at the Service layer, keeping the Manager transport-neutral.
	gigotM := gigot.NewManager(sysM)

	// PDF export: probes system + managed-cache Chrome on demand, persists
	// activation to <AppRoot>/config/.pdf-state.json via sysM, renders via
	// pdfRender + picoloom. Formidable does not bundle or download Chrome.
	// See design/pdf-export.md.
	pdfM := pdf.NewManager(d.Logger, sysM, pdfRender, stoM, tplM, nil /*real picoloom factory*/)
	if err := pdfM.Restore(); err != nil {
		d.Logger.Warn("pdf: state restore failed", "err", err)
	}
	// Loopback listener serving central-library cover logos during render.
	// Required on Windows: Chrome under a file:// document can't load
	// <img src="C:/..."> verbatim, and picoloom's rewriter only converts
	// paths under SourceDir, so <AppRoot>/pdf/covers/images/ needs a real
	// URL. Non-fatal; render falls back to absolute paths. Exclude the
	// internal-server port so the asset server can't squat on it and block
	// a later wiki Start.
	pdfImagesDir := filepath.Join(d.AppRoot, "pdf", "covers", "images")
	wikiPort := bootCfg.InternalServerPort
	if wikiPort <= 0 {
		wikiPort = defaultInternalServerPort
	}
	if as, err := pdf.NewAssetServer(pdfImagesDir, d.Logger, wikiPort); err != nil {
		d.Logger.Warn("pdf: asset server unavailable; logo URLs will fall back to absolute paths", "err", err)
	} else {
		pdfM.SetAssetServer(as)
	}

	d.Logger.Info("formidable starting", "appRoot", d.AppRoot)

	return &App{
		About:             about.NewService(openInDefaultBrowser),
		System:            system.NewService(sysM),
		Config:            config.NewService(cfgM),
		Sfr:               sfr.NewService(sfrM),
		Journal:           journal.NewService(jrnM),
		Csv:               csv.NewService(csvM),
		Template:          template.NewService(tplM, tplStorageLocator),
		Storage:           storage.NewService(stoM, emitter),
		Form:              form.NewService(formM),
		I18n:              i18n.NewService(i18nM),
		Dialog:            dialog.NewService(),
		Render:            render.NewService(renderM),
		Nav:               nav.NewService(navM),
		Wiki:              wikiSvc,
		Dataprovider:      dataprovider.NewService(dpM),
		Plugin:            plugin.NewService(pluginM),
		Git:               newGitService(gitM, credentialM, cfgM, jrnM, sysgitR, emitter, opsRegistry),
		Gigot:             newGigotService(gigotM, credentialM, cfgM, jrnM, emitter, opsRegistry),
		OpTrack:           optrack.NewService(opsRegistry),
		Credential:        credential.NewService(credentialM),
		Monitor:           monitor.NewService(monitorM),
		Stat:              statSvc,
		Query:             querySvc,
		Datacore:          datacoreSvc,
		Expression:        expression.NewService(expressionM),
		Formula:           NewFormulaService(tplM, stoM, expressionM),
		History:           historySvc,
		Integrity:         integrity.NewService(integrityM, emitter),
		Logging:           logging.NewService(logging.NewManager(d.LogBroadcaster, applog.LogPath(applog.Options{AppRoot: d.AppRoot}), d.Logger)),
		PDF:               newPDFService(pdfM, opsRegistry),
		Manual:            manual.NewService(),
		CodeFormatter:     codeformatter.NewService(codeformatter.NewManager(pdf.Schemas())),
		UpdateCheck:       updatecheck.NewService(updateCheckM, openInDefaultBrowser),
		Index:             newIndexService(ehM, opsRegistry),
		templateManager:   tplM,
		storageManager:    stoM,
		formManager:       formM,
		renderManager:     renderM,
		navManager:        navM,
		journalManager:    jrnM,
		indexManager:      idxM,
		indexEvents:       ehM,
		statManager:       statM,
		dataProvider:      dpM,
		wikiManager:       wikiM,
		pluginManager:     pluginM,
		gitManager:        gitM,
		gigotManager:      gigotM,
		credentialManager: credentialM,
		apiHandler:        apiHandlerInProcess,
		emitter:           emitter,
		logBroadcaster:    d.LogBroadcaster,
		deps:              d,
	}, nil
}

// newGitService composes git.NewService and git.AttachSysgit so the App
// wiring stays one map literal. AttachSysgit is package-level (not a
// method) to keep interface-typed params off the Wails-bound surface.
func newGitService(m *git.Manager, creds git.CredentialReader, cfg *config.Manager, jrnl journal.Journal, sys git.Sysgit, em *emitterRelay, ops *optrack.Registry) *git.Service {
	svc := git.NewService(m, creds, cfg, jrnl)
	git.AttachSysgit(svc, cfg, sys)
	git.AttachEmitter(svc, em)
	git.AttachOps(svc, ops)
	git.AttachRoot(svc, cfg)
	return svc
}

// newPDFService composes pdf.NewService and pdf.AttachOps so a PDF export is
// tracked and guarded against a concurrent second run.
func newPDFService(m *pdf.Manager, ops *optrack.Registry) *pdf.Service {
	svc := pdf.NewService(m)
	pdf.AttachOps(svc, ops)
	return svc
}

// newIndexService composes index.NewService and index.AttachOps so a reindex is
// tracked and guarded per template.
func newIndexService(h *index.EventHandler, ops *optrack.Registry) *index.Service {
	svc := index.NewService(h)
	index.AttachOps(svc, ops)
	return svc
}

// gigotContextResolver feeds gigot its working folder resolved through the one
// shared resolver every backend uses: config.GetRemoteRootPath (ResolvePath
// against AppRoot, selecting gigot_root for the gigot backend). config.Manager's
// raw ContextFolder() would resolve against the process working directory, so a
// relative root would diff/ledger/push a different folder than where records
// live. Routing through GetRemoteRootPath keeps none/git/gigot identical.
type gigotContextResolver struct{ cfg *config.Manager }

func (r gigotContextResolver) GigotBaseURL() string  { return r.cfg.GigotBaseURL() }
func (r gigotContextResolver) GigotRepoName() string { return r.cfg.GigotRepoName() }
func (r gigotContextResolver) AuthorName() string    { return r.cfg.AuthorName() }
func (r gigotContextResolver) AuthorEmail() string   { return r.cfg.AuthorEmail() }
func (r gigotContextResolver) ContextFolder() string {
	abs, err := r.cfg.GetRemoteRootPath()
	if err != nil {
		return ""
	}
	return abs
}

// newGigotService composes gigot.NewService and gigot.AttachProgress so
// the App wiring stays one map literal. The emitterRelay is late-bound
// via App.SetEmit, so progress events fired before the Wails app is built
// no-op instead of panicking.
func newGigotService(m *gigot.Manager, creds gigot.CredentialReader, cfg *config.Manager, jrnl journal.Journal, em *emitterRelay, ops *optrack.Registry) *gigot.Service {
	svc := gigot.NewService(m, creds, cfg, gigotContextResolver{cfg: cfg}, jrnl)
	gigot.AttachProgress(svc, em.Emit)
	gigot.AttachEmitter(svc, em)
	gigot.AttachOps(svc, ops)
	return svc
}

// APIHandler returns the in-process api handler. main.go feeds it into
// the Wails AssetServer middleware so in-webview /api/* requests reach the
// handler even when the optional HTTP server is OFF.
//
// In-process variant: identity stamping is wired but the network-only
// defenses (LoopbackOnly, RequireOrigin) are NOT, because asset requests
// originate inside the webview itself (empty RemoteAddr, no cross-origin
// tab) where those guards would only false-positive. The loopback server
// uses the fully-wrapped variant.
func (a *App) APIHandler() http.Handler {
	if a == nil {
		return nil
	}
	return a.apiHandler
}

// SetWindowOpener installs the Wails-aware function Wiki.OpenInternalWiki
// uses to spawn an in-app webview window. main.go calls this after the
// Wails app is built (the pointer doesn't exist when New runs). nil
// disables.
func (a *App) SetWindowOpener(fn func(url string) error) {
	if a == nil || a.Wiki == nil {
		return
	}
	wiki.InstallWindowOpener(a.Wiki, fn)
}

// SetEmit installs the transport journal events and the log broadcaster
// flow through. main.go calls this after building the Wails app; once
// installed, every slog record also reaches the frontend as a "log:entry"
// event.
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
