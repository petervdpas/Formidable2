package sfr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// fs is the narrow filesystem surface this module needs.
// *system.Manager satisfies it.
type fs interface {
	ResolvePath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	ListFiles(dir string) ([]string, error)
}

const (
	defaultExtension = ".meta.json"
	mdExtension      = ".md"
)

// Manager applies the SFR conventions on top of a generic filesystem.
type Manager struct {
	fs               fs
	log              *slog.Logger
	defaultExtension string
	defaultJSON      bool
}

// NewManager returns a Manager with the standard `.meta.json` + JSON
// defaults. log may be nil.
func NewManager(filesystem fs, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		fs:               filesystem,
		log:              log,
		defaultExtension: defaultExtension,
		defaultJSON:      true,
	}
}

// ListFiles returns the file names (not paths) under directory whose
// extension matches the requested one. Empty extension uses the manager default.
func (m *Manager) ListFiles(directory, extension string) ([]string, error) {
	if extension == "" {
		extension = m.defaultExtension
	}
	all, err := m.fs.ListFiles(directory)
	if err != nil {
		return nil, fmt.Errorf("sfr: list %q: %w", directory, err)
	}
	out := make([]string, 0, len(all))
	for _, f := range all {
		if strings.HasSuffix(f, extension) {
			out = append(out, f)
		}
	}
	return out, nil
}

// SaveFromBase writes data to <directory>/<normalizedBase><extension>.
// `data` is JSON-marshalled when JSON mode is on (default); otherwise
// it is expected to be a string.
func (m *Manager) SaveFromBase(directory, baseFilename string, data any, opts Options) SaveResult {
	full, err := m.storagePath(directory, baseFilename, opts)
	if err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}

	content, err := m.encode(data, opts)
	if err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}
	if err := m.fs.SaveFile(full, content); err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}
	return SaveResult{Success: true, Path: full}
}

// LoadFromBase reads <directory>/<normalizedBase><extension>.
// Returns the decoded value (any) when JSON mode is on; otherwise the
// raw string.
func (m *Manager) LoadFromBase(directory, baseFilename string, opts Options) (any, error) {
	full, err := m.storagePath(directory, baseFilename, opts)
	if err != nil {
		return nil, err
	}
	if !m.fs.FileExists(full) {
		return nil, fmt.Errorf("sfr: file not found: %s", full)
	}
	raw, err := m.fs.LoadFile(full)
	if err != nil {
		return nil, fmt.Errorf("sfr: load %q: %w", full, err)
	}
	return m.decode(raw, opts), nil
}

// DeleteFromBase removes <directory>/<normalizedBase><extension>.
// Missing files are not an error (matches system.DeleteFile semantics).
func (m *Manager) DeleteFromBase(directory, baseFilename string, opts Options) error {
	full, err := m.storagePath(directory, baseFilename, opts)
	if err != nil {
		return err
	}
	return m.fs.DeleteFile(full)
}

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

// storagePath validates the base filename, normalises it (strips .md
// and the configured extension), and joins with directory. Returns
// an error if the base would escape the directory.
func (m *Manager) storagePath(directory, baseFilename string, opts Options) (string, error) {
	if baseFilename == "" {
		return "", errors.New("sfr: empty base filename")
	}
	if strings.ContainsAny(baseFilename, `/\`) {
		return "", fmt.Errorf("sfr: base filename %q must not contain path separators", baseFilename)
	}
	if baseFilename == "." || baseFilename == ".." || strings.Contains(baseFilename, "..") {
		return "", fmt.Errorf("sfr: base filename %q is not a valid identifier", baseFilename)
	}

	ext := opts.Extension
	if ext == "" {
		ext = m.defaultExtension
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	base := baseFilename
	base = strings.TrimSuffix(base, mdExtension)
	base = strings.TrimSuffix(base, ext)

	resolvedDir := m.fs.ResolvePath(directory)
	full := filepath.Join(resolvedDir, base+ext)

	// Defense in depth: the joined path must remain under the resolved dir.
	rel, err := filepath.Rel(resolvedDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("sfr: %q escapes directory %q", baseFilename, directory)
	}
	if err := m.fs.EnsureDirectory(directory); err != nil {
		return "", fmt.Errorf("sfr: ensure dir %q: %w", directory, err)
	}
	return full, nil
}

func (m *Manager) encode(data any, opts Options) (string, error) {
	useJSON := m.defaultJSON
	if opts.JSON != nil {
		useJSON = *opts.JSON
	}
	if useJSON {
		bytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", fmt.Errorf("sfr: marshal: %w", err)
		}
		return string(bytes), nil
	}
	// Text mode: data must be string.
	switch v := data.(type) {
	case string:
		return v, nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("sfr: text mode expects string, got %T", data)
	}
}

func (m *Manager) decode(raw string, opts Options) any {
	useJSON := m.defaultJSON
	if opts.JSON != nil {
		useJSON = *opts.JSON
	}
	if !useJSON {
		return raw
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// Bad JSON falls through as the raw string - matches the
		// "be liberal in what you accept" stance of the JS version,
		// which also returns the un-parsed string when JSON.parse fails.
		return raw
	}
	return out
}
