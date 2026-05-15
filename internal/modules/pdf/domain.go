package pdf

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

// ErrInvalidExportDir is returned when SetExportDir receives a path
// that is not absolute, does not exist, or is not a directory. The
// frontend should surface this as a user-correctable error rather
// than treating it as engine failure.
var ErrInvalidExportDir = errors.New("pdf: invalid export directory")

// Manager owns the runtime activation state of the PDF engine.
// Phase A+B+C (Stage 2 MVP): probe system + managed-cache, persist
// activation per-machine, gate ExportPDF on the active state. Phase
// D (managed download with progress) is deferred to a follow-up.
//
// dirOK validates an export folder candidate exists and is a
// directory. Injected so tests don't touch the real filesystem; in
// production it's a tiny os.Stat wrapper.
//
// All exported methods are safe for concurrent use.
type Manager struct {
	log    *slog.Logger
	store  *store
	prober *prober
	nowFn  func() time.Time
	dirOK  func(string) bool

	mu     sync.RWMutex
	status Status
}

// NewManager constructs an inactive manager. The composition root
// calls Restore() once at boot to load any persisted activation
// from <AppRoot>/config/.pdf-state.json. sys may be nil — Stage 1
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
		dirOK:  realDirOK,
		status: Status{Source: SourceUnset},
	}
}

// realDirOK returns true iff path exists and is a directory. Used in
// production; tests inject a stub.
func realDirOK(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Restore loads the persisted activation record and revalidates it
// against the current filesystem. If the recorded BrowserBin is
// gone (uninstalled between sessions), the activation half of the
// state is cleared and the manager stays inactive; the ExportDir
// preference is independent and survives a stale browser path.
// Similarly, a stale ExportDir is cleared without affecting
// activation. Idempotent; safe to call once at boot.
func (m *Manager) Restore() error {
	st, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("pdf: restore state: %w", err)
	}

	exportDir := st.ExportDir
	if exportDir != "" && !m.dirOK(exportDir) {
		m.log.Warn("pdf: persisted export dir missing; clearing", "path", exportDir)
		exportDir = ""
	}

	activation := Status{Source: SourceUnset, ExportDir: exportDir}
	if st.Source != SourceUnset && st.BrowserBin != "" {
		if m.prober.fs.exists(st.BrowserBin) {
			activation = Status{
				Active:      true,
				BrowserBin:  st.BrowserBin,
				Source:      st.Source,
				Version:     st.Version,
				ActivatedAt: st.ActivatedAt,
				ExportDir:   exportDir,
			}
			m.log.Info("pdf: restored activation", "path", st.BrowserBin, "source", st.Source)
		} else {
			m.log.Warn("pdf: persisted browser missing; clearing activation", "path", st.BrowserBin)
		}
	}

	m.mu.Lock()
	m.status = activation
	m.mu.Unlock()

	// If we trimmed either field, write the cleaned record back so the
	// next Restore is a no-op.
	if exportDir != st.ExportDir || (!activation.Active && st.BrowserBin != "") {
		_ = m.store.Save(state{
			BrowserBin:  activation.BrowserBin,
			Source:      activation.Source,
			Version:     activation.Version,
			ActivatedAt: activation.ActivatedAt,
			ExportDir:   activation.ExportDir,
		})
	}
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
	existingDir := m.Status().ExportDir
	st := state{
		BrowserBin:  chosen.Path,
		Source:      chosen.Source,
		Version:     chosen.Version,
		ActivatedAt: now,
		ExportDir:   existingDir,
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
		ExportDir:   existingDir,
	}
	out := m.status
	m.mu.Unlock()

	m.log.Info("pdf: activated", "path", chosen.Path, "source", chosen.Source, "version", chosen.Version)
	return out, nil
}

// Deactivate flips the manager back to inactive and clears the
// activation half of the persisted state. The managed Chromium
// cache (if present) is NOT deleted — the user can re-activate
// without re-downloading. The ExportDir preference is preserved so
// the user's export folder choice survives a deactivate/activate
// cycle. Idempotent: calling while already inactive is a no-op that
// returns nil.
func (m *Manager) Deactivate() error {
	m.log.Debug("pdf: deactivate")
	existingDir := m.Status().ExportDir
	if err := m.store.Save(state{ExportDir: existingDir}); err != nil {
		return fmt.Errorf("pdf: clear state: %w", err)
	}
	m.mu.Lock()
	m.status = Status{Source: SourceUnset, ExportDir: existingDir}
	m.mu.Unlock()
	m.log.Info("pdf: deactivated")
	return nil
}

// SetExportDir stores the user's preferred export folder. Empty
// path clears the preference (Stage 4 will fall back to placing
// PDFs next to the form). Non-empty paths must be absolute, exist,
// and be a directory — anything else returns ErrInvalidExportDir.
//
// Independent of activation: callable while inactive, doesn't flip
// engine state, doesn't require a browser binding.
func (m *Manager) SetExportDir(path string) (Status, error) {
	m.log.Debug("pdf: set export dir", "path", path)
	if path != "" {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			return m.Status(), fmt.Errorf("%w: %q is not absolute", ErrInvalidExportDir, path)
		}
		if !m.dirOK(path) {
			return m.Status(), fmt.Errorf("%w: %q does not exist or is not a directory", ErrInvalidExportDir, path)
		}
	}

	current := m.Status()
	st := state{
		BrowserBin:  current.BrowserBin,
		Source:      current.Source,
		Version:     current.Version,
		ActivatedAt: current.ActivatedAt,
		ExportDir:   path,
	}
	if st.Source == "" {
		st.Source = SourceUnset
	}
	// Persist a clean record. When inactive the only field worth
	// keeping is ExportDir; an explicit SourceUnset is omitempty so
	// the file is {} when both halves are unset.
	if !current.Active {
		st = state{ExportDir: path}
	}
	if err := m.store.Save(st); err != nil {
		return m.Status(), fmt.Errorf("pdf: persist state: %w", err)
	}

	m.mu.Lock()
	m.status.ExportDir = path
	out := m.status
	m.mu.Unlock()

	if path == "" {
		m.log.Info("pdf: export dir cleared")
	} else {
		m.log.Info("pdf: export dir set", "path", path)
	}
	return out, nil
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
