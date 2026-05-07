package app

import (
	"net/http"
	"strings"
)

// APIAssetMiddleware returns a Wails AssetServer middleware that
// delegates `/api/*` requests to the supplied api handler and lets
// every other path fall through to the default asset chain (the
// embedded Vue dist).
//
// Why: the slideout's <img src="/api/images/<tpl>/<file>"> needs to be
// fulfilled by the api handler regardless of whether the user has the
// optional internal HTTP server (wiki/api over loopback) running. The
// asset server is always up because the webview itself depends on it,
// so this middleware gives us a consistent transport for the same
// route shape.
//
// `api` is allowed to be nil — defensive against composition-root
// reordering — in which case the middleware is a pure pass-through.
//
// The signature matches Wails' assetserver.Middleware type
// (func(next http.Handler) http.Handler) so it can plug straight into
// application.AssetOptions.Middleware without an adapter.
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
