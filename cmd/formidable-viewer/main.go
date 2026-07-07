// Command formidable-viewer is the standalone Formidable Viewer: a small
// Wails app that opens a Formidable offline export (.zip) and renders it in a
// native window, straight from the archive with no unpacking and no running
// Formidable instance.
//
// It ships and installs separately from the main Formidable app. Its only job
// is to serve a bundle's already-rendered pages to a webview, so it carries no
// Vue frontend of its own: the bundle IS the frontend.
//
// A bundle path may be passed as the first argument (file association or
// "open with"); otherwise use File > Open Bundle.
package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/petervdpas/formidable2/internal/modules/viewer"
	"github.com/wailsapp/wails/v3/pkg/application"
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
		Linux:            application.LinuxWindow{Icon: appIcon},
		URL:              "/",
	})

	openBundle := func() {
		path, err := app.Dialog.OpenFile().
			SetTitle("Open Formidable export").
			AddFilter("Formidable export (*.zip)", "*.zip").
			PromptForSingleSelection()
		if err != nil || path == "" {
			return
		}
		b, err := viewer.OpenBundle(path)
		if err != nil {
			app.Dialog.Error().
				SetTitle("Could not open bundle").
				SetMessage(err.Error()).
				Show()
			return
		}
		if prev := srv.SetBundle(b); prev != nil {
			_ = prev.Close()
		}
		win.SetTitle(windowTitle(srv))
		win.Reload()
	}

	app.Menu.SetApplicationMenu(buildMenu(openBundle))

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func buildMenu(openBundle func()) *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)

	file := menu.AddSubmenu("File")
	file.Add("Open Bundle...").
		SetAccelerator("CmdOrCtrl+O").
		OnClick(func(*application.Context) { openBundle() })
	file.AddSeparator()
	file.AddRole(application.CloseWindow)
	file.AddRole(application.Quit)

	menu.AddRole(application.EditMenu)

	view := menu.AddSubmenu("View")
	view.AddRole(application.Reload)
	view.AddRole(application.ForceReload)
	view.AddSeparator()
	view.AddRole(application.ResetZoom)
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)

	menu.AddRole(application.WindowMenu)
	return menu
}
