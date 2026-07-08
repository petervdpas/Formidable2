// Command formidable-viewer is the standalone Formidable Viewer: a small Wails
// app that opens a Formidable offline export (.zip) and renders it in a native
// window, straight from the archive with no unpacking and no running Formidable
// instance.
//
// It ships and installs separately from the main Formidable app. A small Vue
// shell provides the home screen and settings; the open bundle is served under
// /bundle/ and shown in an iframe. A bundle opens by dropping a .zip onto the
// window, from the shell's Open button, or as the first argument.
package main

import (
	_ "embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	viewerui "github.com/petervdpas/formidable2/frontend-viewer"
	"github.com/petervdpas/formidable2/internal/modules/system"
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

// promptUnlock pokes the Vue shell to open its password prompt for an encrypted
// pack dropped natively, seeding it with the path and cleartext manifest so the
// prompt can name the pack. The shell retries the open with the password.
func promptUnlock(win *application.WebviewWindow, res viewer.OpenResult) {
	payload, err := json.Marshal(map[string]any{
		"path":        res.Path,
		"name":        res.Info.Name,
		"title":       res.Info.Title,
		"description": res.Info.Description,
		"wrong":       res.WrongPassword,
	})
	if err != nil {
		return
	}
	win.ExecJS("window.__viewerUnlock && window.__viewerUnlock(" + string(payload) + ")")
}

func main() {
	spaFS, err := fs.Sub(viewerui.Assets, "dist")
	if err != nil {
		log.Fatal(err)
	}

	bundleServer := viewer.NewServer()        // serves the open bundle (or landing)
	lan := viewer.NewHTTPServer(bundleServer) // optional LAN server over the same handler

	// Always-on loopback server the shell's iframe loads the bundle from. A real
	// http:// origin renders inside the sub-frame, which the app's custom URI
	// scheme does not on WebKitGTK.
	frame := viewer.NewHTTPServer(bundleServer)
	if err := frame.StartOn("127.0.0.1", 0); err != nil {
		log.Printf("%s: frame server: %v", appName, err)
	}

	cfgPath, err := viewer.ConfigPath()
	if err != nil {
		log.Fatal(err)
	}
	// Atomic config writes via the shared system writer (quiet logger).
	sysM := system.NewManager(filepath.Dir(cfgPath), slog.New(slog.NewTextHandler(io.Discard, nil)))
	store := viewer.NewConfigStore(cfgPath, sysM.SaveBytes)

	svc := viewer.NewService(store, bundleServer, lan)
	svc.SetFrameServer(frame)

	// Initial bundle from argv (file association / "open with"). Routed through
	// the UI so an encrypted pack prompts for its password on mount.
	if len(os.Args) > 1 {
		svc.SetPendingOpen(os.Args[1])
	}

	// Asset handler: the Vue shell at /, the open bundle under /bundle/.
	mux := http.NewServeMux()
	mux.Handle("/bundle/", http.StripPrefix("/bundle", bundleServer))
	mux.Handle("/", application.AssetFileServerFS(spaFS))

	cfg := store.Load()

	app := application.New(application.Options{
		Name:        appName,
		Description: "Offline viewer for Formidable exports",
		Icon:        appIcon,
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: mux},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	winOpts := application.WebviewWindowOptions{
		Title:            windowTitle(bundleServer),
		Width:            1100,
		Height:           820,
		MinWidth:         720,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(27, 30, 36),
		EnableFileDrop:   true,
		Linux:            application.LinuxWindow{Icon: appIcon},
		URL:              "/",
	}
	if cfg.RememberSize && cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
		winOpts.Width = cfg.WindowWidth
		winOpts.Height = cfg.WindowHeight
	}
	win := app.Window.NewWithOptions(winOpts)

	if cfg.DefaultZoom > 0 {
		win.SetZoom(cfg.DefaultZoom)
	}

	// Inject the Wails-side behavior the service needs.
	svc.SetOpenFunc(func() (string, error) {
		return app.Dialog.OpenFile().
			SetTitle("Open Formidable bundle").
			AddFilter("Formidable bundle (*.bundle, *.zip)", "*.bundle;*.zip").
			PromptForSingleSelection()
	})
	svc.SetSwapHook(func() {
		win.SetTitle(windowTitle(bundleServer))
		// Poke the Vue shell to re-read the current bundle and switch its view.
		// A direct call is reliable where the custom event was not, and avoids a
		// full webview reload (which lands on a raw 301 page).
		win.ExecJS("window.__viewerRefresh && window.__viewerRefresh()")
	})

	// Native drop: load the first .bundle/.zip through the service so recents
	// record and the swap hook fires (the shell reacts to the event). An
	// encrypted pack that needs a password pokes the shell to open the unlock
	// prompt instead of loading.
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		for _, f := range e.Context().DroppedFiles() {
			ext := strings.ToLower(filepath.Ext(f))
			if ext == ".bundle" || ext == ".zip" {
				res, err := svc.OpenPath(f, "")
				if err != nil {
					log.Printf("%s: cannot open %q: %v", appName, f, err)
					return
				}
				if res.NeedsPassword || res.WrongPassword {
					promptUnlock(win, res)
				}
				// A successful open fires the swap hook, which pokes the Vue
				// shell to switch its view. No reload needed.
				return
			}
		}
	})

	// Persist window size on close when enabled.
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		c := store.Load()
		if !c.RememberSize {
			return
		}
		w, h := win.Size()
		if w > 0 && h > 0 {
			c.WindowWidth, c.WindowHeight = w, h
			_ = store.Save(c)
		}
	})

	// Reflect persisted config (starts the LAN server if enabled).
	if err := svc.Apply(); err != nil {
		log.Printf("%s: apply config: %v", appName, err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
