package connection

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrExists is returned when an install would replace something already stored.
var ErrExists = errors.New("connection: already exists")

// BundleReader reads one file of a client shipped outside the store, by its
// path relative to the shipped api-clients folder: "<id>.yaml" for the
// definition, "specs/<spec_file>" for the document it names. A file that is
// not there must report fs.ErrNotExist, which is how a document-only bundle is
// told apart from an unreadable one.
type BundleReader func(name string) ([]byte, error)

// Has reports whether a definition is stored under id.
func (m *Manager) Has(id string) bool {
	if checkSlug(id) != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fs != nil && m.fs.FileExists(defPath(id))
}

// specExts are the names a shipped document may carry when no definition
// names it, in the order they are tried.
var specExts = []string{".json", ".yaml", ".yml"}

// Derive drafts a definition from a document alone. Name and auth shape are
// read off the document and the base URL is left to its first server. Each
// detected resource is kept only if it validates by itself: a draft whose
// operation needs a parameter nothing supplies is dropped, not saved broken.
// Operations stay callable by id either way; the resources are a head start.
func Derive(id string, cat *Catalog) Connection {
	c := Connection{ID: id, Name: id, SpecFile: id + ".json"}
	if cat == nil {
		return c
	}
	if title := strings.TrimSpace(cat.Title); title != "" {
		c.Name = title
	}
	c.Auth = cat.Auth
	c.Resources = detectedResources(&c, cat)
	return c
}

// fillFromDocument completes a partial definition with what the document
// states: a name, the auth shape, and the resources. Whatever the definition
// already says (a base URL, say) is kept.
func fillFromDocument(c *Connection, cat *Catalog) {
	if strings.TrimSpace(c.Name) == "" {
		c.Name = Derive(c.ID, cat).Name
	}
	if c.Auth.Kind == "" {
		c.Auth = cat.Auth
	}
	if c.SpecFile == "" {
		c.SpecFile = c.ID + ".json"
	}
	if len(c.Resources) == 0 {
		c.Resources = detectedResources(c, cat)
	}
}

// detectedResources returns the drafts that validate by themselves on c.
func detectedResources(c *Connection, cat *Catalog) []Resource {
	var out []Resource
	for _, d := range DetectResources(cat, c) {
		probe := *c
		probe.Resources = []Resource{d.Resource}
		if len(Validate(&probe, cat)) == 0 {
			out = append(out, d.Resource)
		}
	}
	return out
}

// Create registers a client a program assembled, create-only for the same
// reason Install is. What the draft leaves out is read off the document: the
// name, the auth shape, and the resources. A draft that names resources keeps
// exactly those.
func (m *Manager) Create(c Connection, spec []byte) error {
	if err := checkSlug(c.ID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return errors.New("connection: no filesystem")
	}
	if m.fs.FileExists(defPath(c.ID)) {
		return fmt.Errorf("%w: %s", ErrExists, c.ID)
	}
	cat, err := ParseSpec(spec)
	if err != nil {
		return err
	}
	fillFromDocument(&c, cat)
	c.SpecFile = c.ID + SpecExtension(spec)
	return m.store(c, spec, cat)
}

// Install registers a shipped client, create-only, and reports whether its
// resources were derived. A bundle ships the document as specs/<id>.json (or
// .yaml, .yml) and optionally a definition <id>.yaml. The definition may be
// partial: a base URL alone is enough, and the rest is read off the document.
//
// A stored secret is keyed by client id, so replacing a definition could aim
// that secret at whatever host the shipped copy names: an existing id is
// refused, never updated. The spec lands under the client id whatever the
// bundle called it, and a name another client still uses is refused too.
func (m *Manager) Install(id string, read BundleReader) (derived bool, err error) {
	if err := checkSlug(id); err != nil {
		return false, err
	}
	if read == nil {
		return false, errors.New("connection: nothing to install from")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return false, errors.New("connection: no filesystem")
	}
	if m.fs.FileExists(defPath(id)) {
		return false, fmt.Errorf("%w: %s", ErrExists, id)
	}

	c := Connection{ID: id}
	raw, err := read(id + defExt)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return false, fmt.Errorf("connection: %q is not a valid definition: %w", id, err)
		}
		if c.ID == "" {
			c.ID = id
		}
		if c.ID != id {
			return false, fmt.Errorf("connection: shipped definition %q declares id %q", id, c.ID)
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return false, fmt.Errorf("connection: cannot read shipped definition %q: %w", id, err)
	}

	// A definition that names its document is read from there; one that does
	// not, or no definition at all, takes the document named after the client.
	var spec []byte
	if c.SpecFile != "" {
		if err := checkSpecFile(c.SpecFile); err != nil {
			return false, err
		}
		if spec, err = read(path.Join("specs", c.SpecFile)); err != nil {
			return false, fmt.Errorf("connection: cannot read shipped spec %q: %w", c.SpecFile, err)
		}
	} else {
		for _, ext := range specExts {
			if spec, err = read(path.Join("specs", id+ext)); err == nil {
				break
			}
		}
		if err != nil {
			return false, fmt.Errorf("connection: %q ships no document: %w", id, err)
		}
	}

	cat, err := ParseSpec(spec)
	if err != nil {
		return false, err
	}
	derived = len(c.Resources) == 0
	fillFromDocument(&c, cat)
	c.SpecFile = id + SpecExtension(spec)
	return derived, m.store(c, spec, cat)
}

// store validates and writes a new client: the document under the client id,
// then the definition. Callers hold m.mu and have checked the id is free.
func (m *Manager) store(c Connection, spec []byte, cat *Catalog) error {
	file := c.SpecFile
	if m.fs.FileExists(specPath(file)) && m.specStillUsed(file) {
		return fmt.Errorf("%w: spec %s", ErrExists, file)
	}
	if errs := Validate(&c, cat); len(errs) > 0 {
		return &ValidationFailedError{Errors: errs}
	}
	out, err := yaml.Marshal(&c)
	if err != nil {
		return fmt.Errorf("connection: cannot serialize %q: %w", c.ID, err)
	}
	if err := m.fs.SaveFile(specPath(file), string(spec)); err != nil {
		return fmt.Errorf("connection: cannot store spec: %w", err)
	}
	if err := m.fs.SaveFile(defPath(c.ID), string(out)); err != nil {
		// A spec with no definition is an orphan nothing would ever clean up.
		_ = m.fs.DeleteFile(specPath(file))
		return fmt.Errorf("connection: cannot store definition: %w", err)
	}
	m.cache[file] = cat
	return nil
}
