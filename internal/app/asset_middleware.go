package app

import (
	"net/http"
	"strings"
)

// APIAssetMiddleware delegates `/api/*` to the api handler; other paths
// fall through to the embedded Vue dist.
//
// Why: the slideout's <img src="/api/images/<tpl>/<file>"> must resolve
// whether or not the optional internal HTTP server is running. The asset
// server is always up (the webview depends on it), so this gives the same
// route shape a consistent transport.
//
// api may be nil (defensive against composition-root reordering): then the
// middleware is a pure pass-through. The signature matches Wails'
// assetserver.Middleware so it plugs into AssetOptions.Middleware directly.
func APIAssetMiddleware(api http.Handler) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if api != nil && strings.HasPrefix(r.URL.Path, "/api/") {
				api.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
