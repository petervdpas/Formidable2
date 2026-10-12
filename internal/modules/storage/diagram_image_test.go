package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

const diagTpl = "map.yaml"

func newDiagramStack(t *testing.T) *Manager {
	t.Helper()
	m, _, tplM, _ := newTestStack(t)
	tpl := &template.Template{Name: "Map", Filename: diagTpl, Fields: []template.Field{
		{Key: "id", Type: "guid"},
		{Key: "mapping", Type: "table", Format: template.TablePatternDataLineage},
		{Key: "lineage", Type: "diagram", Label: "Lineage", Options: []any{
			map[string]any{"value": template.DiagramSource, "label": "mapping"},
		}},
	}}
	if err := tplM.SaveTemplate(diagTpl, tpl); err != nil {
		t.Fatal(err)
	}
	return m
}

// img is the generated diagram file name of a record of the test template.
func img(record string) string {
	return template.DiagramImageName(diagTpl, record+".meta.json", "lineage")
}

func imgPath(m *Manager, name string) string {
	return filepath.Join(m.TemplateImageDir(diagTpl), name)
}

func exists(m *Manager, name string) bool { return m.fs.FileExists(imgPath(m, name)) }

func saveRecord(t *testing.T, m *Manager, record string) {
	t.Helper()
	data := map[string]any{"mapping": []any{[]any{"A", "x", "T", "y", "", ""}}}
	if r := m.SaveForm(context.Background(), diagTpl, record+".meta.json", data); !r.Success {
		t.Fatal(r.Error)
	}
}

func TestWriteDiagramImage_WritesReplacesAndRemoves(t *testing.T) {
	m := newDiagramStack(t)
	if err := m.WriteDiagramImage(diagTpl, img("r1"), "<svg>one</svg>"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.fs.LoadFile(imgPath(m, img("r1"))); got != "<svg>one</svg>" {
		t.Fatalf("file = %q", got)
	}
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg>two</svg>")
	if got, _ := m.fs.LoadFile(imgPath(m, img("r1"))); got != "<svg>two</svg>" {
		t.Fatalf("not replaced: %q", got)
	}
	if err := m.WriteDiagramImage(diagTpl, img("r1"), ""); err != nil || exists(m, img("r1")) {
		t.Fatalf("empty content must remove the file (err %v)", err)
	}
	if err := m.WriteDiagramImage(diagTpl, img("r1"), ""); err != nil {
		t.Fatalf("removing a missing file is a no-op: %v", err)
	}
}

func TestWriteDiagramImage_UnchangedContentIsNotRewritten(t *testing.T) {
	m := newDiagramStack(t)
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg/>")
	full := m.fs.ResolvePath(imgPath(m, img("r1")))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg/>")
	if st, err := os.Stat(full); err != nil || !st.ModTime().Equal(old) {
		t.Fatalf("file was rewritten (err %v)", err)
	}
}

func TestWriteDiagramImage_OnlyAcceptsGeneratedPlainNames(t *testing.T) {
	m := newDiagramStack(t)
	for _, name := range []string{"", "photo.svg", "../" + img("r1"), "sub/" + img("r1")} {
		if err := m.WriteDiagramImage(diagTpl, name, "<svg/>"); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
}

func TestSaveForm_DoesNotDrawDiagrams(t *testing.T) {
	m := newDiagramStack(t)
	saveRecord(t, m, "r1")
	if exists(m, img("r1")) {
		t.Fatal("drawing belongs to the render step, not to saving a record")
	}
}

func TestDeleteForm_RemovesItsDiagramImageOnly(t *testing.T) {
	m := newDiagramStack(t)
	for _, r := range []string{"r1", "r2", "r1.x"} {
		saveRecord(t, m, r)
		_ = m.WriteDiagramImage(diagTpl, img(r), "<svg/>")
	}
	if err := m.DeleteForm(diagTpl, "r1.meta.json"); err != nil {
		t.Fatal(err)
	}
	if exists(m, img("r1")) || !exists(m, img("r2")) || !exists(m, img("r1.x")) {
		t.Fatal("wrong files removed")
	}
}

func TestPruneDiagramImages_DropsFilesWithoutRecordOrField(t *testing.T) {
	m := newDiagramStack(t)
	saveRecord(t, m, "r1")
	gone := img("gone")
	oldField := template.DiagramImageName(diagTpl, "r1.meta.json", "oldfield")
	for _, n := range []string{img("r1"), gone, oldField} {
		_ = m.WriteDiagramImage(diagTpl, n, "<svg/>")
	}
	m.SaveImageFile(diagTpl, "photo.svg", []byte("<svg/>"))
	if err := m.PruneDiagramImages(diagTpl); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{img("r1"): true, gone: false, oldField: false, "photo.svg": true} {
		if got := exists(m, name); got != want {
			t.Errorf("%s exists = %v, want %v", name, got, want)
		}
	}
}

func TestPruneDiagramImages_UnknownTemplateErrors(t *testing.T) {
	if err := newDiagramStack(t).PruneDiagramImages("nope.yaml"); err == nil {
		t.Fatal("want error")
	}
}

func TestListImageFiles_HidesGeneratedDiagrams(t *testing.T) {
	m := newDiagramStack(t)
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg/>")
	m.SaveImageFile(diagTpl, "photo.png", []byte("x"))
	got, err := m.ListImageFiles(diagTpl)
	if err != nil || len(got) != 1 || got[0] != "photo.png" {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestUploads_MayNotUseTheReservedDiagramEnding(t *testing.T) {
	m := newDiagramStack(t)
	if r := m.SaveImageFile(diagTpl, "mine.diagram.svg", []byte("<svg/>")); r.Success {
		t.Fatal("upload with the reserved ending must be refused")
	}
	m.SaveImageFile(diagTpl, "mine.svg", []byte("<svg/>"))
	if err := m.RenameImageFile(diagTpl, "mine.svg", "mine.diagram.svg"); err == nil {
		t.Fatal("rename onto the reserved ending must be refused")
	}
	if r := m.SaveImageFile(diagTpl, "mine.png", []byte("x")); !r.Success {
		t.Fatalf("normal uploads keep working: %s", r.Error)
	}
}
