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

// AuthorReader yields the active profile's identity. SaveTemplate uses
// it to stamp Template.AuthorName / Template.AuthorEmail when the
// caller leaves them empty (mirrors how record .meta.json files carry
// meta.author_name / meta.author_email). Composition root wires this
// to config.Manager. Nil disables the auto-fill — saves still succeed
// but the fields stay empty.
type AuthorReader interface {
	Author() (name, email string)
}

// AuthorFunc adapts a closure to the AuthorReader interface so the
// composition root can pass `template.AuthorFunc(func() ...)` instead
// of declaring a tiny struct wrapper around config.Manager.
type AuthorFunc func() (name, email string)

// Author satisfies AuthorReader.
func (f AuthorFunc) Author() (string, string) { return f() }

// Manager holds the template directory binding.
// Stateless beyond its dependencies (no caching — config owns the VFS cache).
type Manager struct {
	fs           fs
	log          *slog.Logger
	templatesDir string
	indexer      Indexer
	author       AuthorReader
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

// SetAuthorReader installs the AuthorReader that SaveTemplate uses to
// auto-fill missing AuthorName / AuthorEmail. Composition root wires
// this to the config manager. Pass nil to disable auto-fill.
func (m *Manager) SetAuthorReader(a AuthorReader) { m.author = a }

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

// HasTemplates reports whether at least one *.yaml file exists in
// the templates folder. Used by the ribbon to ghost workspaces that
// require a template to be meaningful (Storage). Errors collapse to
// false — a missing dir, unreadable folder, or any other I/O issue
// is treated as "no templates available".
func (m *Manager) HasTemplates() bool {
	files, err := m.ListTemplates()
	if err != nil {
		return false
	}
	return len(files) > 0
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
	t.NeedsResave = yamlMissingLevelScope([]byte(raw))
	if coerceExpressionItemOffRoot(t.Fields) {
		t.NeedsResave = true
	}
	return &t, nil
}

// SaveTemplate writes the template to disk in deterministic field order.
// Runs Normalize first so type-specific properties (textarea Format,
// later api defaults) are coerced to the canonical shape before they
// hit YAML, then Validate to refuse broken shapes (duplicate keys,
// multiple guid/tags fields, mismatched loop pairs, …). The frontend
// pre-validates the same way; this is defense-in-depth for any caller
// that bypasses the editor (HTTP, sync, scripts).
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
	if t.Fields == nil {
		// An empty template is logically valid (no rules to violate).
		// Validate's invalid-template guard rejects nil Fields, so
		// coerce here to keep the save-time path lenient for empty
		// drafts the editor produces during template creation.
		t.Fields = []Field{}
	}
	// Stamp the author identity from the active profile when missing.
	// Explicitly-set values pass through unchanged so a template
	// authored by Alice keeps Alice's identity even when Bob saves
	// it (meaningful for sync round-trips: only the next save FROM
	// scratch picks up the saver's identity).
	if m.author != nil {
		if t.AuthorName == "" || t.AuthorEmail == "" {
			name, email := m.author.Author()
			if t.AuthorName == "" {
				t.AuthorName = name
			}
			if t.AuthorEmail == "" {
				t.AuthorEmail = email
			}
		}
	}
	Normalize(t)
	if errs := Validate(t); len(errs) > 0 {
		// Surface each validation error to formidable.log so the failure
		// has a paper trail beyond the toast. The frontend pre-validates
		// too, so anything reaching this branch is either a programmatic
		// caller (HTTP/sync) or a stale cache — both worth recording.
		for _, e := range errs {
			m.log.Warn("template validation rejected save",
				"name", name,
				"type", e.Type,
				"key", e.Key,
				"message", e.Message,
				"detail", e.Detail,
			)
		}
		return &ValidationFailedError{Errors: errs}
	}
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
//
// Open-time author backfill: if the loaded template is missing
// AuthorName / AuthorEmail and an AuthorReader is wired, we stamp the
// active profile's identity in memory AND write the stamped YAML back
// to disk as a best-effort. The first user to OPEN an unstamped
// template gets the credit — no need to wait for an explicit save.
// Failures (read-only filesystem, etc.) log a warning but never block
// the descriptor from returning; the editor still gets the in-memory
// stamp.
//
// The write-back bypasses Validate so a template that's broken for
// other reasons (e.g. duplicate keys) still backfills its author.
// SaveTemplate would refuse those, which would be punitive on open —
// the user opened the template to fix the broken state, not to be
// blocked by it.
func (m *Manager) GetDescriptor(name, storageLocation string) (Descriptor, error) {
	t, err := m.LoadTemplate(name)
	if err != nil {
		return Descriptor{}, err
	}
	if m.maybeStampAuthor(t) {
		if writeErr := m.writeYAMLDirect(name, t); writeErr != nil {
			m.log.Warn("template author backfill write failed",
				"name", name, "err", writeErr)
		}
	}
	return Descriptor{
		Name:            name,
		YAML:            t,
		StorageLocation: storageLocation,
	}, nil
}

// maybeStampAuthor fills in t.AuthorName and t.AuthorEmail from the
// wired AuthorReader if either is empty. Returns true when the
// template was modified (caller decides whether to persist). No-op
// when no reader is wired or when both fields are already set.
func (m *Manager) maybeStampAuthor(t *Template) bool {
	if t == nil || m.author == nil {
		return false
	}
	if t.AuthorName != "" && t.AuthorEmail != "" {
		return false
	}
	name, email := m.author.Author()
	changed := false
	if t.AuthorName == "" && name != "" {
		t.AuthorName = name
		changed = true
	}
	if t.AuthorEmail == "" && email != "" {
		t.AuthorEmail = email
		changed = true
	}
	return changed
}

// writeYAMLDirect serialises a template and writes it via the
// filesystem, skipping Validate / Normalize. Open-time backfill uses
// this so a broken-but-openable template still gets its author
// stamped on disk. The indexer hook is intentionally NOT fired —
// nothing semantic changed for downstream consumers.
func (m *Manager) writeYAMLDirect(name string, t *Template) error {
	bytes, err := marshalYAML(t)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	return m.fs.SaveFile(full, string(bytes))
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
