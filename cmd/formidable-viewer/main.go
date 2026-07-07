// Command formidable-viewer is the standalone Formidable Viewer: a small
// Wails app that opens a Formidable offline export (.zip) and renders it in a
// native window, straight from the archive with no unpacking and no running
// Formidable instance.
//
// It ships and installs separately from the main Formidable app. Its only job
// is to serve a bundle's already-rendered pages to a webview, so it carries no
// Vue frontend of its own: the bundle IS the frontend.
//
// A bundle opens two ways: a path passed as the first argument (file
// association / "open with"), or dropping a .zip onto the window.
package main

import (
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/viewer"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const appName = "Formidable Viewer"

//go:embed appicon.png
var appIcon []byte

func windowTitle(srv *viewer.Server) string {
	if b := srv.Current(); b != nil {
		return b.Name() + " - " + appName
	}
	return appName
}

func main() {
	srv := viewer.NewServer()

	// A bundle path arriving as the first argument (file association /
	// "open with") loads immediately so the window opens onto content.
	if len(os.Args) > 1 {
		if b, err := viewer.OpenBundle(os.Args[1]); err == nil {
			srv.SetBundle(b)
		} else {
			log.Printf("%s: cannot open %q: %v", appName, os.Args[1], err)
		}
	}

	app := application.New(application.Options{
		Name:        appName,
		Description: "Offline viewer for Formidable exports",
		Icon:        appIcon,
		Assets:      application.AssetOptions{Handler: srv},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            windowTitle(srv),
		Width:            1100,
		Height:           820,
		MinWidth:         720,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(27, 30, 36),
		EnableFileDrop:   true,
		Linux:            application.LinuxWindow{Icon: appIcon},
		URL:              "/",
	})

	// No native menu bar: it looked out of place next to the main app's
	// chrome. A bundle opens by dropping a .zip anywhere on the window; the
	// first .zip in the drop wins and swaps the current bundle.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		for _, f := range e.Context().DroppedFiles() {
			if !strings.EqualFold(filepath.Ext(f), ".zip") {
				continue
			}
			b, err := viewer.OpenBundle(f)
			if err != nil {
				log.Printf("%s: cannot open %q: %v", appName, f, err)
				return
			}
			if prev := srv.SetBundle(b); prev != nil {
				_ = prev.Close()
			}
			win.SetTitle(windowTitle(srv))
			win.Reload()
			return
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
