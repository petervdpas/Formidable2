package connection

import (
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// On-disk layout, relative to AppRoot. One definition and one spec per client,
// named to match the API Clients panel so the folder is recognisable to anyone
// browsing it. Clients sit outside the context folder on purpose: they are
// machine scoped and carry credentials, so they must not ride along with a git
// or gigot sync of the user's templates and storage.
//
//	<AppRoot>/api-clients/<id>.yaml         the binding definition
//	<AppRoot>/api-clients/specs/<id>.json   the uploaded swagger, verbatim
const (
	dirClients = "api-clients"
	dirSpecs   = "api-clients/specs"
	defExt     = ".yaml"
)

// ErrNotFound is returned when no connection is stored under an id.
var ErrNotFound = errors.New("connection: not found")

// ValidationFailedError wraps the issue set so callers can errors.As to it
// instead of parsing a message.
type ValidationFailedError struct {
	Errors []ValidationError
}

func (e *ValidationFailedError) Error() string {
	parts := make([]string, len(e.Errors))
	for i, v := range e.Errors {
		parts[i] = v.Type
	}
	return "connection: validation failed: " + strings.Join(parts, ", ")
}

// Summary is the list-view row. OK is false when the definition is unreadable,
// its spec is missing, or a binding no longer matches the spec.
type Summary struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	SpecFile  string `json:"spec_file,omitempty"`
	Resources int    `json:"resources"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// SpecInfo is what an upload produced: the filename it landed under and the
// catalog parsed from it. The caller writes File into Connection.SpecFile.
type SpecInfo struct {
	File    string   `json:"file"`
	Catalog *Catalog `json:"catalog"`
}

// storeFS is the filesystem surface this module needs. Paths are AppRoot
// relative; SaveFile is expected to be atomic.
type storeFS interface {
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path, content string) error
	DeleteFile(path string) error
	ListDir(path string) ([]string, error)
}

// Manager owns the connection definitions and the parsed spec cache. All
// exported methods are safe for concurrent use; writes are serialized so a
// read-modify-write can never interleave with another.
type Manager struct {
	fs  storeFS
	log *slog.Logger

	mu    sync.Mutex
	cache map[string]*Catalog
}

func NewManager(fs storeFS, log *slog.Logger) *Manager {
	return &Manager{fs: fs, log: log, cache: map[string]*Catalog{}}
}

// Reload drops every cached catalog, so the next read reparses from disk. The
// cache is only invalidated by this module's own writes; call this after the
// user edits a spec file by hand.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = map[string]*Catalog{}
}

// ImportSpec parses an uploaded document, stores it verbatim under
// specs/<name>.<ext>, and returns where it landed. The upload is never
// rewritten: a drift check compares the stored bytes against the remote, and a
// reformat would make every comparison a false positive.
func (m *Manager) ImportSpec(name string, data []byte) (SpecInfo, error) {
	if err := checkSlug(name); err != nil {
		return SpecInfo{}, err
	}
	cat, err := ParseSpec(data)
	if err != nil {
		return SpecInfo{}, err
	}
	file := name + SpecExtension(data)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return SpecInfo{}, errors.New("connection: no filesystem")
	}
	if err := m.fs.SaveFile(specPath(file), string(data)); err != nil {
		return SpecInfo{}, fmt.Errorf("connection: cannot store spec: %w", err)
	}
	m.cache[file] = cat
	return SpecInfo{File: file, Catalog: cat}, nil
}

// Get returns a connection and the catalog parsed from its spec.
func (m *Manager) Get(id string) (*Connection, *Catalog, error) {
	if err := checkSlug(id); err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.load(id)
	if err != nil {
		return nil, nil, err
	}
	cat, err := m.catalog(c.SpecFile)
	if err != nil {
		return nil, nil, err
	}
	return c, cat, nil
}

// Catalog returns the parsed spec behind a connection.
func (m *Manager) Catalog(id string) (*Catalog, error) {
	_, cat, err := m.Get(id)
	return cat, err
}

// Save validates a connection against its spec and writes it only if every
// binding holds. A definition on disk is therefore always executable.
func (m *Manager) Save(c *Connection) error {
	if c == nil {
		return errors.New("connection: nothing to save")
	}
	if err := checkSlug(c.ID); err != nil {
		return err
	}
	if err := checkSpecFile(c.SpecFile); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return errors.New("connection: no filesystem")
	}
	cat, err := m.catalog(c.SpecFile)
	if err != nil {
		return err
	}
	if errs := Validate(c, cat); len(errs) > 0 {
		return &ValidationFailedError{Errors: errs}
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("connection: cannot serialize %q: %w", c.ID, err)
	}
	return m.fs.SaveFile(defPath(c.ID), string(out))
}

// Delete removes a connection. Its spec goes too, unless another connection
// still points at the same file: one document routinely backs an acceptance
// and a production connection.
func (m *Manager) Delete(id string) error {
	if err := checkSlug(id); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return errors.New("connection: no filesystem")
	}

	c, err := m.load(id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err := m.fs.DeleteFile(defPath(id)); err != nil {
		return err
	}
	if err != nil || c.SpecFile == "" {
		return nil
	}
	if m.specStillUsed(c.SpecFile) {
		return nil
	}
	delete(m.cache, c.SpecFile)
	return m.fs.DeleteFile(specPath(c.SpecFile))
}

// List returns every stored connection, sorted by id. A definition that fails
// to load or validate is listed with OK false rather than dropped: an invisible
// broken connection is worse than a visible one.
func (m *Manager) List() ([]Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return nil, errors.New("connection: no filesystem")
	}
	ids, err := m.ids()
	if err != nil {
		return nil, err
	}

	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		s := Summary{ID: id}
		c, err := m.load(id)
		if err != nil {
			s.Error = err.Error()
			out = append(out, s)
			continue
		}
		s.Name, s.SpecFile, s.Resources = c.Name, c.SpecFile, len(c.Resources)

		cat, err := m.catalog(c.SpecFile)
		if err != nil {
			s.Error = err.Error()
			out = append(out, s)
			continue
		}
		s.Title = cat.Title
		if errs := Validate(c, cat); len(errs) > 0 {
			s.Error = (&ValidationFailedError{Errors: errs}).Error()
		} else {
			s.OK = true
		}
		out = append(out, s)
	}
	return out, nil
}

// ids returns the stored connection ids, sorted. Dotfiles and anything that is
// not a .yaml definition are skipped, so a stray note in the folder is inert.
func (m *Manager) ids() ([]string, error) {
	entries, err := m.fs.ListDir(dirClients)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if strings.HasPrefix(e, ".") || !strings.HasSuffix(e, defExt) {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e, defExt))
	}
	slices.Sort(ids)
	return ids, nil
}

// load reads and decodes one definition. Callers hold m.mu.
func (m *Manager) load(id string) (*Connection, error) {
	if m.fs == nil {
		return nil, errors.New("connection: no filesystem")
	}
	raw, err := m.fs.LoadFile(defPath(id))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	var c Connection
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("connection: %q is not a valid definition: %w", id, err)
	}
	if c.ID == "" {
		c.ID = id
	}
	return &c, nil
}

// catalog returns the parsed spec for a spec filename, reading it once and
// keeping it until Reload. Callers hold m.mu.
func (m *Manager) catalog(specFile string) (*Catalog, error) {
	if err := checkSpecFile(specFile); err != nil {
		return nil, err
	}
	if cat, ok := m.cache[specFile]; ok {
		return cat, nil
	}
	raw, err := m.fs.LoadFile(specPath(specFile))
	if err != nil {
		return nil, fmt.Errorf("connection: spec %q is missing", specFile)
	}
	cat, err := ParseSpec([]byte(raw))
	if err != nil {
		return nil, err
	}
	m.cache[specFile] = cat
	return cat, nil
}

// specStillUsed reports whether any stored connection references specFile.
// Callers hold m.mu.
func (m *Manager) specStillUsed(specFile string) bool {
	ids, err := m.ids()
	if err != nil {
		// Unreadable folder: keep the spec. Leaving a file is recoverable,
		// deleting one another connection needs is not.
		return true
	}
	for _, id := range ids {
		c, err := m.load(id)
		if err == nil && c.SpecFile == specFile {
			return true
		}
	}
	return false
}

func defPath(id string) string    { return path.Join(dirClients, id+defExt) }
func specPath(file string) string { return path.Join(dirSpecs, file) }

// checkSlug guards every id that becomes a filename.
func checkSlug(id string) error {
	if !slugRe.MatchString(id) {
		return fmt.Errorf("connection: invalid id %q", id)
	}
	return nil
}

// checkSpecFile keeps a spec reference a bare filename inside specs/, so a
// hand-edited definition cannot read or delete a file elsewhere on disk.
func checkSpecFile(file string) error {
	if file == "" {
		return errors.New("connection: no spec file")
	}
	if file != path.Base(file) || strings.ContainsAny(file, `/\`) || strings.HasPrefix(file, ".") {
		return fmt.Errorf("connection: spec file %q must be a plain name inside specs/", file)
	}
	return nil
}
