// Package app is Formidable2's composition root.
//
// It constructs every domain module with its dependencies, exposes the
// Wails service list to main.go, and (later) registers the loopback
// HTTP routes for the modules that opt into them.
package app

import (
	"log/slog"
	"os"

	applog "github.com/petervdpas/formidable2/internal/log"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

type Deps struct {
	AppRoot string
	Logger  *slog.Logger
}

type App struct {
	System *system.Service

	deps Deps
}

func New(d Deps) *App {
	if d.AppRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			d.AppRoot = cwd
		}
	}
	if d.Logger == nil {
		d.Logger = applog.New(applog.Options{AppRoot: d.AppRoot})
	}

	sysM := system.NewManager(d.AppRoot, d.Logger)

	d.Logger.Info("formidable2 starting", "appRoot", d.AppRoot)

	return &App{
		System: system.NewService(sysM),
		deps:   d,
	}
}

func (a *App) Logger() *slog.Logger { return a.deps.Logger }
