package template

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

type recordingIndexer struct {
	changed []string
	deleted []string
	saveErr error
	delErr  error
}

func (r *recordingIndexer) OnTemplateChanged(name string) error {
	r.changed = append(r.changed, name)
	return r.saveErr
}
func (r *recordingIndexer) OnTemplateDeleted(name string) error {
	r.deleted = append(r.deleted, name)
	return r.delErr
}

func TestSaveTemplate_FiresIndexer(t *testing.T) {
	dir := t.TempDir()
	sysM := system.NewManager(dir, nil)
	m := NewManager(sysM, "templates", nil)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)

	if err := m.SaveTemplate("basic.yaml", &Template{Name: "Basic", Filename: "basic.yaml"}); err != nil {
		t.Fatal(err)
	}
	if len(idx.changed) != 1 || idx.changed[0] != "basic.yaml" {
		t.Errorf("changed = %v, want [basic.yaml]", idx.changed)
	}
}

func TestDeleteTemplate_FiresIndexer(t *testing.T) {
	dir := t.TempDir()
	sysM := system.NewManager(dir, nil)
	m := NewManager(sysM, "templates", nil)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)

	if err := m.SaveTemplate("basic.yaml", &Template{Name: "Basic", Filename: "basic.yaml"}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteTemplate("basic.yaml"); err != nil {
		t.Fatal(err)
	}
	if len(idx.deleted) != 1 || idx.deleted[0] != "basic.yaml" {
		t.Errorf("deleted = %v, want [basic.yaml]", idx.deleted)
	}
}

func TestSaveTemplate_IndexerErrorIsLoggedNotPropagated(t *testing.T) {
	// The indexer is a derived view — a failure to update it must not
	// fail the user's save. The save returned nil; the hook error is
	// logged at warn level (not asserted here, just observed).
	dir := t.TempDir()
	sysM := system.NewManager(dir, nil)
	m := NewManager(sysM, "templates", nil)
	idx := &recordingIndexer{saveErr: errors.New("indexer down")}
	m.SetIndexer(idx)

	if err := m.SaveTemplate("basic.yaml", &Template{Name: "Basic", Filename: "basic.yaml"}); err != nil {
		t.Errorf("save must not propagate indexer error, got %v", err)
	}
}

func TestNoIndexer_SaveDeleteWork(t *testing.T) {
	// Nil indexer (default) — save/delete must work without it.
	dir := t.TempDir()
	sysM := system.NewManager(dir, nil)
	m := NewManager(sysM, "templates", nil)

	if err := m.SaveTemplate("x.yaml", &Template{Name: "X", Filename: "x.yaml"}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteTemplate("x.yaml"); err != nil {
		t.Fatal(err)
	}
}
