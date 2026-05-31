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

const shutdownTimeout = 5 * time.Second

// Manager owns the HTTP listener lifecycle and the active handler. All methods are goroutine-safe;
// lifecycle ops serialize through mu. The handler can be hot-swapped while running.
type Manager struct {
	log *slog.Logger

	mu        sync.RWMutex
	handler   http.Handler // never nil; defaultHandler when unset
	server    *http.Server // non-nil while running
	listener  net.Listener // non-nil while running
	startedAt time.Time    // zero when not running

	// Stop waits on servewg after Shutdown so the kernel releases the port before returning;
	// otherwise a rapid Stop->Start on the same port races the listener-close path and bind() gets EADDRINUSE.
	servewg sync.WaitGroup
}

// NewManager builds a stopped manager rooted at log (nil -> slog.Default).
func NewManager(log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		log:     log,
		handler: defaultHandler(),
	}
}

// SetHandler installs the active handler (takes effect on the next request); nil restores the default 404 mux.
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

// Start binds localhost:<port> and serves on a goroutine; port 0 asks the OS for a free port (read it via Status).
func (m *Manager) Start(port int) error {
	m.mu.Lock()
	if m.server != nil {
		actual := m.actualPortLocked()
		m.mu.Unlock()
		return fmt.Errorf("wiki: server already running on port %d", actual)
	}
	// Listen first so a bind failure surfaces synchronously to the caller.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("wiki: bind %q: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           m.handler,
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

// Stop drains in-flight handlers up to shutdownTimeout, then forces the listener closed (no-op when not running).
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
		// Shutdown couldn't drain: slam the listener so the serve goroutine gets ErrServerClosed and exits.
		_ = listener.Close()
		log.Warn("wiki: shutdown forced", "err", shutdownErr)
	}
	// Wait for the serve goroutine to exit so the kernel releases the listener (see servewg above).
	m.servewg.Wait()
	if shutdownErr != nil {
		return fmt.Errorf("wiki: shutdown: %w", shutdownErr)
	}
	log.Info("wiki: server stopped")
	return nil
}

// Status snapshots the live state (zero Port + StartedAt when not running).
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

// actualPortLocked extracts the OS-assigned port; caller must hold m.mu.
func (m *Manager) actualPortLocked() int {
	if m.listener == nil {
		return 0
	}
	if addr, ok := m.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// defaultHandler is the placeholder 404 mux installed before a real handler is set.
func defaultHandler() http.Handler {
	return http.NewServeMux()
}
