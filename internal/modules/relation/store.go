package relation

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// fs is the storage seam for the subsystem's OWN files (satisfied by system.Manager).
type fs interface {
	FileExists(path string) bool
	EnsureDirectory(path string) error
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
}

// Catalog is the narrow port over the main templates + records. The app implements it over the
// dataprovider; this module imports neither template nor storage. Keep it to these two questions.
type Catalog interface {
	IsCollection(template string) bool
	RecordExists(template, id string) bool
}

// Manager reads and writes a template's relations, one file per template under relationsDir.
type Manager struct {
	fs  fs
	cat Catalog
	dir string
	mu  sync.Mutex
}

func NewManager(filesystem fs, relationsDir string, catalog Catalog) *Manager {
	if relationsDir == "" {
		relationsDir = "relations"
	}
	return &Manager{fs: filesystem, cat: catalog, dir: relationsDir}
}

func (m *Manager) path(template string) string { return filepath.Join(m.dir, template) }

// GetRelations returns the relations declared by a template (nil if none). Reads tolerate whatever
// is on disk, including edges gone stale since a record was deleted.
func (m *Manager) GetRelations(template string) ([]Relation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.path(template)
	if !m.fs.FileExists(p) {
		return nil, nil
	}
	raw, err := m.fs.LoadFile(p)
	if err != nil {
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return nil, fmt.Errorf("relation: parse %s: %w", template, err)
	}
	return f.Relations, nil
}

// SetRelations declares a template's relations: the source and every target must be a live
// collection (hard reject), then persists. Edge record-existence is NOT checked here: edges are
// volatile (records get deleted out from under them), so a bulk re-save must tolerate stale edges.
func (m *Manager) SetRelations(template string, rels []Relation) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("relation: empty template")
	}
	if !m.cat.IsCollection(template) {
		return fmt.Errorf("relation: %s is not a collection", template)
	}
	for _, r := range rels {
		if !m.cat.IsCollection(r.To) {
			return fmt.Errorf("relation: target %s (relation %q) is not a collection", r.To, r.Name)
		}
	}
	return m.saveRelations(template, rels)
}

// saveRelations is the persistence floor: structural validation + atomic write, NO catalog checks.
// Edge mutations and cleanup go through it so removal/persist works even against degraded state.
func (m *Manager) saveRelations(template string, rels []Relation) error {
	for i, r := range rels {
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("relation: #%d has no name", i+1)
		}
		if strings.TrimSpace(r.To) == "" {
			return fmt.Errorf("relation: %q has no target", r.Name)
		}
		if !r.Cardinality.valid() {
			return fmt.Errorf("relation: %q has unknown cardinality %q", r.Name, r.Cardinality)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(file{Template: template, Relations: rels}); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fs.EnsureDirectory(m.dir); err != nil {
		return err
	}
	return m.fs.SaveFile(m.path(template), buf.String())
}

// AddEdge links two records through a named relation. Both records must exist right now (hard
// reject): a brand-new dangling link is never allowed, even though edges can dangle later.
func (m *Manager) AddEdge(template, relationName string, e Edge) error {
	if strings.TrimSpace(e.From) == "" || strings.TrimSpace(e.To) == "" {
		return fmt.Errorf("relation: edge needs both from and to")
	}
	rels, err := m.GetRelations(template)
	if err != nil {
		return err
	}
	i := relationIndex(rels, relationName)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation named %q", template, relationName)
	}
	if !m.cat.RecordExists(template, e.From) {
		return fmt.Errorf("relation: source record %q not found in %s", e.From, template)
	}
	if !m.cat.RecordExists(rels[i].To, e.To) {
		return fmt.Errorf("relation: target record %q not found in %s", e.To, rels[i].To)
	}
	if slices.Contains(rels[i].Edges, e) {
		return fmt.Errorf("relation: edge %s -> %s already exists", e.From, e.To)
	}
	rels[i].Edges = append(rels[i].Edges, e)
	return m.saveRelations(template, rels)
}

// RemoveEdge unlinks two records. Goes through the persistence floor so cleanup works even when the
// target template or records have since gone away (the volatile case).
func (m *Manager) RemoveEdge(template, relationName string, e Edge) error {
	rels, err := m.GetRelations(template)
	if err != nil {
		return err
	}
	i := relationIndex(rels, relationName)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation named %q", template, relationName)
	}
	before := len(rels[i].Edges)
	rels[i].Edges = slices.DeleteFunc(rels[i].Edges, func(x Edge) bool { return x == e })
	if len(rels[i].Edges) == before {
		return fmt.Errorf("relation: edge %s -> %s not found", e.From, e.To)
	}
	return m.saveRelations(template, rels)
}

func relationIndex(rels []Relation, name string) int {
	for i, r := range rels {
		if r.Name == name {
			return i
		}
	}
	return -1
}
