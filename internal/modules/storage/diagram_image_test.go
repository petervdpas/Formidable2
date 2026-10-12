package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func saveMapping(t *testing.T, m *Manager, record, target string) {
	t.Helper()
	data := map[string]any{"mapping": []any{}}
	if target != "" {
		data["mapping"] = []any{[]any{"A", "x", target, "y", "", ""}}
	}
	if r := m.SaveForm(context.Background(), diagTpl, record+".meta.json", data); !r.Success {
		t.Fatal(r.Error)
	}
}

func TestSaveForm_CreatesTheDiagramImage(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "T")
	got, _ := m.fs.LoadFile(imgPath(m, img("r1")))
	if !strings.HasPrefix(got, "<svg") || !strings.Contains(got, ">T<") {
		t.Fatalf("save must draw the record's diagram image: %.80q", got)
	}
}

// The record is what matters: if the image cannot be written, the data save
// still succeeds and the record is on disk.
func TestSaveForm_SucceedsEvenWhenTheImageCannotBeWritten(t *testing.T) {
	m := newDiagramStack(t)
	// A plain file where the images folder should be makes every image write fail.
	if err := m.fs.EnsureDirectory(m.templateDir(diagTpl)); err != nil {
		t.Fatal(err)
	}
	if err := m.fs.SaveFile(m.TemplateImageDir(diagTpl), "not a folder"); err != nil {
		t.Fatal(err)
	}
	r := m.SaveForm(context.Background(), diagTpl, "r1.meta.json", map[string]any{"mapping": []any{[]any{"A", "x", "T", "y", "", ""}}})
	if !r.Success {
		t.Fatalf("data save failed because of the image: %s", r.Error)
	}
	if f := m.LoadForm(diagTpl, "r1.meta.json"); f == nil || len(asSlice(f.Data["mapping"])) != 1 {
		t.Fatalf("record not persisted: %+v", f)
	}
	if err := m.SaveDiagramImages(diagTpl, "r1.meta.json"); err == nil {
		t.Fatal("the image path itself must report its failure")
	}
}

func TestSaveDiagramImages_DrawsFromTheRecordOnDisk(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "OnDisk")
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg>stale</svg>")
	if err := m.SaveDiagramImages(diagTpl, "r1.meta.json"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.fs.LoadFile(imgPath(m, img("r1"))); !strings.Contains(got, ">OnDisk<") {
		t.Fatalf("image must reflect the persisted record: %.80q", got)
	}
}

func TestSaveDiagramImages_Errors(t *testing.T) {
	m := newDiagramStack(t)
	if err := m.SaveDiagramImages(diagTpl, "ghost.meta.json"); err == nil {
		t.Error("unknown record must error")
	}
	if err := m.SaveDiagramImages("nope.yaml", "r1.meta.json"); err == nil {
		t.Error("unknown template must error")
	}
}

func TestSaveDiagramImages_TemplateWithoutDiagramIsANoOp(t *testing.T) {
	m, _, tplM, _ := newTestStack(t)
	_ = tplM.SaveTemplate("p.yaml", &template.Template{Name: "P", Filename: "p.yaml", Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "t", Type: "text"}}})
	if r := m.SaveForm(context.Background(), "p.yaml", "r.meta.json", map[string]any{"t": "x"}); !r.Success {
		t.Fatal(r.Error)
	}
	if err := m.SaveDiagramImages("p.yaml", "r.meta.json"); err != nil {
		t.Fatal(err)
	}
	if m.fs.FileExists(m.TemplateImageDir("p.yaml")) {
		if names, _ := m.fs.ListFiles(m.TemplateImageDir("p.yaml")); len(names) != 0 {
			t.Fatalf("images = %v", names)
		}
	}
}

func TestSyncDiagramImages_DrawsEveryRecordAndPrunesOrphans(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "T")
	saveMapping(t, m, "r2", "T")
	_ = m.fs.DeleteFile(imgPath(m, img("r1")))
	_ = m.WriteDiagramImage(diagTpl, img("gone"), "<svg/>")
	if err := m.SyncDiagramImages(diagTpl); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{img("r1"): true, img("r2"): true, img("gone"): false} {
		if got := exists(m, name); got != want {
			t.Errorf("%s exists = %v, want %v", name, got, want)
		}
	}
}

func TestSaveForm_BringsAnExistingDiagramImageUpToDate(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "Old")
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg>stale</svg>")
	saveMapping(t, m, "r1", "New")
	got, _ := m.fs.LoadFile(imgPath(m, img("r1")))
	if !strings.HasPrefix(got, "<svg") || !strings.Contains(got, ">New<") || strings.Contains(got, "stale") {
		t.Fatalf("file not redrawn from the saved data: %.120q", got)
	}
}

func TestSaveForm_UpToDateDiagramImageIsNotRewritten(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "T")
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg>stale</svg>")
	saveMapping(t, m, "r1", "T")
	full := m.fs.ResolvePath(imgPath(m, img("r1")))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	saveMapping(t, m, "r1", "T")
	if st, err := os.Stat(full); err != nil || !st.ModTime().Equal(old) {
		t.Fatalf("an unchanged drawing must not touch the file (err %v)", err)
	}
}

func TestSaveForm_EmptiedDiagramRemovesItsImage(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "T")
	saveMapping(t, m, "r1", "")
	if exists(m, img("r1")) {
		t.Fatal("a diagram with nothing to draw must not leave a stale image")
	}
}

func TestSaveForm_OnlyTouchesTheSavedRecordsImage(t *testing.T) {
	m := newDiagramStack(t)
	saveMapping(t, m, "r1", "T")
	saveMapping(t, m, "r2", "T")
	_ = m.WriteDiagramImage(diagTpl, img("r2"), "<svg>other</svg>")
	_ = m.WriteDiagramImage(diagTpl, img("r1"), "<svg>stale</svg>")
	saveMapping(t, m, "r1", "New")
	if got, _ := m.fs.LoadFile(imgPath(m, img("r2"))); got != "<svg>other</svg>" {
		t.Fatalf("another record's image was changed: %q", got)
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
