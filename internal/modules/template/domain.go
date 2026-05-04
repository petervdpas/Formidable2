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

// Manager holds the template directory binding.
// Stateless beyond its dependencies (no caching — config owns the VFS cache).
type Manager struct {
	fs           fs
	log          *slog.Logger
	templatesDir string
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
	bytes, err := marshalYAML(t)
	if err != nil {
		return fmt.Errorf("template: marshal: %w", err)
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	return m.fs.SaveFile(full, string(bytes))
}

// DeleteTemplate removes the named template file. Missing file is a no-op
// (matches system.DeleteFile semantics).
func (m *Manager) DeleteTemplate(name string) error {
	if name == "" {
		return errors.New("template: empty name")
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	return m.fs.DeleteFile(full)
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
