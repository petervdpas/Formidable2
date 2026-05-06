package wiki

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestNewManager_NilLoggerFallsBackToDefault(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("manager nil")
	}
	if m.log == nil {
		t.Error("logger not set; expected slog.Default fallback")
	}
}

func TestStatus_BeforeStart(t *testing.T) {
	m := NewManager(nil)
	s := m.Status()
	if s.Running {
		t.Error("expected not running")
	}
	if s.Port != 0 {
		t.Errorf("port = %d, want 0", s.Port)
	}
	if !s.StartedAt.IsZero() {
		t.Errorf("StartedAt = %v, want zero", s.StartedAt)
	}
}

func TestStartStopRoundTrip(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatalf("start: %v", err)
	}

	s := m.Status()
	if !s.Running {
		t.Error("expected running")
	}
	if s.Port == 0 {
		t.Error("expected non-zero bound port")
	}
	if s.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}

	// Verify the listener actually accepts requests.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", s.Port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()

	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.Status().Running {
		t.Error("expected not running after stop")
	}
}

func TestStop_WhenIdleIsNoOp(t *testing.T) {
	m := NewManager(nil)
	if err := m.Stop(); err != nil {
		t.Errorf("stop on idle should be no-op, got %v", err)
	}
	// Calling again is also fine.
	if err := m.Stop(); err != nil {
		t.Errorf("second stop on idle should be no-op, got %v", err)
	}
}

func TestStart_DoubleStartErrors(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if err := m.Start(0); err == nil {
		t.Error("expected error on double start")
	}
}

func TestStart_PortInUseErrors(t *testing.T) {
	a := NewManager(nil)
	if err := a.Start(0); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	port := a.Status().Port
	b := NewManager(nil)
	err := b.Start(port)
	if err == nil {
		_ = b.Stop()
		t.Fatalf("expected port-in-use error on port %d", port)
	}
}

func TestStartStopRestartSamePort(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	port := m.Status().Port
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(port); err != nil {
		t.Fatalf("restart on same port: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	if got := m.Status().Port; got != port {
		t.Errorf("port = %d, want %d", got, port)
	}
}

func TestSetHandler_BeforeStart(t *testing.T) {
	m := NewManager(nil)
	m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "before")
	}))
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if got := bodyAt(t, m.Status().Port, "/"); got != "before" {
		t.Errorf("body = %q, want before", got)
	}
}

func TestSetHandler_HotSwapWhileRunning(t *testing.T) {
	m := NewManager(nil)
	m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "v1")
	}))
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	if got := bodyAt(t, m.Status().Port, "/"); got != "v1" {
		t.Fatalf("v1 body = %q", got)
	}

	m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "v2")
	}))

	if got := bodyAt(t, m.Status().Port, "/"); got != "v2" {
		t.Errorf("v2 body = %q, want v2", got)
	}
}

func TestSetHandler_NilFallsBackToDefault(t *testing.T) {
	m := NewManager(nil)
	m.SetHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "real")
	}))
	m.SetHandler(nil) // nil should restore default 404 mux, not crash
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", m.Status().Port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestStatus_ConcurrentReadsDoNotRace(t *testing.T) {
	// `go test -race` enforces this; the assertions are just smoke.
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(50 * time.Millisecond)
			for time.Now().Before(deadline) {
				_ = m.Status()
			}
		}()
	}
	wg.Wait()
}

// ── helpers ─────────────────────────────────────────────────────────

func bodyAt(t *testing.T, port int, path string) string {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(body)
}
