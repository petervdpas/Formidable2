package pdf

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFS captures which paths a test wants to consider "present".
// All other paths return false.
type fakeFS map[string]bool

func (f fakeFS) exists(p string) bool { return f[p] }

// fakeVersions answers --version probes deterministically. A nil entry
// means "binary refuses to give a version" (e.g. permission error,
// timeout); the probe should still include the candidate but with an
// empty Version string.
type fakeVersions map[string]struct {
	version string
	err     error
}

func (m fakeVersions) get(p string) (string, error) {
	if v, ok := m[p]; ok {
		return v.version, v.err
	}
	return "", errors.New("no fake version registered")
}

func TestProbe_EnvVarOverrideAlwaysFirst(t *testing.T) {
	fs := fakeFS{
		"/custom/chrome":     true,
		"/usr/bin/chromium":  true,
	}
	vers := fakeVersions{
		"/custom/chrome":    {version: "Chrome 99.0", err: nil},
		"/usr/bin/chromium": {version: "Chromium 148.0", err: nil},
	}
	p := &prober{
		fs:        fs,
		versions:  vers,
		envBin:    "/custom/chrome",
		goos:      "linux",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()

	if len(got.Candidates) == 0 {
		t.Fatal("expected candidates, got none")
	}
	if got.Candidates[0].Path != "/custom/chrome" {
		t.Errorf("first candidate = %q, want env-var override", got.Candidates[0].Path)
	}
	if got.Candidates[0].Source != SourceSystem {
		t.Errorf("env override source = %q, want system", got.Candidates[0].Source)
	}
	if got.Candidates[0].Version != "Chrome 99.0" {
		t.Errorf("env override version = %q", got.Candidates[0].Version)
	}
}

func TestProbe_EnvVarPathMissingFallsThroughToSystem(t *testing.T) {
	fs := fakeFS{
		"/usr/bin/chromium": true,
	}
	vers := fakeVersions{
		"/usr/bin/chromium": {version: "Chromium 148.0", err: nil},
	}
	p := &prober{
		fs:        fs,
		versions:  vers,
		envBin:    "/does/not/exist",
		goos:      "linux",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()

	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got.Candidates))
	}
	if got.Candidates[0].Path != "/usr/bin/chromium" {
		t.Errorf("first candidate = %q, want /usr/bin/chromium", got.Candidates[0].Path)
	}
}

func TestProbe_LinuxSystemPathOrder(t *testing.T) {
	fs := fakeFS{
		"/usr/bin/chromium":             true,
		"/usr/bin/google-chrome-stable": true,
	}
	vers := fakeVersions{
		"/usr/bin/chromium":             {version: "v1", err: nil},
		"/usr/bin/google-chrome-stable": {version: "v2", err: nil},
	}
	p := &prober{
		fs:        fs,
		versions:  vers,
		goos:      "linux",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()

	if len(got.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got.Candidates))
	}
	wantOrder := []string{"/usr/bin/google-chrome-stable", "/usr/bin/chromium"}
	for i, c := range got.Candidates {
		if c.Path != wantOrder[i] {
			t.Errorf("candidate[%d] = %q, want %q", i, c.Path, wantOrder[i])
		}
		if c.Source != SourceSystem {
			t.Errorf("candidate[%d] source = %q, want system", i, c.Source)
		}
	}
}

func TestProbe_NoMatchesReturnsEmpty(t *testing.T) {
	p := &prober{
		fs:        fakeFS{},
		versions:  fakeVersions{},
		goos:      "linux",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()
	if len(got.Candidates) != 0 {
		t.Errorf("got %d candidates, want 0", len(got.Candidates))
	}
}

func TestProbe_VersionFailureKeepsCandidate(t *testing.T) {
	fs := fakeFS{"/usr/bin/chromium": true}
	vers := fakeVersions{
		"/usr/bin/chromium": {version: "", err: errors.New("timeout")},
	}
	p := &prober{
		fs:        fs,
		versions:  vers,
		goos:      "linux",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()
	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1 (version error must not drop candidate)", len(got.Candidates))
	}
	if got.Candidates[0].Version != "" {
		t.Errorf("version on failure = %q, want empty", got.Candidates[0].Version)
	}
}

func TestProbe_ManagedCacheIncluded(t *testing.T) {
	managedBin := filepath.Join("/cache/rod/browser/chromium-1234", "chrome")
	fs := fakeFS{
		managedBin: true,
	}
	vers := fakeVersions{
		managedBin: {version: "Chromium 137.0", err: nil},
	}
	p := &prober{
		fs:           fs,
		versions:     vers,
		goos:         "linux",
		cacheRoot:    "/cache/rod/browser",
		listCacheDir: func(_ string) ([]string, error) { return []string{"chromium-1234"}, nil },
	}

	got := p.Probe()

	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got.Candidates))
	}
	if got.Candidates[0].Source != SourceManaged {
		t.Errorf("source = %q, want managed", got.Candidates[0].Source)
	}
	if got.Candidates[0].Path != managedBin {
		t.Errorf("path = %q, want %q", got.Candidates[0].Path, managedBin)
	}
}

func TestProbe_ManagedHighestRevisionWins(t *testing.T) {
	low := filepath.Join("/cache/rod/browser/chromium-100", "chrome")
	high := filepath.Join("/cache/rod/browser/chromium-9999", "chrome")
	mid := filepath.Join("/cache/rod/browser/chromium-500", "chrome")
	fs := fakeFS{low: true, high: true, mid: true}
	vers := fakeVersions{
		low:  {version: "old", err: nil},
		high: {version: "new", err: nil},
		mid:  {version: "mid", err: nil},
	}
	p := &prober{
		fs:        fs,
		versions:  vers,
		goos:      "linux",
		cacheRoot: "/cache/rod/browser",
		listCacheDir: func(_ string) ([]string, error) {
			return []string{"chromium-100", "chromium-500", "chromium-9999"}, nil
		},
	}

	got := p.Probe()
	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1 (only the latest revision)", len(got.Candidates))
	}
	if !strings.Contains(got.Candidates[0].Path, "chromium-9999") {
		t.Errorf("highest-revision pick = %q, want chromium-9999", got.Candidates[0].Path)
	}
}

func TestProbe_SystemThenManaged(t *testing.T) {
	managedBin := filepath.Join("/cache/rod/browser/chromium-1234", "chrome")
	fs := fakeFS{
		"/usr/bin/chromium": true,
		managedBin:          true,
	}
	vers := fakeVersions{
		"/usr/bin/chromium": {version: "sys", err: nil},
		managedBin:          {version: "managed", err: nil},
	}
	p := &prober{
		fs:           fs,
		versions:     vers,
		goos:         "linux",
		cacheRoot:    "/cache/rod/browser",
		listCacheDir: func(_ string) ([]string, error) { return []string{"chromium-1234"}, nil },
	}

	got := p.Probe()

	if len(got.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got.Candidates))
	}
	if got.Candidates[0].Source != SourceSystem {
		t.Errorf("first source = %q, want system", got.Candidates[0].Source)
	}
	if got.Candidates[1].Source != SourceManaged {
		t.Errorf("second source = %q, want managed", got.Candidates[1].Source)
	}
}

func TestProbe_MacOSPaths(t *testing.T) {
	chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	fs := fakeFS{chromePath: true}
	vers := fakeVersions{chromePath: {version: "Chrome 99", err: nil}}
	p := &prober{
		fs:        fs,
		versions:  vers,
		goos:      "darwin",
		cacheRoot: "/no/such/root",
	}

	got := p.Probe()
	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got.Candidates))
	}
	if got.Candidates[0].Path != chromePath {
		t.Errorf("path = %q, want %q", got.Candidates[0].Path, chromePath)
	}
}

func TestProbe_WindowsPaths(t *testing.T) {
	const winChrome = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	fs := fakeFS{winChrome: true}
	vers := fakeVersions{winChrome: {version: "Chrome 99", err: nil}}
	p := &prober{
		fs:        fs,
		versions:  vers,
		goos:      "windows",
		cacheRoot: `C:\nope`,
		expandEnv: func(s string) string {
			return strings.ReplaceAll(s, "${ProgramFiles}", `C:\Program Files`)
		},
	}

	got := p.Probe()
	if len(got.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got.Candidates))
	}
	if got.Candidates[0].Path != winChrome {
		t.Errorf("path = %q, want %q", got.Candidates[0].Path, winChrome)
	}
}
