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
	ListFiles(dir string) ([]string, error)
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
	return m.getRelationsLocked(template)
}

// getRelationsLocked reads + parses a template's file; caller holds m.mu.
func (m *Manager) getRelationsLocked(template string) ([]Relation, error) {
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

// SetRelations declares a template's relations and keeps the relationship stored on BOTH sides:
// the source and every target must be a live collection (hard reject), then the template's own file
// is written AND each target's file gets the flipped counterpart (the inverse). A target no longer
// referenced has its counterpart removed. The two sides are one relationship persisted twice, so it
// can be read and self-healed from either end.
func (m *Manager) SetRelations(template string, rels []Relation) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("relation: empty template")
	}
	if !m.cat.IsCollection(template) {
		return fmt.Errorf("relation: %s is not a collection", template)
	}
	for _, r := range rels {
		if !m.cat.IsCollection(r.To) {
			return fmt.Errorf("relation: target %s is not a collection", r.To)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	prev, err := m.getRelationsLocked(template)
	if err != nil {
		return err
	}
	if err := m.saveRelationsLocked(template, rels); err != nil {
		return err
	}

	// Mirror each relation onto its target with the flipped cardinality. A self-relation
	// (to == template) is its own mirror, so skip it to avoid clobbering its entry.
	newTargets := make(map[string]Cardinality, len(rels))
	for _, r := range rels {
		if r.To == template {
			continue
		}
		newTargets[r.To] = r.Cardinality
	}
	for _, r := range rels {
		if r.To == template {
			continue
		}
		if err := m.upsertMirrorLocked(r.To, template, r.Cardinality.inverse(), !r.Inverse); err != nil {
			return err
		}
	}
	// Drop the counterpart from targets this template no longer relates to.
	for _, r := range prev {
		if r.To == template {
			continue
		}
		if _, still := newTargets[r.To]; !still {
			if err := m.removeMirrorLocked(r.To, template); err != nil {
				return err
			}
		}
	}
	return nil
}

// upsertMirrorLocked writes/updates the counterpart entry (to backTo, with card and inverse flag) in
// target's file, preserving every other entry in that file. The counterpart's Inverse is the
// opposite of the source half, so the pair always has one non-inverse and one inverse side. Caller
// holds m.mu.
func (m *Manager) upsertMirrorLocked(target, backTo string, card Cardinality, inverse bool) error {
	rels, err := m.getRelationsLocked(target)
	if err != nil {
		return err
	}
	if i := relationIndex(rels, backTo); i >= 0 {
		rels[i].Cardinality = card
		rels[i].Inverse = inverse
	} else {
		rels = append(rels, Relation{To: backTo, Cardinality: card, Inverse: inverse})
	}
	return m.saveRelationsLocked(target, rels)
}

// removeMirrorLocked drops the counterpart entry pointing at backTo from target's file. Idempotent:
// a missing entry (or file) is fine. Caller holds m.mu.
func (m *Manager) removeMirrorLocked(target, backTo string) error {
	rels, err := m.getRelationsLocked(target)
	if err != nil {
		return err
	}
	i := relationIndex(rels, backTo)
	if i < 0 {
		return nil
	}
	rels = append(rels[:i], rels[i+1:]...)
	return m.saveRelationsLocked(target, rels)
}

// saveRelationsLocked is the persistence floor: structural validation + atomic write, NO catalog
// checks. Caller holds m.mu. Edge mutations and mirror upkeep go through it so they work even
// against degraded state.
func (m *Manager) saveRelationsLocked(template string, rels []Relation) error {
	seen := make(map[string]bool, len(rels))
	for i, r := range rels {
		if strings.TrimSpace(r.To) == "" {
			return fmt.Errorf("relation: #%d has no target", i+1)
		}
		if !r.Cardinality.valid() {
			return fmt.Errorf("relation: %s has unknown cardinality %q", r.To, r.Cardinality)
		}
		if seen[r.To] {
			return fmt.Errorf("relation: duplicate relation to %s", r.To)
		}
		seen[r.To] = true
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
	if err := m.fs.EnsureDirectory(m.dir); err != nil {
		return err
	}
	return m.fs.SaveFile(m.path(template), buf.String())
}

// AddEdge links two records through the relation from template to `to`. Both records must exist
// right now (hard reject): a brand-new dangling link is never allowed, even though edges can dangle
// later.
func (m *Manager) AddEdge(template, to string, e Edge) error {
	if strings.TrimSpace(e.From) == "" || strings.TrimSpace(e.To) == "" {
		return fmt.Errorf("relation: edge needs both from and to")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rels, err := m.getRelationsLocked(template)
	if err != nil {
		return err
	}
	i := relationIndex(rels, to)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation to %s", template, to)
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
	return m.saveRelationsLocked(template, rels)
}

// RemoveEdge unlinks two records from the relation from template to `to`. Goes through the
// persistence floor so cleanup works even when the target template or records have since gone away
// (the volatile case).
func (m *Manager) RemoveEdge(template, to string, e Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rels, err := m.getRelationsLocked(template)
	if err != nil {
		return err
	}
	i := relationIndex(rels, to)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation to %s", template, to)
	}
	before := len(rels[i].Edges)
	rels[i].Edges = slices.DeleteFunc(rels[i].Edges, func(x Edge) bool { return x == e })
	if len(rels[i].Edges) == before {
		return fmt.Errorf("relation: edge %s -> %s not found", e.From, e.To)
	}
	return m.saveRelationsLocked(template, rels)
}

func relationIndex(rels []Relation, to string) int {
	for i, r := range rels {
		if r.To == to {
			return i
		}
	}
	return -1
}
