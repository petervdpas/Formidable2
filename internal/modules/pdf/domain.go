package pdf

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ErrNoBrowserFound is returned when an auto-pick Activate finds no
// Chrome/Chromium candidates. The frontend's activation dialog
// should respond by offering the managed-download flow (Phase D).
var ErrNoBrowserFound = errors.New("pdf: no Chrome or Chromium found")

// ErrInvalidBrowserBin is returned when an explicit Activate path
// does not exist, is not executable, or refuses --version.
var ErrInvalidBrowserBin = errors.New("pdf: invalid browser binary")

// Manager owns the runtime activation state of the PDF engine.
// Phase A+B+C (Stage 2 MVP): probe system + managed-cache, persist
// activation per-machine, gate ExportPDF on the active state. Phase
// D (managed download with progress) is deferred to a follow-up.
//
// All exported methods are safe for concurrent use.
type Manager struct {
	log     *slog.Logger
	store   *store
	prober  *prober
	nowFn   func() time.Time

	mu     sync.RWMutex
	status Status
}

// NewManager constructs an inactive manager. The composition root
// calls Restore() once at boot to load any persisted activation
// from <AppRoot>/config/pdf-state.json. sys may be nil — Stage 1
// tests and headless test runs pass nil and the store no-ops.
func NewManager(log *slog.Logger, sys storeFS) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		log:    log,
		store:  &store{fs: sys, log: log},
		prober: newProber(),
		nowFn:  time.Now,
		status: Status{Source: SourceUnset},
	}
}

// Restore loads the persisted activation record and revalidates it
// against the current filesystem. If the recorded BrowserBin is
// gone (uninstalled between sessions), the state file is cleared
// and the manager stays inactive. Idempotent; safe to call once at
// boot.
func (m *Manager) Restore() error {
	st, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("pdf: restore state: %w", err)
	}
	if st.Source == SourceUnset || st.BrowserBin == "" {
		return nil
	}
	if !m.prober.fs.exists(st.BrowserBin) {
		m.log.Warn("pdf: persisted browser missing; clearing state", "path", st.BrowserBin)
		_ = m.store.Clear()
		return nil
	}
	m.mu.Lock()
	m.status = Status{
		Active:      true,
		BrowserBin:  st.BrowserBin,
		Source:      st.Source,
		Version:     st.Version,
		ActivatedAt: st.ActivatedAt,
	}
	m.mu.Unlock()
	m.log.Info("pdf: restored activation", "path", st.BrowserBin, "source", st.Source)
	return nil
}

// Status returns the live snapshot. Zero value (Active=false,
// Source=SourceUnset) is the fresh-install / deactivated state.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Activate adopts a Chrome/Chromium binary. If opts.BrowserBin is
// non-empty, that exact path is validated and adopted. Otherwise
// the prober runs and the first candidate (env-var override >
// system path > managed cache) is picked. Returns ErrNoBrowserFound
// when no candidate is available — Formidable does not bundle or
// download Chrome; the user must install one and re-probe.
func (m *Manager) Activate(opts ActivateOpts) (Status, error) {
	m.log.Debug("pdf: activate", "browser_bin", opts.BrowserBin)

	var chosen ChromeCandidate
	if opts.BrowserBin != "" {
		c, err := m.validate(opts.BrowserBin)
		if err != nil {
			return m.Status(), err
		}
		chosen = c
	} else {
		probe := m.prober.Probe()
		if len(probe.Candidates) == 0 {
			return m.Status(), ErrNoBrowserFound
		}
		chosen = probe.Candidates[0]
	}

	now := m.nowFn()
	st := state{
		BrowserBin:  chosen.Path,
		Source:      chosen.Source,
		Version:     chosen.Version,
		ActivatedAt: now,
	}
	if err := m.store.Save(st); err != nil {
		return m.Status(), fmt.Errorf("pdf: persist state: %w", err)
	}

	m.mu.Lock()
	m.status = Status{
		Active:      true,
		BrowserBin:  chosen.Path,
		Source:      chosen.Source,
		Version:     chosen.Version,
		ActivatedAt: now,
	}
	out := m.status
	m.mu.Unlock()

	m.log.Info("pdf: activated", "path", chosen.Path, "source", chosen.Source, "version", chosen.Version)
	return out, nil
}

// Deactivate flips the manager back to inactive and clears the
// persisted state. The managed Chromium cache (if present) is NOT
// deleted — the user can re-activate without re-downloading.
// Idempotent: calling while already inactive is a no-op that
// returns nil.
func (m *Manager) Deactivate() error {
	m.log.Debug("pdf: deactivate")
	if err := m.store.Clear(); err != nil {
		return fmt.Errorf("pdf: clear state: %w", err)
	}
	m.mu.Lock()
	m.status = Status{Source: SourceUnset}
	m.mu.Unlock()
	m.log.Info("pdf: deactivated")
	return nil
}

// Export is a Stage 4 entry point. The Active check short-circuits
// here so the rendering pipeline is never constructed for an
// inactive engine — Chrome must not boot until the user has opted in.
func (m *Manager) Export(formGUID string, opts ExportOpts) (Result, error) {
	m.log.Debug("pdf: export", "form_guid", formGUID, "output_path", opts.OutputPath, "style", opts.Style)
	if !m.Status().Active {
		return Result{}, ErrPDFNotActivated
	}
	return Result{}, errors.New("pdf: export not yet implemented (Stage 4)")
}

// Probe runs the activation probe without mutating state. Used by
// the Information-page dialog to render the candidate list before
// the user picks one.
func (m *Manager) Probe() ProbeResult { return m.prober.Probe() }

// validate checks a single path: file exists, --version responds,
// and infers Source from whether the path lives under the managed
// cache root.
func (m *Manager) validate(path string) (ChromeCandidate, error) {
	if !m.prober.fs.exists(path) {
		return ChromeCandidate{}, fmt.Errorf("%w: %s does not exist", ErrInvalidBrowserBin, path)
	}
	ver, err := m.prober.versions.get(path)
	if err != nil {
		return ChromeCandidate{}, fmt.Errorf("%w: %s --version failed: %v", ErrInvalidBrowserBin, path, err)
	}
	src := SourceSystem
	if m.prober.cacheRoot != "" && strings.HasPrefix(path, m.prober.cacheRoot) {
		src = SourceManaged
	}
	return ChromeCandidate{Path: path, Source: src, Version: ver}, nil
}
