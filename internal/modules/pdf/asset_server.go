package pdf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// AssetServer is the loopback HTTP listener that serves
// <AppRoot>/pdf/covers/images/<file> to Chrome during a PDF render.
// It exists because picoloom's path rewriter only converts *relative*
// paths to file:// URLs (pathrewrite.go isRelativePath rejects
// IsAbs); a bare absolute path lands in <img src> verbatim, which
// Chrome on Windows refuses to load because `C:` looks like a URL
// scheme. Picoloom's IsURL accepts http(s):// but not file://, so the
// only friction-free fix is to hand picoloom a real http:// URL on
// loopback.
//
// Scope is intentionally narrow: a single rootDir, single route
// `/covers/<filename>`, no traversal, no per-request token map. The
// surface is small enough that the path-traversal guard fits in
// twenty lines. Started once at app boot; the listener lives for the
// process lifetime so the address is stable across PDF exports.
type AssetServer struct {
	listener net.Listener
	server   *http.Server
	rootDir  string
	log      *slog.Logger

	mu     sync.RWMutex
	closed bool
}

// NewAssetServer binds a fresh loopback listener and starts serving
// from rootDir. rootDir must be an absolute path; the constructor
// does NOT mkdir it - a missing dir simply means every request 404s.
// The bind happens synchronously so the caller can rely on Addr()
// returning a usable value the moment NewAssetServer returns.
//
// excludePorts lists ports the asset server must NOT bind to even
// if the OS hands them out - typically the user's configured wiki
// server port (default 8383). If the OS picks an excluded port we
// close the listener and retry, up to bindRetryLimit times before
// giving up. This stops the asset server from squatting on the wiki
// port while the wiki is off, which would block a later Start.
func NewAssetServer(rootDir string, log *slog.Logger, excludePorts ...int) (*AssetServer, error) {
	if log == nil {
		log = slog.Default()
	}
	if !filepath.IsAbs(rootDir) {
		return nil, fmt.Errorf("pdf: asset server rootDir must be absolute: %q", rootDir)
	}
	ln, err := bindAvoidingPorts(excludePorts)
	if err != nil {
		return nil, fmt.Errorf("pdf: asset server bind: %w", err)
	}
	as := &AssetServer{
		listener: ln,
		rootDir:  filepath.Clean(rootDir),
		log:      log,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/covers/", as.handleCovers)
	as.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := as.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pdf: asset server stopped", "err", err)
		}
	}()
	log.Info("pdf: asset server started", "addr", as.Addr(), "root", as.rootDir)
	return as, nil
}

// Addr returns the host:port the listener is bound to, or "" once closed.
func (as *AssetServer) Addr() string {
	if as == nil {
		return ""
	}
	as.mu.RLock()
	defer as.mu.RUnlock()
	if as.closed || as.listener == nil {
		return ""
	}
	return as.listener.Addr().String()
}

// URLFor returns the URL Chrome should hit to load filename from the
// root dir, or "" if the server is closed or filename is not a bare
// basename (path separators / traversal rejected).
func (as *AssetServer) URLFor(filename string) string {
	if as == nil {
		return ""
	}
	addr := as.Addr()
	if addr == "" {
		return ""
	}
	if filename == "" || strings.ContainsAny(filename, `/\`) || strings.Contains(filename, "..") {
		return ""
	}
	return "http://" + addr + "/covers/" + url.PathEscape(filename)
}

// Close shuts the listener down. Safe to call multiple times.
func (as *AssetServer) Close() error {
	if as == nil {
		return nil
	}
	as.mu.Lock()
	if as.closed {
		as.mu.Unlock()
		return nil
	}
	as.closed = true
	srv := as.server
	as.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// bindRetryLimit caps the OS-port-collision retry loop. Hitting it
// in practice would require the OS to repeatedly hand back excluded
// ports out of the entire ephemeral range - practically impossible
// unless the exclusion list itself covers most of the range. We log
// + give up rather than spin indefinitely.
const bindRetryLimit = 10

// bindAvoidingPorts binds 127.0.0.1:0, re-binding if the OS hands back
// a port in excludePorts. Empty list = one attempt, no retry overhead.
func bindAvoidingPorts(excludePorts []int) (net.Listener, error) {
	for range bindRetryLimit {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		addr, ok := ln.Addr().(*net.TCPAddr)
		if !ok || !slices.Contains(excludePorts, addr.Port) {
			return ln, nil
		}
		_ = ln.Close()
	}
	return nil, fmt.Errorf("could not bind loopback port avoiding %v after %d attempts", excludePorts, bindRetryLimit)
}

// handleCovers serves files under rootDir. Sanitisation:
//   - method must be GET or HEAD (no body-mutating verbs)
//   - URL.Path after /covers/ must decode to a bare basename
//     (no path separators, no ".." segments, non-empty)
//   - the joined path must stay under rootDir (belt-and-suspenders
//     against any decoding surprise that slipped past the basename
//     check)
//
// On any rejection: 404 (rather than 400) so a probe can't
// distinguish "wrong path shape" from "file not there".
func (as *AssetServer) handleCovers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/covers/")
	if raw == "" {
		http.NotFound(w, r)
		return
	}
	name, err := url.PathUnescape(raw)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(as.rootDir, name)
	clean := filepath.Clean(full)
	rootWithSep := as.rootDir
	if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
		rootWithSep += string(filepath.Separator)
	}
	if !strings.HasPrefix(clean, rootWithSep) && clean != as.rootDir {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, clean)
}
