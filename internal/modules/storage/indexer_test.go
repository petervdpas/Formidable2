package storage

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

type recordingIndexer struct {
	changed [][2]string
	deleted [][2]string
	saveErr error
	delErr  error
}

func (r *recordingIndexer) OnFormChanged(tpl, file string) error {
	r.changed = append(r.changed, [2]string{tpl, file})
	return r.saveErr
}
func (r *recordingIndexer) OnFormDeleted(tpl, file string) error {
	r.deleted = append(r.deleted, [2]string{tpl, file})
	return r.delErr
}

func newTestStorage(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	sysM := system.NewManager(dir, nil)
	sfrM := sfr.NewManager(sysM, nil)

	// Pre-seed a template so storage's sanitize step has fields to read.
	tplM := template.NewManager(sysM, "templates", nil)
	must(t, tplM.SaveTemplate("basic.yaml", &template.Template{
		Name: "Basic", Filename: "basic.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	}))
	return NewManager(sysM, sfrM, tplM, "storage", nil)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSaveForm_FiresIndexer(t *testing.T) {
	m := newTestStorage(t)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)

	res := m.SaveForm("basic.yaml", "x.meta.json", map[string]any{"title": "X"})
	if !res.Success {
		t.Fatalf("save: %s", res.Error)
	}
	if len(idx.changed) != 1 || idx.changed[0] != [2]string{"basic.yaml", "x.meta.json"} {
		t.Errorf("changed = %v, want one [basic.yaml x.meta.json]", idx.changed)
	}
}

func TestDeleteForm_FiresIndexer(t *testing.T) {
	m := newTestStorage(t)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)

	res := m.SaveForm("basic.yaml", "x.meta.json", map[string]any{"title": "X"})
	if !res.Success {
		t.Fatal(res.Error)
	}
	if err := m.DeleteForm("basic.yaml", "x.meta.json"); err != nil {
		t.Fatal(err)
	}
	if len(idx.deleted) != 1 || idx.deleted[0] != [2]string{"basic.yaml", "x.meta.json"} {
		t.Errorf("deleted = %v, want one [basic.yaml x.meta.json]", idx.deleted)
	}
}

func TestSaveForm_IndexerErrorIsLoggedNotPropagated(t *testing.T) {
	m := newTestStorage(t)
	m.SetIndexer(&recordingIndexer{saveErr: errors.New("down")})

	res := m.SaveForm("basic.yaml", "x.meta.json", map[string]any{"title": "X"})
	if !res.Success {
		t.Errorf("indexer failure must not break save: %v", res)
	}
}

func TestSaveFormFailure_DoesNotFireIndexer(t *testing.T) {
	// A path-traversal attempt fails sanitization → SaveResult.Success=false.
	// The indexer should NOT see a hook for a failed save.
	m := newTestStorage(t)
	idx := &recordingIndexer{}
	m.SetIndexer(idx)

	res := m.SaveForm("basic.yaml", "../escape.meta.json", map[string]any{})
	if res.Success {
		t.Fatal("expected save to refuse traversal")
	}
	if len(idx.changed) != 0 {
		t.Errorf("indexer fired on failed save: %v", idx.changed)
	}
}
