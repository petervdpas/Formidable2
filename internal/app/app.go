// Package app is Formidable2's composition root.
//
// It constructs every domain module with its dependencies, exposes the
// Wails service list to main.go, and (later) registers the loopback
// HTTP routes for the modules that opt into them.
package app

import (
	"log/slog"
	"os"

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
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if d.AppRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			d.AppRoot = cwd
		}
	}

	sysM := system.NewManager(d.AppRoot, d.Logger)

	return &App{
		System: system.NewService(sysM),
		deps:   d,
	}
}

func (a *App) Logger() *slog.Logger { return a.deps.Logger }
