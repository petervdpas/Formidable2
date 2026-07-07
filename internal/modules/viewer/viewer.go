// Package viewer serves a Formidable offline export (.zip) directly from
// the archive, without ever extracting it to disk. It is the core of the
// standalone Formidable Viewer app: point an http.Handler at a bundle and
// the webview loads its already-rendered pages, images, and assets straight
// out of the zip.
package viewer

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"sync"
)

// Bundle is a read-only view over one offline export. The zip stays sealed;
// entries are read on demand through the archive's random-access reader.
type Bundle struct {
	name   string
	reader *zip.Reader
	closer io.Closer // non-nil when opened from a file on disk
	h      http.Handler
}

func newBundle(name string, zr *zip.Reader, closer io.Closer) *Bundle {
	return &Bundle{
		name:   name,
		reader: zr,
		closer: closer,
		h:      http.FileServerFS(zr),
	}
}

// OpenBundle opens a .zip export from disk. The underlying file handle is
// kept open (returned by Close) so entries are served without unpacking.
func OpenBundle(path string) (*Bundle, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return newBundle(filepath.Base(path), &rc.Reader, rc), nil
}

// BundleFromBytes wraps an in-memory .zip (e.g. one just produced by the
// exporter, opened without a round-trip through disk).
func BundleFromBytes(b []byte, name string) (*Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	return newBundle(name, zr, nil), nil
}

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

// Close releases the on-disk file handle. It is a no-op for byte-backed
// bundles.
func (b *Bundle) Close() error {
	if b.closer != nil {
		return b.closer.Close()
	}
	return nil
}

// Server serves whichever Bundle is currently loaded, or a landing page when
// none is. It is safe for concurrent use so the loaded bundle can be swapped
// at runtime (e.g. from a native Open dialog) while the webview is live.
type Server struct {
	mu      sync.RWMutex
	bundle  *Bundle
	landing http.Handler
}

// NewServer returns a Server with no bundle loaded; it serves the landing
// page until SetBundle is called.
func NewServer() *Server {
	return &Server{landing: http.HandlerFunc(serveLanding)}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	b := s.bundle
	s.mu.RUnlock()
	if b == nil {
		s.landing.ServeHTTP(w, r)
		return
	}
	b.ServeHTTP(w, r)
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
    <p>Drop a Formidable export (<kbd>.zip</kbd>) here to view it offline.</p>
    <p class="hint">No bundle is open.</p>
  </div>
</body>
</html>
`
