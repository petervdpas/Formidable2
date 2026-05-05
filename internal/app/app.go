// Package app is Formidable2's composition root.
//
// It constructs every domain module with its dependencies, exposes the
// Wails service list to main.go, and (later) registers the loopback
// HTTP routes for the modules that opt into them.
package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	applog "github.com/petervdpas/formidable2/internal/log"
	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/csv"
	"github.com/petervdpas/formidable2/internal/modules/dialog"
	"github.com/petervdpas/formidable2/internal/modules/form"
	"github.com/petervdpas/formidable2/internal/modules/i18n"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/render"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
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
	AppRoot string
	Logger  *slog.Logger
}

type App struct {
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
	Render   *render.Service

	templateManager *template.Manager
	storageManager  *storage.Manager
	formManager     *form.Manager
	renderManager   *render.Manager
	journalManager  *journal.Manager
	emitter         *emitterRelay
	deps            Deps
}

func New(d Deps) (*App, error) {
	if d.AppRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			d.AppRoot = cwd
		}
	}
	if d.Logger == nil {
		d.Logger = applog.New(applog.Options{AppRoot: d.AppRoot})
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

	// Render manager — Handlebars→Markdown→HTML pipeline shared by the
	// Storage workspace's Render button and the future internal HTTP
	// server. Image URLs resolve to file:// against the on-disk storage
	// path; the HTTP server replaces this strategy at construction.
	renderM := render.NewManager(tplM, stoM, func(templateFilename, name string) string {
		dir := stoM.TemplateImageDir(templateFilename)
		return "file://" + filepath.ToSlash(filepath.Join(dir, name))
	}, d.Logger)

	i18nM, err := i18n.NewManager(d.Logger)
	if err != nil {
		return nil, fmt.Errorf("init i18n: %w", err)
	}

	emitter := &emitterRelay{}
	jrnM := journal.NewManager(sysM, d.Logger, emitter)

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

	d.Logger.Info("formidable2 starting", "appRoot", d.AppRoot)

	return &App{
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
		templateManager: tplM,
		storageManager:  stoM,
		formManager:     formM,
		renderManager:   renderM,
		journalManager:  jrnM,
		emitter:         emitter,
		deps:            d,
	}, nil
}

// SetEmit installs the transport that journal events flow through.
// main.go calls this after building the Wails application.
func (a *App) SetEmit(fn EmitFunc) {
	if a == nil || a.emitter == nil {
		return
	}
	a.emitter.set(fn)
}

func (a *App) Logger() *slog.Logger { return a.deps.Logger }
