package pdf

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// versionTimeout caps how long `<bin> --version` can hang. Chrome
// responds in tens of ms when healthy; anything slower than this is
// effectively unusable for the activation dialog.
const versionTimeout = 3 * time.Second

// prober owns the platform-specific search machinery for Chrome /
// Chromium binaries. Every external dependency is injected so tests
// can mock without touching the filesystem or spawning processes.
//
//   - fs           checks for a path's existence + executability.
//   - versions     resolves <path> --version best-effort.
//   - listCacheDir lists ~/.cache/rod/browser entries (chromium-<rev>).
//   - expandEnv    expands Windows-style ${VAR} tokens; tests stub.
//   - envBin       ROD_BROWSER_BIN snapshot at construction time.
//   - goos         runtime.GOOS at construction time (overridable in tests).
//   - cacheRoot    go-rod's managed browser root.
type prober struct {
	fs           interface{ exists(p string) bool }
	versions     interface{ get(p string) (string, error) }
	listCacheDir func(root string) ([]string, error)
	expandEnv    func(s string) string

	envBin    string
	goos      string
	cacheRoot string
}

func newProber() *prober {
	return &prober{
		fs:           realFS{},
		versions:     realVersions{},
		listCacheDir: realListCacheDir,
		expandEnv:    os.ExpandEnv,
		envBin:       os.Getenv("ROD_BROWSER_BIN"),
		goos:         runtime.GOOS,
		cacheRoot:    defaultRodCacheRoot(),
	}
}

// Probe returns every Chrome/Chromium binary the activation flow can
// adopt, in priority order: env-var override, then platform system
// paths in their conventional order, then managed-cache picks
// (highest revision first). Each candidate is best-effort versioned;
// a binary that refuses `--version` is still returned with empty
// Version so the dialog can decide whether to surface it.
func (p *prober) Probe() ProbeResult {
	seen := map[string]bool{}
	candidates := []ChromeCandidate{}

	add := func(path string, src Source) {
		if path == "" || seen[path] {
			return
		}
		if !p.fs.exists(path) {
			return
		}
		seen[path] = true
		ver, _ := p.versions.get(path)
		candidates = append(candidates, ChromeCandidate{
			Path:    path,
			Source:  src,
			Version: ver,
		})
	}

	if p.envBin != "" {
		add(p.envBin, SourceSystem)
	}
	for _, sp := range p.systemPaths() {
		add(sp, SourceSystem)
	}
	for _, mp := range p.managedPaths() {
		add(mp, SourceManaged)
	}
	return ProbeResult{Candidates: candidates}
}

// systemPaths returns the GOOS-specific conventional install paths,
// in user-priority order (Google Chrome → Chromium → Edge).
func (p *prober) systemPaths() []string {
	switch p.goos {
	case "linux":
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		expand := p.expandEnv
		if expand == nil {
			expand = os.ExpandEnv
		}
		return []string{
			expand(`${ProgramFiles}\Google\Chrome\Application\chrome.exe`),
			expand(`${ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe`),
			expand(`${LocalAppData}\Google\Chrome\Application\chrome.exe`),
			expand(`${ProgramFiles}\Microsoft\Edge\Application\msedge.exe`),
		}
	}
	return nil
}

// managedPaths discovers go-rod's downloaded Chromium revisions and
// returns binary paths sorted highest-revision-first so the latest
// pinned download wins.
func (p *prober) managedPaths() []string {
	if p.cacheRoot == "" || p.listCacheDir == nil {
		return nil
	}
	entries, err := p.listCacheDir(p.cacheRoot)
	if err != nil || len(entries) == 0 {
		return nil
	}
	revs := []int{}
	revToEntry := map[int]string{}
	for _, e := range entries {
		rev, ok := parseChromiumRevision(e)
		if !ok {
			continue
		}
		revs = append(revs, rev)
		revToEntry[rev] = e
	}
	sort.Sort(sort.Reverse(sort.IntSlice(revs)))
	// Only the latest revision is user-relevant. Older revisions in the
	// cache are go-rod's housekeeping (failed extractions, abandoned
	// pins) and would clutter the activation dialog.
	if len(revs) == 0 {
		return nil
	}
	return []string{filepath.Join(p.cacheRoot, revToEntry[revs[0]], managedBinaryName(p.goos))}
}

// parseChromiumRevision pulls the integer revision out of go-rod's
// "chromium-<rev>" directory name. Anything else returns (0, false).
func parseChromiumRevision(dir string) (int, bool) {
	const prefix = "chromium-"
	if !strings.HasPrefix(dir, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(dir[len(prefix):])
	if err != nil {
		return 0, false
	}
	return n, true
}

// managedBinaryName is the chrome executable name go-rod unpacks
// inside chromium-<rev>/ for the given GOOS.
func managedBinaryName(goos string) string {
	switch goos {
	case "windows":
		return "chrome.exe"
	case "darwin":
		return filepath.Join("Chromium.app", "Contents", "MacOS", "Chromium")
	}
	return "chrome"
}

// defaultRodCacheRoot mirrors go-rod's `os.UserCacheDir() / rod /
// browser` default. Empty string when the user cache dir is
// unavailable — probe gracefully skips managed candidates.
func defaultRodCacheRoot() string {
	c, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(c, "rod", "browser")
}

// realFS is the production filesystem check — exists + not-a-dir.
type realFS struct{}

func (realFS) exists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// realVersions runs `<path> --version` with a short timeout and
// returns the trimmed first line. Used in production; tests inject a
// fakeVersions instead.
type realVersions struct{}

func (realVersions) get(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return "", errors.New("empty --version output")
	}
	return line, nil
}

// realListCacheDir reads the rod browser cache directory and returns
// child entry names. Missing dir → empty slice (not an error) so the
// probe treats it as "no managed downloads yet".
func realListCacheDir(root string) ([]string, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d.IsDir() {
			out = append(out, d.Name())
		}
	}
	return out, nil
}
