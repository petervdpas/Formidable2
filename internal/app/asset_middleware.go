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
				api.ServeHTTP(w, unconditional(r))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// unconditional strips the revalidation headers from a webview request. Wails'
// Windows bridge rewrites a 304 to a 500 (a 304 hangs WebView2), so a handler
// behind this middleware must always answer with the full body. Off the
// webview (the wiki/api HTTP server) revalidation stays intact.
func unconditional(r *http.Request) *http.Request {
	if r.Header.Get("If-None-Match") == "" && r.Header.Get("If-Modified-Since") == "" {
		return r
	}
	c := r.Clone(r.Context())
	c.Header.Del("If-None-Match")
	c.Header.Del("If-Modified-Since")
	return c
}
