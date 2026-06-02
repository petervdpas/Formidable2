package template

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/petervdpas/formidable2/internal/util/keymu"
)

// fs is the filesystem surface this module needs.
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
	templateExt   = ".yaml"
	basicYAMLName = "basic.yaml"
)

// Indexer is the post-write hook (the SQLite index) fired after a
// successful Save/Delete. Failures are logged and never propagated.
type Indexer interface {
	OnTemplateChanged(filename string) error
	OnTemplateDeleted(filename string) error
}

// Observer is a deletion-only post-hook for additional listeners beyond
// the Indexer. Multiple may register; failures are logged, not propagated.
type Observer interface {
	OnTemplateDeleted(filename string) error
}

// ObserverFunc adapts a closure to Observer.
type ObserverFunc func(filename string) error

func (f ObserverFunc) OnTemplateDeleted(name string) error { return f(name) }

// CreationObserver fires when SaveTemplate writes a brand-new file.
// Updates of existing templates do NOT fire it.
type CreationObserver interface {
	OnTemplateCreated(filename string) error
}

// CreationObserverFunc adapts a closure to CreationObserver.
type CreationObserverFunc func(filename string) error

func (f CreationObserverFunc) OnTemplateCreated(name string) error { return f(name) }

// AuthorReader yields the active profile's identity to stamp empty Author fields; nil disables auto-fill.
type AuthorReader interface {
	Author() (name, email string)
}

// AuthorFunc adapts a closure to AuthorReader.
type AuthorFunc func() (name, email string)

func (f AuthorFunc) Author() (string, string) { return f() }

// Manager holds the template directory binding. LoadTemplate is cached + serialized per-filename so a
// 50-row sidebar mount storm parses once, not N times. The cache assumes this process is the only writer;
// external edits are not detected.
type Manager struct {
	fs           fs
	log          *slog.Logger
	templatesDir string
	indexer      Indexer
	observers    []Observer
	creationObs  []CreationObserver
	author       AuthorReader

	loadMu  keymu.Map
	cacheMu sync.RWMutex
	cache   map[string]*Template
}

// NewManager constructs a template manager rooted at templatesDir under the app root.
func NewManager(filesystem fs, templatesDir string, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if templatesDir == "" {
		templatesDir = "templates"
	}
	return &Manager{fs: filesystem, log: log, templatesDir: templatesDir}
}

// SetIndexer installs the post-write hook (nil disables).
func (m *Manager) SetIndexer(i Indexer) { m.indexer = i }

// AddObserver registers a deletion observer; observers run in registration order, failures logged not propagated.
func (m *Manager) AddObserver(o Observer) {
	if o == nil {
		return
	}
	m.observers = append(m.observers, o)
}

// AddCreationObserver registers a listener that fires only when SaveTemplate writes a new file; failures logged not propagated.
func (m *Manager) AddCreationObserver(o CreationObserver) {
	if o == nil {
		return
	}
	m.creationObs = append(m.creationObs, o)
}

// SetAuthorReader installs the AuthorReader SaveTemplate uses to auto-fill missing Author fields (nil disables).
func (m *Manager) SetAuthorReader(a AuthorReader) { m.author = a }

// TemplatesDir returns the absolute path of the templates folder.
func (m *Manager) TemplatesDir() string { return m.templatesDir }

// EnsureTemplateDirectory creates the templates folder if missing.
func (m *Manager) EnsureTemplateDirectory() error {
	return m.fs.EnsureDirectory(m.templatesDir)
}

// ListTemplates returns the YAML filenames in the templates folder; missing folder yields an empty slice.
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

// HasTemplates reports whether at least one *.yaml exists; any I/O error collapses to false.
func (m *Manager) HasTemplates() bool {
	files, err := m.ListTemplates()
	if err != nil {
		return false
	}
	return len(files) > 0
}

// LoadTemplate reads, parses, and sanitizes <name>. Cached + serialized per-name; the returned
// pointer may be shared across callers, so treat it as read-only.
func (m *Manager) LoadTemplate(name string) (*Template, error) {
	if name == "" {
		return nil, errors.New("template: empty name")
	}
	if t := m.cacheGet(name); t != nil {
		return t, nil
	}

	unlock := m.loadMu.Lock(name)
	defer unlock()

	if t := m.cacheGet(name); t != nil {
		return t, nil
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
	// Lift legacy statistics[].scaling into the top-level scalings catalog on
	// load, so the editor and the S["name"] calculator see the migrated shape
	// without waiting for a save. In-memory only (sets NeedsResave to persist).
	if migrateLegacyScalings(&t) {
		t.NeedsResave = true
	}
	m.cachePut(name, &t)
	return &t, nil
}

// LoadMany resolves each name in order; missing files emit a (nil, Error) slot rather than aborting the batch.
func (m *Manager) LoadMany(names []string) []LoadManyResult {
	out := make([]LoadManyResult, len(names))
	for i, n := range names {
		t, err := m.LoadTemplate(n)
		if err != nil {
			out[i] = LoadManyResult{Filename: n, Error: err.Error()}
			continue
		}
		out[i] = LoadManyResult{Filename: n, Template: t}
	}
	return out
}

func (m *Manager) cacheGet(name string) *Template {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()
	return m.cache[name]
}

func (m *Manager) cachePut(name string, t *Template) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.cache == nil {
		m.cache = map[string]*Template{}
	}
	m.cache[name] = t
}

func (m *Manager) cacheClear(name string) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	delete(m.cache, name)
}

// SaveTemplate Normalizes then Validates before writing in deterministic field order; the Validate pass
// is defense-in-depth for callers that bypass the editor (HTTP, sync, scripts).
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
		// Coerce nil to empty so empty creation drafts pass Validate's nil-Fields guard.
		t.Fields = []Field{}
	}
	// Stamp author only when missing, so a template keeps its original author across sync round-trips.
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
		// Log each error: anything reaching here bypassed the frontend pre-validation.
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
	// Decide create-vs-update before writing so CreationObserver fires only on the truly-new path.
	isNew := !m.fs.FileExists(full)
	if err := m.fs.SaveFile(full, string(bytes)); err != nil {
		return err
	}
	m.cacheClear(name)
	if m.indexer != nil {
		if err := m.indexer.OnTemplateChanged(name); err != nil {
			m.log.Warn("template indexer save hook failed", "name", name, "err", err)
		}
	}
	if isNew {
		for _, o := range m.creationObs {
			if err := o.OnTemplateCreated(name); err != nil {
				m.log.Warn("template creation observer failed", "name", name, "err", err)
			}
		}
	}
	return nil
}

// DeleteTemplate removes the named template file (missing file is a no-op).
func (m *Manager) DeleteTemplate(name string) error {
	if name == "" {
		return errors.New("template: empty name")
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	if err := m.fs.DeleteFile(full); err != nil {
		return err
	}
	m.cacheClear(name)
	if m.indexer != nil {
		if err := m.indexer.OnTemplateDeleted(name); err != nil {
			m.log.Warn("template indexer delete hook failed", "name", name, "err", err)
		}
	}
	for _, o := range m.observers {
		if err := o.OnTemplateDeleted(name); err != nil {
			m.log.Warn("template observer delete hook failed", "name", name, "err", err)
		}
	}
	return nil
}

// Validate runs the stateless validation pipeline on an in-memory template.
func (m *Manager) Validate(t *Template) []ValidationError {
	return Validate(t)
}

// GetDescriptor returns {name, yaml, storageLocation} for the named template.
// On open it backfills a missing author in memory and best-effort writes it back, bypassing Validate
// so a broken-but-openable template still gets stamped (blocking on open would be punitive: the user opened it to fix it).
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

// maybeStampAuthor fills missing Author fields from the wired AuthorReader; returns true when modified.
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

// writeYAMLDirect writes a template skipping Validate/Normalize and the indexer hook (nothing semantic changed).
func (m *Manager) writeYAMLDirect(name string, t *Template) error {
	bytes, err := marshalYAML(t)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	return m.fs.SaveFile(full, string(bytes))
}

// GetItemFields returns the top-level (non-loop) text fields in a template.
func (m *Manager) GetItemFields(name string) ([]ItemField, error) {
	t, err := m.LoadTemplate(name)
	if err != nil {
		return nil, err
	}
	return TopLevelTextFields(t.Fields), nil
}

// SeedBasicIfEmpty creates basic.yaml only when the templates folder is empty (never overwrites).
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

// TopLevelTextFields returns type-"text" fields outside any loopstart/loopstop pair.
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

func ensureYAMLExt(name string) string {
	if filepath.Ext(name) == "" {
		return name + templateExt
	}
	return name
}

var _ = ensureYAMLExt
