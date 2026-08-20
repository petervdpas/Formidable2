package connection

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/api/swaggerui"
)

// DocsServer renders an uploaded document with Swagger UI, over loopback HTTP.
//
// The obvious approach, a blob: URL in a sandboxed iframe, renders the overview
// and then hangs the moment an operation is expanded: swagger-ui resolves that
// subtree through its own resolver, which fetches `baseDoc`, and in an opaque
// origin that fetch cannot succeed. A real origin removes the whole class of
// problem rather than working around one symptom of it, and it is how
// Formidable already serves Swagger UI for its own REST surface.
//
// Being cross-origin from the app's webview is a bonus: a document is authored
// elsewhere, and swagger-ui renders its descriptions as markdown, so keeping it
// off the app's origin means a hostile document cannot reach the app.
//
// Scope is deliberately small, mirroring pdf.AssetServer: loopback only, three
// routes, and every filename checked with the same guard the rest of this
// module uses.
type DocsServer struct {
	listener net.Listener
	server   *http.Server
	mgr      *Manager
	log      *slog.Logger

	mu     sync.RWMutex
	closed bool
}

// bindRetryLimit caps how many times a bind is retried when the OS hands back
// a port the caller asked to avoid.
const bindRetryLimit = 8

// NewDocsServer binds a loopback listener and starts serving. excludePorts
// lists ports it must not take, typically the configured internal server port:
// squatting that while the wiki is off would block it starting later.
func NewDocsServer(m *Manager, log *slog.Logger, excludePorts ...int) (*DocsServer, error) {
	if log == nil {
		log = slog.Default()
	}
	if m == nil {
		return nil, errors.New("connection: docs server needs a manager")
	}
	ln, err := bindLoopback(excludePorts)
	if err != nil {
		return nil, fmt.Errorf("connection: docs server bind: %w", err)
	}

	d := &DocsServer{listener: ln, mgr: m, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("/docs/", d.handleDocs)
	mux.HandleFunc("/spec/", d.handleSpec)
	d.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		if err := d.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("connection: docs server stopped", "err", err)
		}
	}()
	log.Info("connection: docs server started", "addr", d.Addr())
	return d, nil
}

// bindLoopback takes a free loopback port, retrying while the OS offers one the
// caller excluded.
func bindLoopback(exclude []int) (net.Listener, error) {
	var held []net.Listener
	defer func() {
		for _, ln := range held {
			_ = ln.Close()
		}
	}()

	for range bindRetryLimit {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		addr, ok := ln.Addr().(*net.TCPAddr)
		if !ok || !excluded(exclude, addr.Port) {
			return ln, nil
		}
		// Hold it open so the next Listen cannot be handed the same port back.
		held = append(held, ln)
	}
	return nil, errors.New("no free port outside the excluded set")
}

func excluded(ports []int, port int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}

// Addr returns the bound host:port, or "" once closed.
func (d *DocsServer) Addr() string {
	if d == nil {
		return ""
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed || d.listener == nil {
		return ""
	}
	return d.listener.Addr().String()
}

// BaseURL returns the server's origin, or "" once closed.
func (d *DocsServer) BaseURL() string {
	addr := d.Addr()
	if addr == "" {
		return ""
	}
	return "http://" + addr
}

// URLFor returns the page that renders specFile, or "" when the server is
// closed or the name is not a plain file inside the specs folder.
func (d *DocsServer) URLFor(specFile string) string {
	base := d.BaseURL()
	if base == "" {
		return ""
	}
	if err := checkSpecFile(specFile); err != nil {
		return ""
	}
	return base + "/docs/?spec=" + url.QueryEscape(specFile)
}

// Close stops the server. Calling it twice is not an error, since a shutdown
// path may run after something else already closed it.
func (d *DocsServer) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if d.server == nil {
		return nil
	}
	return d.server.Close()
}

// handleDocs serves the shell at /docs/ and the vendored assets beneath it.
func (d *DocsServer) handleDocs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/docs/")
	if name == "" || name == "index.html" {
		spec := r.URL.Query().Get("spec")
		if err := checkSpecFile(spec); err != nil {
			http.Error(w, "unknown document", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(shellHTML(spec)))
		return
	}
	data, mime, ok := swaggerui.File(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// handleSpec serves one stored document as JSON. This is the URL the shell
// hands swagger-ui, so it is also what the resolver fetches when an operation
// is expanded.
func (d *DocsServer) handleSpec(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimPrefix(r.URL.Path, "/spec/")
	doc, err := d.mgr.SpecDocument(file)
	if err != nil {
		http.Error(w, "unknown document", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(doc.JSON))
}

// shellHTML is the Swagger UI page. Assets and the document are all
// same-origin, so nothing here reaches the network.
//
// Submit methods are empty on purpose: a call from this page would go out with
// no base URL, no auth and no vault behind it. The Try tab is the supported
// way to call, and it goes through the invoker.
func shellHTML(specFile string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>` + htmlEscape(specFile) + `</title>
<link rel="stylesheet" href="/docs/swagger-ui.css">
<style>body{margin:0;background:#fff;}</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="/docs/swagger-ui-bundle.js"></script>
<script src="/docs/swagger-ui-standalone-preset.js"></script>
<script>
window.addEventListener("DOMContentLoaded", function () {
  window.ui = SwaggerUIBundle({
    url: "/spec/` + jsEscape(specFile) + `",
    dom_id: "#swagger-ui",
    deepLinking: false,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    plugins: [SwaggerUIBundle.plugins.DownloadUrl],
    layout: "BaseLayout",
    supportedSubmitMethods: []
  });
});
</script>
</body>
</html>`
}

// htmlEscape and jsEscape guard the two places specFile is interpolated. The
// filename is already checked to be a plain name, so this is belt and braces
// rather than the only line of defence.
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func jsEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "<", `<`, "\n", "", "\r", "").Replace(s)
}
