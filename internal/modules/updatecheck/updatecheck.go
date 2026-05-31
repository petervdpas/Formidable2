// Package updatecheck performs a best-effort "is there a newer release" probe
// against formidable.tools/api/latest on startup, comparing its version to the
// running about.Version and caching the verdict for the About panel.
//
// Everything here is deliberately silent: on any failure the cached Status
// stays Checked=false and the UI shows nothing. An update check must never
// interrupt or alarm the user.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultEndpoint is the canonical version document.
const DefaultEndpoint = "https://formidable.tools/api/latest"

// RefreshTimeout bounds the startup probe; short so a dead endpoint can't
// delay anything the user sees.
const RefreshTimeout = 6 * time.Second

// devVersion is about.Version's compile-time default. No update is ever
// claimed against it, since the comparison would be meaningless.
const devVersion = "0.1.0"

// Status is the wire shape the About panel reads. Checked distinguishes
// "no newer version" from "we never got a valid answer".
type Status struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	URL             string `json:"url"`
	Checked         bool   `json:"checked"`
}

// remoteRelease mirrors the /api/latest JSON contract.
type remoteRelease struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
}

// Manager owns the cached status and the HTTP probe. endpoint and client are
// package-private so tests can point them at an httptest server.
type Manager struct {
	mu       sync.RWMutex
	status   Status
	client   *http.Client
	endpoint string
	current  string
	enabled  func() bool
}

// NewManager builds the probe. enabled is consulted on every Refresh so the
// update_check config toggle governs the feature live (nil means always on).
// When disabled, Refresh makes no network call and reports an unchecked status.
func NewManager(current string, enabled func() bool) *Manager {
	return &Manager{
		client:   &http.Client{Timeout: RefreshTimeout},
		endpoint: DefaultEndpoint,
		current:  current,
		enabled:  enabled,
		status:   Status{Current: current},
	}
}

// Enabled reports whether the update check is currently turned on.
func (m *Manager) Enabled() bool {
	return m.enabled == nil || m.enabled()
}

// GetStatus returns the last cached verdict; safe to poll.
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Refresh probes the endpoint and updates the cached status. The error is
// returned only for debug-level logging; it must never reach the user. On any
// failure the cached status keeps Checked=false.
func (m *Manager) Refresh(ctx context.Context) (Status, error) {
	if !m.Enabled() {
		cleared := Status{Current: m.current}
		m.mu.Lock()
		m.status = cleared
		m.mu.Unlock()
		return cleared, nil
	}

	rel, err := m.fetch(ctx)
	if err != nil {
		return m.GetStatus(), err
	}

	latest := strings.TrimSpace(rel.Version)
	if latest == "" {
		return m.GetStatus(), errors.New("updatecheck: response had no version")
	}

	st := Status{
		Current: m.current,
		Latest:  latest,
		URL:     rel.URL,
		Checked: true,
	}
	if !isDevVersion(m.current) && compareVersions(latest, m.current) > 0 {
		st.UpdateAvailable = true
	}

	m.mu.Lock()
	m.status = st
	m.mu.Unlock()
	return st, nil
}

func (m *Manager) fetch(ctx context.Context) (remoteRelease, error) {
	var rel remoteRelease
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.endpoint, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rel, errors.New("updatecheck: endpoint returned " + strconv.Itoa(resp.StatusCode))
	}

	// Capped: the document is tiny, so anything large is suspect.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return rel, err
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return rel, err
	}
	return rel, nil
}

// isDevVersion reports whether v is the untagged local-build default.
func isDevVersion(v string) bool {
	return versionCore(v) == devVersion || versionCore(v) == ""
}

// compareVersions compares two version strings by their numeric
// dotted core (leading "v" and any pre-release/build suffix like
// "-dirty" are ignored). Returns -1, 0, or 1 for a < b, a == b, a > b.
func compareVersions(a, b string) int {
	pa, pb := parseCore(a), parseCore(b)
	n := max(len(pa), len(pb))
	for i := range n {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// versionCore extracts the leading numeric-dotted run, e.g.
// "v2.4.8-dirty" -> "2.4.8", "2.4" -> "2.4", "weird" -> "".
func versionCore(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	end := 0
	for end < len(v) {
		c := v[end]
		if (c < '0' || c > '9') && c != '.' {
			break
		}
		end++
	}
	return strings.Trim(v[:end], ".")
}

func parseCore(v string) []int {
	core := versionCore(v)
	if core == "" {
		return nil
	}
	parts := strings.Split(core, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
