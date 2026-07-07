package viewer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// HTTPServer is the viewer's optional companion server: it exposes whatever
// http.Handler it wraps (the same *Server the webview uses) over real HTTP on
// all interfaces, so an open bundle can also be viewed in a browser or reached
// from another device on the network. It is off unless Start is called.
type HTTPServer struct {
	handler http.Handler

	mu  sync.Mutex
	srv *http.Server
	ln  net.Listener
}

// NewHTTPServer wraps handler (typically a *Server) but binds nothing yet.
func NewHTTPServer(handler http.Handler) *HTTPServer {
	return &HTTPServer{handler: handler}
}

// Start binds :port on all interfaces (LAN-reachable) and serves in the
// background. If already running it is restarted on the new port, so callers
// can treat Start as "(re)bind here". A port of 0 binds an ephemeral port.
func (s *HTTPServer) Start(port int) error {
	return s.StartOn("", port)
}

// StartOn binds host:port (host "" means all interfaces; "127.0.0.1" keeps it
// loopback-only) and serves in the background. Restarts if already running.
func (s *HTTPServer) StartOn(host string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()

	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.handler, ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv
	s.ln = ln
	go func() {
		// ErrServerClosed is the normal outcome of Stop.
		_ = srv.Serve(ln)
	}()
	return nil
}

// Stop shuts the server down. It is safe to call when not running.
func (s *HTTPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked()
}

func (s *HTTPServer) stopLocked() error {
	if s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := s.srv.Shutdown(ctx)
	s.srv = nil
	s.ln = nil
	return err
}

// Running reports whether the server is currently bound.
func (s *HTTPServer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.srv != nil
}

// Port returns the actual bound TCP port, or 0 when not running.
func (s *HTTPServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return 0
	}
	if a, ok := s.ln.Addr().(*net.TCPAddr); ok {
		return a.Port
	}
	return 0
}

// URLs lists the reachable base URLs while running: localhost plus every
// non-loopback IPv4 interface address, so the UI can show a LAN link. Empty
// when not running.
func (s *HTTPServer) URLs() []string {
	port := s.Port()
	if port == 0 {
		return nil
	}
	urls := []string{fmt.Sprintf("http://localhost:%d", port)}
	for _, ip := range lanIPv4s() {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, port))
	}
	return urls
}

// lanIPv4s returns this host's non-loopback IPv4 addresses.
func lanIPv4s() []string {
	var out []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			out = append(out, v4.String())
		}
	}
	return out
}

// ErrNotRunning is returned by operations that require a running server.
var ErrNotRunning = errors.New("viewer: http server not running")
