package main

import (
	"embed"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/petervdpas/formidable2/internal/app"
	applog "github.com/petervdpas/formidable2/internal/log"
	"github.com/petervdpas/formidable2/internal/modules/collaboration/gigot"
	"github.com/petervdpas/formidable2/internal/modules/journal"
	"github.com/petervdpas/formidable2/internal/modules/nav"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register journal events with the Wails binding generator so the
	// frontend gets typed signatures.
	application.RegisterEvent[journal.Entry](journal.EventChanged)
	application.RegisterEvent[*nav.Target](nav.EventChanged)
	application.RegisterEvent[applog.Entry]("log:entry")
	application.RegisterEvent[gigot.SyncProgress](gigot.EventSyncProgress)
}

func main() {
	cwd, _ := os.Getwd()

	a, err := app.New(app.Deps{AppRoot: cwd})
	if err != nil {
		log.Fatal(err)
	}

	wapp := application.New(application.Options{
		Name:        "Formidable",
		Description: "A System for Templates and Markdown Forms",
		Services:    a.WailsServices(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// Route /api/* requests from the in-app webview to the api
			// handler so the slideout's <img src="/api/images/…"> works
			// regardless of whether the optional wiki/api HTTP server
			// is running. Everything else falls through to the embedded
			// Vue dist.
			Middleware: application.Middleware(app.APIAssetMiddleware(a.APIHandler())),
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
			Title:    "Formidable Wiki",
			Width:    1024,
			Height:   800,
			MinWidth: 600,
			URL:      url,
		})
		return nil
	})

	// Splash window: pure-HTML page in frontend/public. Loads
	// immediately, masks the SPA boot. Identity (name/version/tagline/
	// author) is passed via URL query params so the page itself stays
	// dependency-free (no Wails runtime, no Vue).
	info := a.About.GetInfo()
	splashQ := url.Values{}
	splashQ.Set("n", info.Name)
	splashQ.Set("v", info.Version)
	splashQ.Set("t", info.Tagline)
	splashQ.Set("a", info.Author)
	splash := wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            info.Name,
		Width:            580,
		Height:           320,
		Frameless:        true,
		AlwaysOnTop:      true,
		DisableResize:    true,
		BackgroundColour: application.NewRGB(26, 18, 48),
		URL:              "/splash.html?" + splashQ.Encode(),
	})

	winOpts := application.WebviewWindowOptions{
		Title:     info.Name + " " + info.Version,
		Width:     1024,
		Height:    800,
		MinWidth:  720,
		MinHeight: 600,
		Hidden:    true, // shown after splash dismissal
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

	mainWin := wapp.Window.NewWithOptions(winOpts)

	// Unsaved-changes guard. RegisterHook runs synchronously BEFORE the
	// window's built-in destroy listener (and before the aux-window
	// closer below), so cancelling here keeps the window open. The SPA
	// mirrors the active form's dirty state via system.SetUnsavedChanges;
	// when set, an OS-driven close (X button, Cmd+Q) is vetoed and we
	// ask the frontend to show its Save / Discard / Cancel dialog. The
	// frontend calls system.ConfirmClose() once the user chooses to
	// leave, which flips AllowClose and re-triggers the close.
	mainWin.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if system.AllowClose() || !system.UnsavedChanges() {
			return
		}
		e.Cancel()
		wapp.Event.Emit("app:close-requested", nil)
	})

	// Closing the main window must take auxiliary windows (wiki,
	// swagger, future popouts) with it. Otherwise they're orphaned
	// after the app's primary UI is gone. Iterate every Wails-known
	// window and close anything that isn't main.
	mainWin.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		for _, win := range wapp.Window.GetAll() {
			if win == nil || win.ID() == mainWin.ID() {
				continue
			}
			win.Close()
		}
	})

	// Dismiss the splash on whichever happens first: the SPA emits
	// `spa:ready` after Vue mounts, or a 10-second fallback fires in
	// case the SPA never reaches that point. sync.Once guards against
	// the swap running twice.
	var swapOnce sync.Once
	dismissSplash := func(reason string) {
		swapOnce.Do(func() {
			if splash != nil {
				splash.Close()
			}
			mainWin.Show()
			// Tell the SPA the main window is now visible. Anything that
			// must surface to the user at startup (e.g. the update-check
			// toast) waits for this so it isn't painted behind the splash.
			wapp.Event.Emit("main:shown", nil)
			a.Logger().Info("splash dismissed", "reason", reason)
		})
	}
	wapp.Event.On("spa:ready", func(_ *application.CustomEvent) {
		dismissSplash("spa:ready")
	})
	go func() {
		time.Sleep(10 * time.Second)
		dismissSplash("timeout")
	}()

	if err = wapp.Run(); err != nil {
		log.Fatal(err)
	}
}
