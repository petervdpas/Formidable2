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

	"github.com/petervdpas/formidable2/internal/util/keymu"
)

// ErrNoBrowserFound is returned when an auto-pick Activate finds no
// Chrome/Chromium candidates.
var ErrNoBrowserFound = errors.New("pdf: no Chrome or Chromium found")

// ErrInvalidBrowserBin is returned when an Activate path is missing,
// not executable, or refuses --version.
var ErrInvalidBrowserBin = errors.New("pdf: invalid browser binary")

// ErrInvalidExportDir is returned when SetExportDir gets a path that
// is not absolute, missing, or not a directory.
var ErrInvalidExportDir = errors.New("pdf: invalid export directory")

// Manager owns the runtime activation state of the PDF engine. All
// exported methods are safe for concurrent use. formMu serializes
// Export calls for the same (template, datafile); distinct forms
// render in parallel.
type Manager struct {
	log    *slog.Logger
	store  *store
	prober *prober
	nowFn  func() time.Time
	dirOK  func(string) bool

	renderer  renderer
	storage   storageFS
	templates templateLoader
	convertFn converterFactory
	formMu    keymu.Map

	mu          sync.RWMutex
	status      Status
	lastSuccess *ExportTelemetry
	lastFailure *ExportTelemetry
	// assetServer feeds Chrome central-library cover logos during render.
	// Optional: nil falls back to absolute-path resolution (Linux-only).
	assetServer *AssetServer
}

// SetAssetServer plugs in the loopback listener that feeds picoloom
// http:// URLs for cover-library logos.
func (m *Manager) SetAssetServer(as *AssetServer) {
	m.mu.Lock()
	m.assetServer = as
	m.mu.Unlock()
}

// AssetServer returns the attached asset server, or nil if none was wired.
func (m *Manager) AssetServer() *AssetServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.assetServer
}

// LastExport returns the most recent success and failure telemetry.
// The returned pointers are copies, so callers need no lock.
func (m *Manager) LastExport() ExportTelemetrySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var s, f *ExportTelemetry
	if m.lastSuccess != nil {
		cp := *m.lastSuccess
		s = &cp
	}
	if m.lastFailure != nil {
		cp := *m.lastFailure
		f = &cp
	}
	return ExportTelemetrySnapshot{LastSuccess: s, LastFailure: f}
}

func (m *Manager) recordSuccess(t *ExportTelemetry) {
	if t == nil {
		return
	}
	m.mu.Lock()
	m.lastSuccess = t
	m.mu.Unlock()
}

func (m *Manager) recordFailure(t *ExportTelemetry) {
	if t == nil {
		return
	}
	m.mu.Lock()
	m.lastFailure = t
	m.mu.Unlock()
}

// NewManager constructs an inactive manager; call Restore() at boot.
// A nil tpl skips the manifest merge layer (best-effort); a nil
// convertFn defaults to the picoloom-backed factory.
func NewManager(log *slog.Logger, sys storeFS, rdr renderer, stg storageFS, tpl templateLoader, convertFn converterFactory) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if convertFn == nil {
		convertFn = realConverterFactory
	}
	return &Manager{
		log:       log,
		store:     &store{fs: sys, log: log},
		prober:    newProber(),
		nowFn:     time.Now,
		dirOK:     realDirOK,
		renderer:  rdr,
		storage:   stg,
		templates: tpl,
		convertFn: convertFn,
		status:    Status{Source: SourceUnset},
	}
}

func realDirOK(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Restore loads the persisted activation and revalidates it against
// the filesystem. A vanished BrowserBin clears activation; a stale
// ExportDir is cleared independently. Idempotent.
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

	// Write the cleaned record back so the next Restore is a no-op.
	if exportDir != st.ExportDir || (!activation.Active && st.BrowserBin != "") {
		_ = m.store.Save(state{
			BrowserBin:  activation.BrowserBin,
			Source:      activation.Source,
			Version:     activation.Version,
			ActivatedAt: activation.ActivatedAt,
			ExportDir:   activation.ExportDir,
		})
	}

	if err := scaffoldCovers(m.store.fs, m.log); err != nil {
		m.log.Warn("pdf: cover scaffold failed; library may be incomplete", "err", err)
	}

	return nil
}

// SaveCover validates and persists a cover HTML to disk, rejecting
// with ErrCoverInvalid on any error-severity issue.
func (m *Manager) SaveCover(name, html string) error {
	m.log.Debug("pdf: save cover", "name", name)
	if err := saveDiskCover(m.store.fs, name, html); err != nil {
		return err
	}
	m.log.Info("pdf: cover saved", "name", name)
	return nil
}

// ListCovers returns descriptors for every cover under
// <AppRoot>/pdf/covers/. signature.html is filtered out; invalid
// files come back OK=false rather than being dropped.
func (m *Manager) ListCovers() ([]CoverDescriptor, error) {
	return listDiskCovers(m.store.fs)
}

// LoadCover returns the raw cover HTML for editing. Validation is NOT
// applied, so the editor can load a broken cover for the user to fix.
func (m *Manager) LoadCover(name string) (string, error) {
	return loadDiskCoverRaw(m.store.fs, name)
}

// DeleteCover removes <name>.html. Deleting a seed cover only removes
// the disk file; the next boot's scaffold re-writes it from the embed,
// so the frontend should phrase seed deletion as "Reset".
func (m *Manager) DeleteCover(name string) error {
	m.log.Debug("pdf: delete cover", "name", name)
	if err := deleteDiskCover(m.store.fs, name); err != nil {
		return err
	}
	m.log.Info("pdf: cover deleted", "name", name)
	return nil
}

// ExportCoverArchive bundles a cover and its image refs into a zip at
// zipPath (absolute or AppRoot-relative). Missing refs are reported
// without aborting.
func (m *Manager) ExportCoverArchive(name, zipPath string) (ExportCoverArchiveResult, error) {
	m.log.Debug("pdf: export cover archive", "name", name, "zip", zipPath)
	res, err := exportCoverArchive(m.store.fs, name, zipPath)
	if err != nil {
		return res, err
	}
	m.log.Info("pdf: cover archive exported", "name", name, "zip", res.ZipPath, "images", len(res.Images), "missing", len(res.MissingImages))
	return res, nil
}

// ImportCoverArchive unpacks a cover archive zip into
// <AppRoot>/pdf/covers/. Refuses to clobber an existing cover unless
// overwrite=true; bundled images always replace (not user state).
func (m *Manager) ImportCoverArchive(zipPath string, overwrite bool) (ImportCoverArchiveResult, error) {
	m.log.Debug("pdf: import cover archive", "zip", zipPath, "overwrite", overwrite)
	res, err := importCoverArchive(m.store.fs, zipPath, overwrite)
	if err != nil {
		return res, err
	}
	m.log.Info("pdf: cover archive imported", "name", res.Name, "overwritten", res.Overwritten, "images", len(res.Images))
	return res, nil
}

// Status returns the live snapshot; zero value is the inactive state.
func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// Activate adopts a Chrome/Chromium binary: opts.BrowserBin when set,
// else the prober's first candidate. Returns ErrNoBrowserFound when
// none is available; Formidable does not bundle or download Chrome.
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

// Deactivate flips back to inactive and clears the activation state.
// The managed Chromium cache is NOT deleted (re-activate without
// re-download) and ExportDir is preserved. Idempotent.
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

// SetExportDir stores the preferred export folder. Empty clears it
// (renders then land next to the form); non-empty must be absolute,
// existent, and a directory, else ErrInvalidExportDir. Independent of
// activation.
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
	// When inactive only ExportDir matters; drop the rest so the file
	// is {} when both halves are unset.
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

// Export lives in render.go; its Active check short-circuits there so
// Chrome never boots for an inactive engine.

// Probe runs the activation probe without mutating state.
func (m *Manager) Probe() ProbeResult { return m.prober.Probe() }

// validate checks one path (exists + --version) and infers Source from
// whether it lives under the managed cache root.
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
