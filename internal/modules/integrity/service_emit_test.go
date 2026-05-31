package integrity

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
)

type recordingEmitter struct {
	names []string
	data  []any
}

func (r *recordingEmitter) Emit(name string, data any) {
	r.names = append(r.names, name)
	r.data = append(r.data, data)
}

// A repair that writes forms must announce storage:changed so the frontend reloads
// the corrected data instead of leaving the user with a stale view.
func TestService_Fix_EmitsStorageChangedOnWrite(t *testing.T) {
	f := cleanForm()
	f.Data["ghost"] = "x"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	fe := &recordingEmitter{}
	res, err := NewService(h.m, fe).Fix(h.tpl.Filename,
		FixPlan{Items: []FixPlanItem{{Kind: IssueExtraField, Strategy: FixStrip}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.FormsSaved != 1 {
		t.Fatalf("FormsSaved = %d, want 1", res.FormsSaved)
	}
	if len(fe.names) != 1 || fe.names[0] != "storage:changed" || fe.data[0] != h.tpl.Filename {
		t.Errorf("emitted %v / %v, want one storage:changed for %s", fe.names, fe.data, h.tpl.Filename)
	}
}

// A repair that saves nothing must not emit, so the view is not needlessly reloaded.
func TestService_Fix_NoEmitWhenNothingSaved(t *testing.T) {
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": cleanForm()})

	fe := &recordingEmitter{}
	res, err := NewService(h.m, fe).Fix(h.tpl.Filename, FixPlan{Items: nil})
	if err != nil {
		t.Fatal(err)
	}
	if res.FormsSaved != 0 {
		t.Fatalf("FormsSaved = %d, want 0", res.FormsSaved)
	}
	if len(fe.names) != 0 {
		t.Errorf("emitted %v, want nothing", fe.names)
	}
}
