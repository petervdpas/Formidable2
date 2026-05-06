package main

import (
	"embed"
	"log"
	"os"

	"github.com/petervdpas/formidable2/internal/app"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/nav"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register journal events with the Wails binding generator so the
	// frontend gets typed signatures.
	application.RegisterEvent[journal.Entry](journal.EventChanged)
	application.RegisterEvent[*nav.Target](nav.EventChanged)
}

func main() {
	cwd, _ := os.Getwd()

	a, err := app.New(app.Deps{AppRoot: cwd})
	if err != nil {
		log.Fatal(err)
	}

	wapp := application.New(application.Options{
		Name:        "Formidable",
		Description: "Editor for templates and Markdown forms",
		Services:    a.WailsServices(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	a.SetEmit(func(name string, data any) {
		wapp.Event.Emit(name, data)
	})

	// Install the Wails-aware window opener so wiki.OpenInternalWiki
	// can spawn an in-app webview window pointed at the loopback
	// server. Composition root left this as nil because the Wails
	// application doesn't exist yet at App.New() time.
	a.SetWindowOpener(func(url string) error {
		wapp.Window.NewWithOptions(application.WebviewWindowOptions{
			Title:    "Formidable — Wiki",
			Width:    1024,
			Height:   800,
			MinWidth: 600,
			URL:      url,
		})
		return nil
	})

	winOpts := application.WebviewWindowOptions{
		Title:     "Formidable",
		Width:     1024,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	}

	// Honor persisted window bounds. The 0×0 sentinel means "start in
	// fullscreen". Any positive (w, h) overrides the defaults; Wails
	// clamps to MinWidth/MinHeight so absurdly small values are safe.
	if cfg, err := a.Config.LoadUserConfig(); err == nil && cfg != nil {
		w, h := cfg.WindowBounds.Width, cfg.WindowBounds.Height
		switch {
		case w == 0 && h == 0:
			winOpts.StartState = application.WindowStateFullscreen
			// Leave Width/Height at defaults so exiting fullscreen
			// (Esc / window button) yields a sensible window size.
		case w > 0 && h > 0:
			winOpts.Width = w
			winOpts.Height = h
		}
	}

	wapp.Window.NewWithOptions(winOpts)

	if err = wapp.Run(); err != nil {
		log.Fatal(err)
	}
}
