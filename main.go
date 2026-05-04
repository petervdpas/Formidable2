package main

import (
	"log"
	"os"

	"github.com/petervdpas/formidable2/internal/app"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/ui/handlers"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const appVersion = "0.1.0-dev"

func init() {
	// Register journal events with the Wails binding generator so the
	// frontend gets typed signatures.
	application.RegisterEvent[journal.Entry](journal.EventChanged)
}

func main() {
	cwd, _ := os.Getwd()

	a, err := app.New(app.Deps{AppRoot: cwd})
	if err != nil {
		log.Fatal(err)
	}

	uiHandler := handlers.New(handlers.Deps{
		Template: a.Template,
		Storage:  a.Storage,
		Config:   a.Config,
		AppName:  "Formidable2",
		Version:  appVersion,
	})

	wapp := application.New(application.Options{
		Name:        "Formidable2",
		Description: "Editor for templates and Markdown forms",
		Services:    a.WailsServices(),
		Assets: application.AssetOptions{
			Handler: uiHandler,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Bridge journal events into Wails events.
	a.SetEmit(func(name string, data any) {
		wapp.Event.Emit(name, data)
	})

	wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Formidable2",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	if err = wapp.Run(); err != nil {
		log.Fatal(err)
	}
}
