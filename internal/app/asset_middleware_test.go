package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// markerHandler tags responses so the test can assert which underlying
// handler answered the request.
func markerHandler(tag string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Handled-By", tag)
		_, _ = io.WriteString(w, tag)
	})
}

// ─────────────────────────────────────────────────────────────────────
// APIAssetMiddleware
//
// The Wails AssetServer hosts the Vue dist by default. The slideout's
// <img src="/api/images/…"> needs to reach the api handler even when
// the optional wiki/api HTTP server is OFF, so we install a middleware
// that intercepts /api/* requests and delegates them to the api
// handler. Everything else falls through to the default asset chain.
// ─────────────────────────────────────────────────────────────────────

func TestAPIAssetMiddleware_RoutesAPIPathToAPIHandler(t *testing.T) {
	api := markerHandler("api")
	fallback := markerHandler("assets")

	wrapped := APIAssetMiddleware(api)(fallback)

	cases := []string{
		"/api/images/recepten/cake.png",
		"/api/collections",
		"/api/openapi.json",
		"/api/docs/",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Handled-By"); got != "api" {
			t.Errorf("path %q routed to %q, want %q", path, got, "api")
		}
	}
}

func TestAPIAssetMiddleware_FallsThroughForAssetPaths(t *testing.T) {
	api := markerHandler("api")
	fallback := markerHandler("assets")

	wrapped := APIAssetMiddleware(api)(fallback)

	cases := []string{
		"/",
		"/index.html",
		"/assets/index-DnPMLxm3.css",
		"/api", // no trailing slash, not a prefix match
		"/apiX/foo",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Handled-By"); got != "assets" {
			t.Errorf("path %q routed to %q, want %q (body=%s)",
				path, got, "assets", strings.TrimSpace(rec.Body.String()))
		}
	}
}

func TestAPIAssetMiddleware_NilAPIIsNoOp(t *testing.T) {
	// Defensive: if the api handler is somehow nil, the middleware
	// shouldn't panic - it should fall through to the default chain
	// for every request, including /api/*.
	fallback := markerHandler("assets")

	wrapped := APIAssetMiddleware(nil)(fallback)

	req := httptest.NewRequest(http.MethodGet, "/api/images/foo/bar.png", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Handled-By"); got != "assets" {
		t.Errorf("nil api: path routed to %q, want fallback", got)
	}
}

func TestAPIAssetMiddleware_PreservesMethodAndQuery(t *testing.T) {
	// The api handler does method + query parsing. Verify the wrapped
	// handler doesn't lose either across the delegation.
	var capturedMethod, capturedQuery string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	})
	fallback := markerHandler("assets")

	wrapped := APIAssetMiddleware(api)(fallback)

	req := httptest.NewRequest(http.MethodHead, "/api/images/x/y.png?format=url", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if capturedMethod != http.MethodHead {
		t.Errorf("method = %q, want HEAD", capturedMethod)
	}
	if capturedQuery != "format=url" {
		t.Errorf("query = %q, want format=url", capturedQuery)
	}
}
