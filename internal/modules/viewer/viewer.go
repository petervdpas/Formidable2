// Package viewer serves a Formidable bundle (.bundle) directly from its
// decrypted archive in memory, without ever extracting it to disk. It is the
// core of the standalone Formidable Viewer app: point an http.Handler at a
// bundle and the webview loads its already-rendered pages, images, and assets
// straight out of the archive.
package viewer

import (
	"archive/zip"
	"bytes"
	"crypto/subtle"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/datadb"
)

// dataEntry is the archive path of the pack's queryable data image, and
// specEntry the OpenAPI document describing it (both written by the exporter
// under the assets folder).
const (
	dataEntry = "_/data.db"
	specEntry = "_/openapi.json"
)

// Bundle is a read-only view over one opened pack: the decrypted payload archive
// plus its cleartext manifest, and (when present) the queryable data image
// behind the agent API. Entries are read on demand through the archive's
// random-access reader; the data image is mounted in memory. Nothing is written
// to disk.
type Bundle struct {
	name     string
	reader   *zip.Reader
	h        http.Handler
	manifest bundle.Manifest
	data     *datadb.DB
	api      http.Handler
}

func newBundle(name string, zr *zip.Reader) *Bundle {
	return &Bundle{name: name, reader: zr, h: http.FileServerFS(zr)}
}

// BundleFromBytes wraps an in-memory archive (the decrypted bundle payload, or a
// zip just produced by the exporter) with no round-trip through disk. A data
// image inside the archive is mounted for the agent API; if it is absent or
// unreadable the bundle still opens for reading, just without an API.
func BundleFromBytes(b []byte, name string) (*Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	bnd := newBundle(name, zr)
	if db, err := openData(zr); err == nil && db != nil {
		bnd.data = db
		bnd.api = datadb.Handler(db, readEntry(zr, specEntry))
	}
	return bnd, nil
}

// openData reads the data image out of the archive and mounts it. Returns
// (nil, nil) when the archive carries no data image.
func openData(zr *zip.Reader) (*datadb.DB, error) {
	raw := readEntry(zr, dataEntry)
	if raw == nil {
		return nil, nil // no data image in this bundle
	}
	return datadb.Open(raw)
}

// readEntry returns the bytes of one archive entry, or nil if it is absent or
// unreadable.
func readEntry(zr *zip.Reader, name string) []byte {
	f, err := zr.Open(name)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	return b
}

// HasData reports whether the bundle carries a queryable data image.
func (b *Bundle) HasData() bool { return b.data != nil }

// ServeAPI serves the read-only agent API over the bundle's data image. It must
// only be called when HasData is true.
func (b *Bundle) ServeAPI(w http.ResponseWriter, r *http.Request) { b.api.ServeHTTP(w, r) }

// Name is the bundle's display name (its file base name, or the name given
// to BundleFromBytes).
func (b *Bundle) Name() string { return b.name }

// HasIndex reports whether the bundle has a root index.html, i.e. looks like
// a real Formidable export rather than an arbitrary zip.
func (b *Bundle) HasIndex() bool {
	f, err := b.reader.Open("index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (b *Bundle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.h.ServeHTTP(w, r)
}

// Close releases the mounted data image, if any. The archive itself is
// byte-backed and holds nothing to release. Called by the server when a bundle
// is swapped out.
func (b *Bundle) Close() error {
	if b.data != nil {
		return b.data.Close()
	}
	return nil
}

// Server serves whichever Bundle is currently loaded, or a landing page when
// none is. It also fronts the read-only agent API under /api/, gated by the
// opt-in ServeAPI setting. It is safe for concurrent use so the loaded bundle
// can be swapped at runtime (e.g. from a native Open dialog) while the webview
// is live.
type Server struct {
	mu       sync.RWMutex
	bundle   *Bundle
	landing  http.Handler
	apiOn    bool
	apiToken string
}

// NewServer returns a Server with no bundle loaded; it serves the landing
// page until SetBundle is called.
func NewServer() *Server {
	return &Server{landing: http.HandlerFunc(serveLanding)}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	b, apiOn, token := s.bundle, s.apiOn, s.apiToken
	s.mu.RUnlock()

	if strings.HasPrefix(r.URL.Path, "/api/") {
		// The agent API answers only when opted in and the bundle carries data;
		// otherwise it is invisible (404), never the HTML fallback.
		if !apiOn || b == nil || !b.HasData() {
			http.NotFound(w, r)
			return
		}
		// Data endpoints require the token; discovery (docs, spec) stays open.
		if datadb.RequiresAuth(r.URL.Path) && !tokenOK(r, token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized: API token required", http.StatusUnauthorized)
			return
		}
		b.ServeAPI(w, r)
		return
	}

	if b == nil {
		s.landing.ServeHTTP(w, r)
		return
	}
	b.ServeHTTP(w, r)
}

// tokenOK reports whether the request carries the API token, accepted as an
// X-API-Key header, an Authorization Bearer token, or a ?key= query parameter.
// An empty configured token fails closed (no request can match).
func tokenOK(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	presented := r.Header.Get("X-API-Key")
	if presented == "" {
		if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			presented = b
		}
	}
	if presented == "" {
		presented = r.URL.Query().Get("key")
	}
	return presented != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

// SetAPIEnabled turns the agent API on or off at runtime, reflecting the
// ServeAPI setting.
func (s *Server) SetAPIEnabled(on bool) {
	s.mu.Lock()
	s.apiOn = on
	s.mu.Unlock()
}

// SetAPIToken installs the token the data endpoints require.
func (s *Server) SetAPIToken(token string) {
	s.mu.Lock()
	s.apiToken = token
	s.mu.Unlock()
}

// APIEnabled reports whether the agent API is currently on.
func (s *Server) APIEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiOn
}

// SetBundle installs b as the current bundle and returns the previous one, if
// any, so the caller can Close it after the webview has reloaded off the new
// one.
func (s *Server) SetBundle(b *Bundle) *Bundle {
	s.mu.Lock()
	prev := s.bundle
	s.bundle = b
	s.mu.Unlock()
	return prev
}

// Current returns the loaded bundle, or nil.
func (s *Server) Current() *Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bundle
}

const landingTitle = "Formidable Viewer"

func serveLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, landingHTML)
}

var landingHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + landingTitle + `</title>
<style>
  html,body{height:100%;margin:0}
  body{display:flex;align-items:center;justify-content:center;
       font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
       background:#1b1e24;color:#e7e9ee}
  .card{text-align:center;max-width:34rem;padding:2.5rem 3rem;border:2px dashed #3a414d;
        border-radius:1rem;background:#20242c}
  h1{font-size:1.4rem;font-weight:600;margin:0 0 .75rem}
  p{margin:.35rem 0;color:#aab1bd;line-height:1.5}
  .hint{margin-top:1rem;font-size:.9rem;color:#7f8794}
  kbd{background:#2a2f38;border-radius:.3rem;padding:.1rem .4rem;font-size:.85em}
</style>
</head>
<body data-file-drop-target>
  <div class="card" data-file-drop-target>
    <h1>` + landingTitle + `</h1>
    <p>Drop a Formidable bundle (<kbd>.bundle</kbd>) here to view it offline.</p>
    <p class="hint">No bundle is open.</p>
  </div>
  <!-- Wails serves its runtime at /wails/runtime.js but does not auto-inject
       it. Native file drops are dispatched through window._wails, so the page
       must load the runtime for drop-to-open to work. -->
  <script type="module" src="/wails/runtime.js"></script>
</body>
</html>
`
