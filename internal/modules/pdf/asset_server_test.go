package pdf

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRunningServer spins a temp rootDir, drops a couple of files, and
// returns a started AssetServer. Caller defers as.Close().
func newRunningServer(t *testing.T) (*AssetServer, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "formidable.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatalf("seed svg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "team.png"), []byte("fakepng"), 0o644); err != nil {
		t.Fatalf("seed png: %v", err)
	}
	as, err := NewAssetServer(root, nil)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return as, root
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func TestAssetServer_RejectsRelativeRoot(t *testing.T) {
	if _, err := NewAssetServer("relative/path", nil); err == nil {
		t.Fatal("expected error for relative rootDir, got nil")
	}
}

func TestAssetServer_AddrAfterClose(t *testing.T) {
	as, _ := newRunningServer(t)
	if as.Addr() == "" {
		t.Fatal("Addr() should be non-empty while running")
	}
	_ = as.Close()
	if as.Addr() != "" {
		t.Fatal("Addr() should return empty after Close")
	}
}

func TestAssetServer_HappyPathServesFile(t *testing.T) {
	as, _ := newRunningServer(t)
	defer as.Close()
	resp, body := get(t, as.URLFor("formidable.svg"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if string(body) != "<svg/>" {
		t.Fatalf("body: %q", body)
	}
}

func TestAssetServer_MissingFile404(t *testing.T) {
	as, _ := newRunningServer(t)
	defer as.Close()
	resp, _ := get(t, "http://"+as.Addr()+"/covers/ghost.png")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAssetServer_RejectsTraversalDotDot(t *testing.T) {
	as, root := newRunningServer(t)
	defer as.Close()
	// Seed an outside file we hope traversal would reach.
	parentDir := filepath.Dir(root)
	if err := os.WriteFile(filepath.Join(parentDir, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cases := []string{
		"http://" + as.Addr() + "/covers/../secret.txt",
		"http://" + as.Addr() + "/covers/%2e%2e/secret.txt",
		"http://" + as.Addr() + "/covers/%2E%2E%2Fsecret.txt",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			resp, body := get(t, u)
			if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "nope") {
				t.Fatalf("traversal leak: %s served %q", u, body)
			}
		})
	}
}

func TestAssetServer_RejectsSubpath(t *testing.T) {
	as, root := newRunningServer(t)
	defer as.Close()
	// File inside a real subdir; URLFor should refuse to build it AND
	// a hand-crafted URL that asks for a subpath should 404.
	sub := filepath.Join(root, "deep")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.png"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed inner: %v", err)
	}
	if u := as.URLFor("deep/inner.png"); u != "" {
		t.Fatalf("URLFor should refuse subpath, got %q", u)
	}
	resp, _ := get(t, "http://"+as.Addr()+"/covers/deep/inner.png")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("subpath status: %d", resp.StatusCode)
	}
}

func TestAssetServer_RejectsNonGet(t *testing.T) {
	as, _ := newRunningServer(t)
	defer as.Close()
	req, err := http.NewRequest(http.MethodPost, "http://"+as.Addr()+"/covers/formidable.svg", strings.NewReader(""))
	if err != nil {
		t.Fatalf("req: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAssetServer_URLForEmptyFilename(t *testing.T) {
	as, _ := newRunningServer(t)
	defer as.Close()
	if u := as.URLFor(""); u != "" {
		t.Fatalf("empty filename should yield empty URL, got %q", u)
	}
}

func TestAssetServer_URLForBackslashRejected(t *testing.T) {
	as, _ := newRunningServer(t)
	defer as.Close()
	if u := as.URLFor(`win\path.png`); u != "" {
		t.Fatalf("backslash filename should yield empty URL, got %q", u)
	}
}

func TestAssetServer_URLForEscapesSpaces(t *testing.T) {
	as, root := newRunningServer(t)
	defer as.Close()
	if err := os.WriteFile(filepath.Join(root, "team logo.png"), []byte("spaced"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	u := as.URLFor("team logo.png")
	if !strings.Contains(u, "team%20logo.png") {
		t.Fatalf("URL not percent-encoded: %q", u)
	}
	resp, body := get(t, u)
	if resp.StatusCode != http.StatusOK || string(body) != "spaced" {
		t.Fatalf("space-name fetch: status=%d body=%q", resp.StatusCode, body)
	}
}

func TestAssetServer_CloseIsIdempotent(t *testing.T) {
	as, _ := newRunningServer(t)
	if err := as.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}
	if err := as.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}
}

func TestAssetServer_AvoidsExcludedPort(t *testing.T) {
	// Bind a "wiki" listener to claim a known port, ask the asset
	// server to avoid that port. We then close the placeholder so the
	// OS would have been free to hand it out — the asset server must
	// still have refused it.
	placeholder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("placeholder listen: %v", err)
	}
	wikiPort := placeholder.Addr().(*net.TCPAddr).Port
	_ = placeholder.Close()

	root := t.TempDir()
	as, err := NewAssetServer(root, nil, wikiPort)
	if err != nil {
		t.Fatalf("NewAssetServer: %v", err)
	}
	defer as.Close()
	got := as.listener.Addr().(*net.TCPAddr).Port
	if got == wikiPort {
		t.Fatalf("asset server squatted on excluded port %d", wikiPort)
	}
}

func TestAssetServer_NilSafe(t *testing.T) {
	var as *AssetServer
	if as.Addr() != "" {
		t.Fatal("nil Addr() should be empty")
	}
	if as.URLFor("x.png") != "" {
		t.Fatal("nil URLFor should be empty")
	}
	if err := as.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}
