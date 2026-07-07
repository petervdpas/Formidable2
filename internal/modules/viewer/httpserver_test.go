package viewer

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func getURL(t *testing.T, url string) (int, string) {
	t.Helper()
	var lastErr error
	// The server starts in a goroutine; retry briefly until it accepts.
	for range 50 {
		res, err := http.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		return res.StatusCode, string(body)
	}
	t.Fatalf("GET %s never succeeded: %v", url, lastErr)
	return 0, ""
}

func TestHTTPServerServesCurrentBundle(t *testing.T) {
	srv := NewServer()
	srv.SetBundle(sampleBundle(t))

	hs := NewHTTPServer(srv)
	if err := hs.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer hs.Stop()

	if !hs.Running() {
		t.Fatal("Running() = false after Start")
	}
	port := hs.Port()
	if port == 0 {
		t.Fatal("Port() = 0 while running")
	}

	status, body := getURL(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	if status != http.StatusOK || !strings.Contains(body, "HOME") {
		t.Fatalf("GET / = %d %q, want 200 with bundle index", status, body)
	}
}

func TestHTTPServerReflectsBundleSwap(t *testing.T) {
	srv := NewServer() // no bundle yet -> landing page
	hs := NewHTTPServer(srv)
	if err := hs.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer hs.Stop()
	base := fmt.Sprintf("http://127.0.0.1:%d/", hs.Port())

	if _, body := getURL(t, base); !strings.Contains(body, landingTitle) {
		t.Fatalf("expected landing before a bundle is set")
	}

	srv.SetBundle(sampleBundle(t))
	if status, body := getURL(t, base); status != http.StatusOK || !strings.Contains(body, "HOME") {
		t.Fatalf("after swap GET / = %d %q, want bundle index", status, body)
	}
}

func TestHTTPServerStopAndRestart(t *testing.T) {
	hs := NewHTTPServer(NewServer())

	if err := hs.Stop(); err != nil {
		t.Fatalf("Stop when not running should be nil: %v", err)
	}
	if err := hs.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := hs.Port()

	// Start again rebinds (restart semantics), yielding a fresh listener.
	if err := hs.Start(0); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !hs.Running() {
		t.Fatal("not running after restart")
	}
	_ = first

	if err := hs.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if hs.Running() {
		t.Fatal("still running after Stop")
	}
	if hs.Port() != 0 {
		t.Fatalf("Port() = %d after Stop, want 0", hs.Port())
	}
	if urls := hs.URLs(); urls != nil {
		t.Fatalf("URLs() = %v after Stop, want nil", urls)
	}
}

func TestHTTPServerURLsIncludeLocalhost(t *testing.T) {
	hs := NewHTTPServer(NewServer())
	if err := hs.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer hs.Stop()
	urls := hs.URLs()
	if len(urls) == 0 || !strings.HasPrefix(urls[0], "http://localhost:") {
		t.Fatalf("URLs() = %v, want localhost first", urls)
	}
}
