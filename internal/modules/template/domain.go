package template

import (
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
	JoinPath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	ListFiles(dir string) ([]string, error)
}

const (
	templateExt    = ".yaml"
	basicYAMLName  = "basic.yaml"
)

// Indexer is the post-write hook surface a downstream cache (e.g. the
// SQLite index used by the wiki/API) plugs into. Manager fires it
// after a successful Save/Delete; failures are logged at the manager
// and never propagated — the index is a derived view, never authoritative.
type Indexer interface {
	OnTemplateChanged(filename string) error
	OnTemplateDeleted(filename string) error
}

// Manager holds the template directory binding.
// Stateless beyond its dependencies (no caching — config owns the VFS cache).
type Manager struct {
	fs           fs
	log          *slog.Logger
	templatesDir string
	indexer      Indexer
}

// NewManager constructs a template manager rooted at <templatesDir> under
// the system's app root. Typical: NewManager(sys, "templates", logger).
func NewManager(filesystem fs, templatesDir string, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if templatesDir == "" {
		templatesDir = "templates"
	}
	return &Manager{fs: filesystem, log: log, templatesDir: templatesDir}
}

// SetIndexer installs the post-write hook. Composition root calls this
// after building both the template manager and the index event handler.
// Pass nil to disable.
func (m *Manager) SetIndexer(i Indexer) { m.indexer = i }

// TemplatesDir returns the absolute path of the templates folder.
// Used by the composition root to stat individual template files
// (mtime + size for the index loader adapter).
func (m *Manager) TemplatesDir() string { return m.templatesDir }

// EnsureTemplateDirectory creates the templates folder if missing.
func (m *Manager) EnsureTemplateDirectory() error {
	return m.fs.EnsureDirectory(m.templatesDir)
}

// ListTemplates returns the YAML filenames in the templates folder.
// Missing folder → empty slice + nil error (matches JS behavior).
func (m *Manager) ListTemplates() ([]string, error) {
	if !m.fs.FileExists(m.templatesDir) {
		return []string{}, nil
	}
	files, err := m.fs.ListFiles(m.templatesDir)
	if err != nil {
		return nil, fmt.Errorf("template: list: %w", err)
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if strings.HasSuffix(f, templateExt) {
			out = append(out, f)
		}
	}
	return out, nil
}

// LoadTemplate reads <name> from the templates folder, parses YAML,
// and returns a sanitized Template.
func (m *Manager) LoadTemplate(name string) (*Template, error) {
	if name == "" {
		return nil, errors.New("template: empty name")
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	if !m.fs.FileExists(full) {
		return nil, fmt.Errorf("template: file not found: %s", full)
	}
	raw, err := m.fs.LoadFile(full)
	if err != nil {
		return nil, fmt.Errorf("template: read %q: %w", name, err)
	}
	var t Template
	if err := unmarshalYAML([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("template: parse %q: %w", name, err)
	}
	if t.Filename == "" {
		t.Filename = name
	}
	return &t, nil
}

// SaveTemplate writes the template to disk in deterministic field order.
// Runs Normalize first so type-specific properties (textarea Format,
// later code/latex/api defaults) are coerced to the canonical shape
// before they hit YAML.
func (m *Manager) SaveTemplate(name string, t *Template) error {
	if name == "" {
		return errors.New("template: empty name")
	}
	if t == nil {
		return errors.New("template: nil template")
	}
	if t.Filename == "" {
		t.Filename = name
	}
	Normalize(t)
	bytes, err := marshalYAML(t)
	if err != nil {
		return fmt.Errorf("template: marshal: %w", err)
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	if err := m.fs.SaveFile(full, string(bytes)); err != nil {
		return err
	}
	if m.indexer != nil {
		if err := m.indexer.OnTemplateChanged(name); err != nil {
			m.log.Warn("template indexer save hook failed", "name", name, "err", err)
		}
	}
	return nil
}

// DeleteTemplate removes the named template file. Missing file is a no-op
// (matches system.DeleteFile semantics).
func (m *Manager) DeleteTemplate(name string) error {
	if name == "" {
		return errors.New("template: empty name")
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	if err := m.fs.DeleteFile(full); err != nil {
		return err
	}
	if m.indexer != nil {
		if err := m.indexer.OnTemplateDeleted(name); err != nil {
			m.log.Warn("template indexer delete hook failed", "name", name, "err", err)
		}
	}
	return nil
}

// Validate runs the validation pipeline on an in-memory template.
// Stateless — useful for the editor before save.
func (m *Manager) Validate(t *Template) []ValidationError {
	return Validate(t)
}

// GetDescriptor returns {name, yaml, storageLocation} for the named
// template. storageLocation is supplied by the caller (config module
// owns the VFS path resolution; passing it in keeps this module
// independent of config).
func (m *Manager) GetDescriptor(name, storageLocation string) (Descriptor, error) {
	t, err := m.LoadTemplate(name)
	if err != nil {
		return Descriptor{}, err
	}
	return Descriptor{
		Name:            name,
		YAML:            t,
		StorageLocation: storageLocation,
	}, nil
}

// GetItemFields returns the top-level (non-loop) text fields in a template.
// Used by the collection editor to choose which field becomes the row label.
func (m *Manager) GetItemFields(name string) ([]ItemField, error) {
	t, err := m.LoadTemplate(name)
	if err != nil {
		return nil, err
	}
	return TopLevelTextFields(t.Fields), nil
}

// SeedBasicIfEmpty creates basic.yaml only when the templates folder
// is currently empty. No-op otherwise (no overwrite).
func (m *Manager) SeedBasicIfEmpty() error {
	if err := m.EnsureTemplateDirectory(); err != nil {
		return err
	}
	files, err := m.ListTemplates()
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return nil
	}

	content := basicTemplate()
	bytes, err := marshalYAML(content)
	if err != nil {
		return err
	}
	full := m.fs.JoinPath(m.templatesDir, basicYAMLName)
	return m.fs.SaveFile(full, string(bytes))
}

// TopLevelTextFields filters fields to those at top-level (not inside any
// loopstart/loopstop pair) and of type "text".
func TopLevelTextFields(fields []Field) []ItemField {
	out := []ItemField{}
	depth := 0
	for _, f := range fields {
		switch f.Type {
		case "loopstart":
			depth++
			continue
		case "loopstop":
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 && f.Type == "text" && f.Key != "" {
			label := f.Label
			if label == "" {
				label = f.Key
			}
			out = append(out, ItemField{Key: f.Key, Label: label})
		}
	}
	return out
}

// guard for editor tools — confirms file extension is yaml
func ensureYAMLExt(name string) string {
	if filepath.Ext(name) == "" {
		return name + templateExt
	}
	return name
}

var _ = ensureYAMLExt // exported on demand later
