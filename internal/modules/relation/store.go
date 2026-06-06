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

// A self-relation's mirror (its inverse half) lives in a self/ subfolder, the role another template's
// file plays for a cross-template relation. This gives self-relations the same two-file redundancy and
// self-heal: lose one of <t>.yaml / self/<t>.yaml, rebuild it from the other.
func (m *Manager) selfDir() string                 { return filepath.Join(m.dir, "self") }
func (m *Manager) selfPath(template string) string { return filepath.Join(m.dir, "self", template) }

// GetRelations returns the relations declared by a template (nil if none). Reads tolerate whatever
// is on disk, including edges gone stale since a record was deleted. The self/ mirror is internal
// redundancy and is NOT merged in: the forward self-relation lives in the template's own file.
func (m *Manager) GetRelations(template string) ([]Relation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getRelationsLocked(template)
}

// getRelationsAtLocked reads + parses any relations file by path; caller holds m.mu.
func (m *Manager) getRelationsAtLocked(p string) ([]Relation, error) {
	if !m.fs.FileExists(p) {
		return nil, nil
	}
	raw, err := m.fs.LoadFile(p)
	if err != nil {
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return nil, fmt.Errorf("relation: parse %s: %w", filepath.Base(p), err)
	}
	return f.Relations, nil
}

func (m *Manager) getRelationsLocked(template string) ([]Relation, error) {
	return m.getRelationsAtLocked(m.path(template))
}

func (m *Manager) getSelfMirrorLocked(template string) ([]Relation, error) {
	return m.getRelationsAtLocked(m.selfPath(template))
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
	// The forward self half is never the inverse side; that flag belongs to the self/ mirror.
	for i := range rels {
		if rels[i].To == template {
			rels[i].Inverse = false
		}
	}
	if err := m.saveRelationsLocked(template, rels); err != nil {
		return err
	}

	// Mirror each cross-template relation onto its target with the flipped cardinality. The self
	// relation (to == template) mirrors into the self/ subfolder instead (handled below).
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

	// Self relation: write (or clear) its inverse mirror in self/<template>.yaml.
	var self *Relation
	for i := range rels {
		if rels[i].To == template {
			self = &rels[i]
			break
		}
	}
	if self != nil {
		mir := []Relation{{
			To:          template,
			Cardinality: self.Cardinality.inverse(),
			Inverse:     true,
			Edges:       reverseEdges(self.Edges),
		}}
		return m.saveSelfMirrorLocked(template, mir)
	}
	return m.clearSelfMirrorLocked(template)
}

// clearSelfMirrorLocked empties the self mirror file when a template no longer has a self relation.
// No fs delete exists, so it writes an empty relations file; tolerant if there was none. Caller holds
// m.mu.
func (m *Manager) clearSelfMirrorLocked(template string) error {
	if !m.fs.FileExists(m.selfPath(template)) {
		return nil
	}
	return m.saveSelfMirrorLocked(template, nil)
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

// saveRelationsAtLocked is the persistence floor: structural validation + atomic write, NO catalog
// checks. Caller holds m.mu. Edge mutations and mirror upkeep go through it so they work even against
// degraded state. The file records `template` regardless of which folder it lands in (dir), so the
// self/ mirror keeps the same template identity as its forward half.
func (m *Manager) saveRelationsAtLocked(p, dir, template string, rels []Relation) error {
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
	if err := m.fs.EnsureDirectory(dir); err != nil {
		return err
	}
	return m.fs.SaveFile(p, buf.String())
}

func (m *Manager) saveRelationsLocked(template string, rels []Relation) error {
	return m.saveRelationsAtLocked(m.path(template), m.dir, template, rels)
}

func (m *Manager) saveSelfMirrorLocked(template string, rels []Relation) error {
	return m.saveRelationsAtLocked(m.selfPath(template), m.selfDir(), template, rels)
}

// AddEdge links a source record to a target record through the relation from source to target, and
// mirrors the reversed edge into the counterpart relation on the target side (edges are stored on
// both sides, just like the declarations). Both records must exist right now (hard reject): a
// brand-new dangling link is never allowed, even though edges can dangle later.
func (m *Manager) AddEdge(source, target string, e Edge) error {
	if strings.TrimSpace(e.From) == "" || strings.TrimSpace(e.To) == "" {
		return fmt.Errorf("relation: edge needs both from and to")
	}
	if source == target && e.From == e.To {
		return fmt.Errorf("relation: a record cannot link to itself")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rels, err := m.getRelationsLocked(source)
	if err != nil {
		return err
	}
	i := relationIndex(rels, target)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation to %s", source, target)
	}
	if !m.cat.RecordExists(source, e.From) {
		return fmt.Errorf("relation: source record %q not found in %s", e.From, source)
	}
	if !m.cat.RecordExists(target, e.To) {
		return fmt.Errorf("relation: target record %q not found in %s", e.To, target)
	}
	if slices.Contains(rels[i].Edges, e) {
		return fmt.Errorf("relation: edge %s -> %s already exists", e.From, e.To)
	}
	if err := checkCardinality(rels[i].Cardinality, rels[i].Edges, e); err != nil {
		return err
	}
	rels[i].Edges = append(rels[i].Edges, e)
	if err := m.saveRelationsLocked(source, rels); err != nil {
		return err
	}
	if source == target {
		// Self-relation: the reversed edge goes to the self/ mirror, not another template's file.
		return m.upsertSelfEdgeMirrorLocked(source, Edge{From: e.To, To: e.From},
			rels[i].Cardinality.inverse())
	}
	return m.upsertEdgeMirrorLocked(target, source, Edge{From: e.To, To: e.From},
		rels[i].Cardinality.inverse(), !rels[i].Inverse)
}

// RemoveEdge unlinks two records and removes the reversed mirror edge from the target side. Goes
// through the persistence floor so cleanup works even when the target template or records have since
// gone away (the volatile case).
func (m *Manager) RemoveEdge(source, target string, e Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rels, err := m.getRelationsLocked(source)
	if err != nil {
		return err
	}
	i := relationIndex(rels, target)
	if i < 0 {
		return fmt.Errorf("relation: %s has no relation to %s", source, target)
	}
	before := len(rels[i].Edges)
	rels[i].Edges = slices.DeleteFunc(rels[i].Edges, func(x Edge) bool { return x == e })
	if len(rels[i].Edges) == before {
		return fmt.Errorf("relation: edge %s -> %s not found", e.From, e.To)
	}
	if err := m.saveRelationsLocked(source, rels); err != nil {
		return err
	}
	if source == target {
		return m.removeSelfEdgeMirrorLocked(source, Edge{From: e.To, To: e.From})
	}
	return m.removeEdgeMirrorLocked(target, source, Edge{From: e.To, To: e.From})
}

// upsertEdgeMirrorLocked adds the reversed edge to the counterpart relation in target's file,
// creating that relation half if it is missing. Idempotent on the edge. Caller holds m.mu.
func (m *Manager) upsertEdgeMirrorLocked(target, backTo string, e Edge, card Cardinality, inverse bool) error {
	rels, err := m.getRelationsLocked(target)
	if err != nil {
		return err
	}
	j := relationIndex(rels, backTo)
	if j < 0 {
		rels = append(rels, Relation{To: backTo, Cardinality: card, Inverse: inverse, Edges: []Edge{e}})
	} else if !slices.Contains(rels[j].Edges, e) {
		rels[j].Edges = append(rels[j].Edges, e)
	}
	return m.saveRelationsLocked(target, rels)
}

// removeEdgeMirrorLocked drops the reversed edge from the counterpart relation in target's file.
// Tolerant: a missing relation or edge is fine. Caller holds m.mu.
func (m *Manager) removeEdgeMirrorLocked(target, backTo string, e Edge) error {
	rels, err := m.getRelationsLocked(target)
	if err != nil {
		return err
	}
	j := relationIndex(rels, backTo)
	if j < 0 {
		return nil
	}
	rels[j].Edges = slices.DeleteFunc(rels[j].Edges, func(x Edge) bool { return x == e })
	return m.saveRelationsLocked(target, rels)
}

// upsertSelfEdgeMirrorLocked adds the reversed edge to the self mirror (self/<template>.yaml),
// creating the mirror's single relation half if it is missing. Idempotent on the edge. Caller holds
// m.mu.
func (m *Manager) upsertSelfEdgeMirrorLocked(template string, e Edge, card Cardinality) error {
	rels, err := m.getSelfMirrorLocked(template)
	if err != nil {
		return err
	}
	j := relationIndex(rels, template)
	if j < 0 {
		rels = append(rels, Relation{To: template, Cardinality: card, Inverse: true, Edges: []Edge{e}})
	} else if !slices.Contains(rels[j].Edges, e) {
		rels[j].Edges = append(rels[j].Edges, e)
	}
	return m.saveSelfMirrorLocked(template, rels)
}

// removeSelfEdgeMirrorLocked drops the reversed edge from the self mirror. Tolerant: a missing mirror
// or edge is fine. Caller holds m.mu.
func (m *Manager) removeSelfEdgeMirrorLocked(template string, e Edge) error {
	rels, err := m.getSelfMirrorLocked(template)
	if err != nil {
		return err
	}
	j := relationIndex(rels, template)
	if j < 0 {
		return nil
	}
	rels[j].Edges = slices.DeleteFunc(rels[j].Edges, func(x Edge) bool { return x == e })
	return m.saveSelfMirrorLocked(template, rels)
}

// checkCardinality rejects an edge that would breach the relation's cardinality: the "one" side
// caps that endpoint at a single link. many-to-many imposes nothing. The mirror needs no separate
// check: the flipped cardinality enforces the same constraint from the other side.
func checkCardinality(c Cardinality, existing []Edge, e Edge) error {
	if c.limitsFrom() {
		for _, x := range existing {
			if x.From == e.From {
				return fmt.Errorf("relation: cardinality %s allows one target per source; %q is already linked", c, e.From)
			}
		}
	}
	if c.limitsTo() {
		for _, x := range existing {
			if x.To == e.To {
				return fmt.Errorf("relation: cardinality %s allows one source per target; %q is already linked", c, e.To)
			}
		}
	}
	return nil
}

func relationIndex(rels []Relation, to string) int {
	for i, r := range rels {
		if r.To == to {
			return i
		}
	}
	return -1
}
