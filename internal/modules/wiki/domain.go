package wiki

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// shutdownTimeout caps how long Stop waits for in-flight requests to
// drain before the listener is force-closed. The wiki has no
// long-lived requests by design, so 5s is plenty.
const shutdownTimeout = 5 * time.Second

// Manager owns the HTTP listener lifecycle and the active handler. It
// is safe to call any public method from any goroutine; lifecycle
// operations serialize through `mu`. The handler can be hot-swapped
// while the server runs (composition root mounts the read-path
// handler in Slice 2 without bouncing the listener).
type Manager struct {
	log *slog.Logger

	mu        sync.RWMutex
	handler   http.Handler // never nil; defaultHandler when unset
	server    *http.Server // non-nil while running
	listener  net.Listener // non-nil while running
	startedAt time.Time    // zero when not running

	// servewg tracks the in-flight Serve goroutine. Stop waits on it
	// after Shutdown returns so the kernel has fully released the port
	// before Stop returns to the caller - without this guarantee a
	// rapid Stop→Start on the same port races with the goroutine's
	// listener-close path and bind() returns EADDRINUSE.
	servewg sync.WaitGroup
}

// NewManager builds a manager rooted at the given logger. nil log →
// slog.Default. The server starts in the stopped state - composition
// root calls Start(port) once config is loaded.
func NewManager(log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		log:     log,
		handler: defaultHandler(),
	}
}

// SetHandler installs the active http.Handler. Safe to call before or
// after Start; if the server is running the new handler takes effect
// on the next request without re-binding the port. nil restores the
// default 404 mux - useful for tests and for "I want to deactivate
// routes without stopping the listener" scenarios.
func (m *Manager) SetHandler(h http.Handler) {
	if h == nil {
		h = defaultHandler()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = h
	if m.server != nil {
		m.server.Handler = h
	}
}

// Start binds to localhost:<port> and serves on a background goroutine.
// port = 0 asks the OS for a free port; the actual port is reflected
// via Status. Errors when already running or when the bind fails
// (port-in-use, permission denied, …).
func (m *Manager) Start(port int) error {
	m.mu.Lock()
	if m.server != nil {
		actual := m.actualPortLocked()
		m.mu.Unlock()
		return fmt.Errorf("wiki: server already running on port %d", actual)
	}
	// Listen first so a bind failure surfaces synchronously to the
	// caller (the about workspace toggle wants to know immediately).
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("wiki: bind %q: %w", addr, err)
	}
	srv := &http.Server{
		Handler: m.handler,
		// Modest header timeout - no slow-loris budget on a loopback.
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.server = srv
	m.listener = listener
	m.startedAt = time.Now()
	log := m.log
	bound := listener.Addr().String()
	m.servewg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.servewg.Done()
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("wiki: server exited with error", "err", err, "addr", bound)
		}
	}()
	log.Info("wiki: server started", "addr", bound)
	return nil
}

// Stop gracefully shuts the server down (drains in-flight handlers
// up to shutdownTimeout, then forces the listener closed). No-op
// when the server is not running.
func (m *Manager) Stop() error {
	m.mu.Lock()
	srv := m.server
	listener := m.listener
	m.server = nil
	m.listener = nil
	m.startedAt = time.Time{}
	log := m.log
	m.mu.Unlock()

	if srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(ctx)
	if shutdownErr != nil {
		// Belt-and-braces: if Shutdown couldn't drain, slam the listener
		// shut so the serve goroutine gets ErrServerClosed and exits.
		_ = listener.Close()
		log.Warn("wiki: shutdown forced", "err", shutdownErr)
	}
	// Wait for the serve goroutine to fully exit so the kernel has
	// released the listener before this returns. Without this, a
	// rapid Stop→Start on the same port can race the close path.
	m.servewg.Wait()
	if shutdownErr != nil {
		return fmt.Errorf("wiki: shutdown: %w", shutdownErr)
	}
	log.Info("wiki: server stopped")
	return nil
}

// Status snapshots the live state. Cheap; safe to call from any
// goroutine. Returns a zero Port + zero StartedAt when not running.
func (m *Manager) Status() ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.server == nil {
		return ServerStatus{}
	}
	return ServerStatus{
		Running:   true,
		Port:      m.actualPortLocked(),
		StartedAt: m.startedAt,
	}
}

// actualPortLocked extracts the OS-assigned port from the listener.
// Caller must hold m.mu (read or write).
func (m *Manager) actualPortLocked() int {
	if m.listener == nil {
		return 0
	}
	if addr, ok := m.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// defaultHandler is the placeholder mux installed when no real
// handler has been set. Returns 404 for all paths - useful for tests
// and for the "lifecycle only" composition during early boot before
// the read-path handler is mounted.
func defaultHandler() http.Handler {
	return http.NewServeMux()
}
