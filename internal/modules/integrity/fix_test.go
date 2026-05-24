package integrity

import (
	"context"
	"sort"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// stubWriter records SaveForm calls and overlays the saved form back
// onto the stub storage so the re-analyze pass at the end of FixTemplate
// sees the post-write state.
type stubWriter struct {
	store *stubStorage
	calls int
}

func newStubWriter(store *stubStorage) *stubWriter { return &stubWriter{store: store} }

func (s *stubWriter) SaveForm(_ context.Context, tpl, fn string, form *storage.Form) error {
	s.calls++
	if s.store.forms[tpl] == nil {
		s.store.forms[tpl] = map[string]*storage.Form{}
	}
	// Deep-ish copy: data values are leaked references, but tests
	// don't mutate after save - only inspect.
	copy := *form
	s.store.forms[tpl][fn] = &copy
	return nil
}

// fixHarness wires a Manager with a writer that mirrors writes back
// into the stub storage so the re-analyze inside FixTemplate sees the
// post-write shape.
type fixHarness struct {
	t     *testing.T
	tpl   *template.Template
	store *stubStorage
	wr    *stubWriter
	m     *Manager
}

func newFixHarness(t *testing.T, tpl *template.Template, forms map[string]*storage.Form) *fixHarness {
	t.Helper()
	st := &stubTemplates{ts: map[string]*template.Template{tpl.Filename: tpl}}
	store := &stubStorage{forms: map[string]map[string]*storage.Form{tpl.Filename: forms}}
	wr := newStubWriter(store)
	m := NewManager(st, store)
	m.SetWriter(wr)
	return &fixHarness{t: t, tpl: tpl, store: store, wr: wr, m: m}
}

func (h *fixHarness) runPlan(items ...FixPlanItem) FixResult {
	h.t.Helper()
	res, err := h.m.FixTemplate(h.tpl.Filename, FixPlan{Items: items})
	if err != nil {
		h.t.Fatalf("FixTemplate: %v", err)
	}
	return res
}

func (h *fixHarness) loadSaved(fn string) *storage.Form {
	h.t.Helper()
	f := h.store.forms[h.tpl.Filename][fn]
	if f == nil {
		h.t.Fatalf("no saved form for %s", fn)
	}
	return f
}

// ─── per-strategy unit tests ───────────────────────────────────────────

func TestFix_Strip_RemovesExtraField(t *testing.T) {
	f := cleanForm()
	f.Data["ghost"] = "x"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueExtraField, Strategy: FixStrip})

	if res.Applied != 1 || res.FormsSaved != 1 {
		t.Fatalf("Applied=%d FormsSaved=%d; want 1/1: %+v", res.Applied, res.FormsSaved, res)
	}
	if _, present := h.loadSaved("a.meta.json").Data["ghost"]; present {
		t.Errorf("ghost key not stripped from saved data")
	}
}

func TestFix_FillDefault_AddsMissingField(t *testing.T) {
	f := cleanForm()
	delete(f.Data, "count")
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMissingField, Strategy: FixFillDefault})

	if res.Applied != 1 || res.FormsSaved != 1 {
		t.Fatalf("Applied=%d FormsSaved=%d; want 1/1: %+v", res.Applied, res.FormsSaved, res)
	}
	got, ok := h.loadSaved("a.meta.json").Data["count"]
	if !ok || got == nil {
		t.Errorf("count default not populated; got %v (ok=%v)", got, ok)
	}
}

func TestFix_Coerce_StringToBool(t *testing.T) {
	for _, want := range []struct {
		in  string
		out bool
	}{{"true", true}, {"false", false}, {"yes", true}, {"no", false}} {
		t.Run(want.in, func(t *testing.T) {
			f := cleanForm()
			f.Data["active"] = want.in
			h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

			res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

			if res.Applied != 1 {
				t.Fatalf("Applied=%d for %q; want 1: %+v", res.Applied, want.in, res)
			}
			got := h.loadSaved("a.meta.json").Data["active"]
			if b, ok := got.(bool); !ok || b != want.out {
				t.Errorf("active=%v (%T); want bool %v", got, got, want.out)
			}
		})
	}
}

func TestFix_Coerce_StringToNumber(t *testing.T) {
	f := cleanForm()
	f.Data["count"] = "42"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["count"]
	if f, ok := got.(float64); !ok || f != 42 {
		t.Errorf("count=%v (%T); want float64 42", got, got)
	}
}

func TestFix_Coerce_StringToTagsArray(t *testing.T) {
	f := cleanForm()
	f.Data["tags"] = "alpha, beta, gamma"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["tags"]
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("tags=%v (%T); want []any", got, got)
	}
	if len(arr) != 3 {
		t.Errorf("tags length=%d; want 3 (split by comma)", len(arr))
	}
}

func TestFix_Coerce_FailedConversionIsSkipped(t *testing.T) {
	f := cleanForm()
	f.Data["count"] = "not-a-number"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

	if res.Applied != 0 || res.Skipped != 1 {
		t.Fatalf("Applied=%d Skipped=%d; want 0/1 on unparseable: %+v",
			res.Applied, res.Skipped, res)
	}
	if res.FormsSaved != 0 {
		t.Errorf("FormsSaved=%d; want 0 (no successful applies = no write)", res.FormsSaved)
	}
}

func TestFix_Clear_ResetsToDefault(t *testing.T) {
	f := cleanForm()
	f.Data["count"] = "garbage"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixClear})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["count"]
	if got == nil {
		t.Errorf("count not reset")
	}
}

func TestFix_Coerce_BadDateFormat_DDMMYYYY(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = "21/07/2025"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["due"]
	if got != "2025-07-21" {
		t.Errorf("due=%v; want ISO 2025-07-21", got)
	}
}

func TestFix_Coerce_BadDateFormat_DashSeparator(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = "21-07-2025"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixCoerce})

	got := h.loadSaved("a.meta.json").Data["due"]
	if got != "2025-07-21" {
		t.Errorf("due=%v; want ISO 2025-07-21 (applied=%d)", got, res.Applied)
	}
}

func TestFix_Clear_BadDate(t *testing.T) {
	f := cleanForm()
	f.Data["due"] = "wibble"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixClear})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["due"]
	if got != "" {
		t.Errorf("due=%v; want empty string", got)
	}
}

func TestFix_Coerce_BadDateInTableCell(t *testing.T) {
	f := tableForm() // row 0 col 1 = "10-11-2025" (DD-MM-YYYY)
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	rows := h.loadSaved("a.meta.json").Data["history"].([]any)
	got := rows[0].([]any)[1]
	if got != "2025-11-10" {
		t.Errorf("history[0][1]=%v; want ISO 2025-11-10", got)
	}
	// The string column (date-looking text) is left verbatim.
	if label := rows[0].([]any)[0]; label != "10-11-2025" {
		t.Errorf("string column mutated: %v", label)
	}
}

func TestFix_Clear_BadDateInTableCell(t *testing.T) {
	f := tableForm()
	f.Data["history"].([]any)[0].([]any)[1] = "wibble"
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixClear})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["history"].([]any)[0].([]any)[1]
	if got != "" {
		t.Errorf("history[0][1]=%v; want empty string", got)
	}
}

func TestFix_Coerce_BadNumberInTableCell(t *testing.T) {
	f := tableForm()
	f.Data["history"].([]any)[0].([]any)[1] = "2025-11-10" // keep the date valid
	f.Data["history"].([]any)[0].([]any)[2] = "12"          // number col holds a string
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["history"].([]any)[0].([]any)[2]
	if got != float64(12) {
		t.Errorf("history[0][2]=%v (%T); want float64(12)", got, got)
	}
}

func TestFix_Coerce_BadBoolInTableCell(t *testing.T) {
	f := tableForm()
	f.Data["history"].([]any)[0].([]any)[1] = "2025-11-10"
	f.Data["history"].([]any)[0].([]any)[3] = "true" // bool col holds a string
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixCoerce})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["history"].([]any)[0].([]any)[3]
	if got != true {
		t.Errorf("history[0][3]=%v (%T); want true", got, got)
	}
}

func TestFix_Clear_BadNumberInTableCell(t *testing.T) {
	f := tableForm()
	f.Data["history"].([]any)[0].([]any)[1] = "2025-11-10"
	f.Data["history"].([]any)[0].([]any)[2] = "nope"
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueTypeMismatch, Strategy: FixClear})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["history"].([]any)[0].([]any)[2]
	if got != float64(0) {
		t.Errorf("history[0][2]=%v (%T); want float64(0)", got, got)
	}
}

func TestFix_MintUUID_FillsMetaId(t *testing.T) {
	tpl := tplWithGuid()
	f := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{"title": "x", "id": "fixture-id"},
	}
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMetaMissing, Strategy: FixMintUUID})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	if h.loadSaved("a.meta.json").Meta.ID == "" {
		t.Errorf("meta.id still empty after MintUUID")
	}
}

func TestFix_Restamp_FixesBadCreated(t *testing.T) {
	f := cleanForm()
	f.Meta.Created.At = "yesterday"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMetaBadFormat, Strategy: FixRestamp})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1 (created restamp): %+v", res.Applied, res)
	}
	if h.loadSaved("a.meta.json").Meta.Created.At == "yesterday" {
		t.Errorf("meta.created still bad after restamp")
	}
}

func TestFix_Restamp_FixesBadUpdated(t *testing.T) {
	f := cleanForm()
	f.Meta.Updated.At = "tomorrow"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMetaBadFormat, Strategy: FixRestamp})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1 (updated restamp): %+v", res.Applied, res)
	}
	if h.loadSaved("a.meta.json").Meta.Updated.At == "tomorrow" {
		t.Errorf("meta.updated still bad after restamp")
	}
}

func TestFix_Restamp_ClearsUnknownFacetSelected(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"flag": {Set: true, Selected: "GHOST"},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	h := newFixHarness(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMetaBadFormat, Strategy: FixRestamp})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Meta.Facets["flag"]
	if got.Selected != "" {
		t.Errorf("selected not cleared, got %q", got.Selected)
	}
	if !got.Set {
		t.Errorf("set flag should survive restamp; got %+v", got)
	}
}

func TestFix_Restamp_DropsUnknownFacetKey(t *testing.T) {
	f := &storage.Form{
		Meta: storage.FormMeta{
			Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"},
			Facets: map[string]storage.FacetState{
				"phantom": {Set: true},
			},
		},
		Data: map[string]any{"title": "x"},
	}
	h := newFixHarness(t, tplWithFlag(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueMetaBadFormat, Strategy: FixRestamp})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	if _, present := h.loadSaved("a.meta.json").Meta.Facets["phantom"]; present {
		t.Errorf("phantom facet should be dropped after restamp")
	}
}

func TestFix_Skip_LeavesEverythingAlone(t *testing.T) {
	f := cleanForm()
	f.Data["ghost"] = "x"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueExtraField, Strategy: FixSkip})

	if res.Applied != 0 || res.FormsSaved != 0 {
		t.Errorf("Skip wrote anything: %+v", res)
	}
}

func TestFix_KindWithoutPlanItemIsSkipped(t *testing.T) {
	// Default behaviour when the plan has no item for a kind == Skip.
	// The UI's "every checkbox unchecked" maps to "empty plan".
	f := cleanForm()
	f.Data["ghost"] = "x"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan() // empty plan

	if res.Applied != 0 || res.FormsSaved != 0 {
		t.Errorf("empty plan changed something: %+v", res)
	}
}

func TestFix_UnknownStrategy_Errors(t *testing.T) {
	f := cleanForm()
	f.Data["ghost"] = "x"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	plan := FixPlan{Items: []FixPlanItem{{Kind: IssueExtraField, Strategy: FixStrategy("bogus")}}}
	_, err := h.m.FixTemplate(h.tpl.Filename, plan)
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
}

func TestFix_NoWriterConfigured_Errors(t *testing.T) {
	tpl := tplBasic()
	st := &stubTemplates{ts: map[string]*template.Template{tpl.Filename: tpl}}
	so := &stubStorage{forms: map[string]map[string]*storage.Form{
		tpl.Filename: {"a.meta.json": cleanForm()},
	}}
	m := NewManager(st, so) // no SetWriter

	_, err := m.FixTemplate(tpl.Filename, FixPlan{
		Items: []FixPlanItem{{Kind: IssueExtraField, Strategy: FixStrip}},
	})
	if err == nil {
		t.Fatal("expected error when writer is unconfigured")
	}
}

func TestFix_RecountsAfterRepair(t *testing.T) {
	f := cleanForm()
	f.Data["ghost1"] = "x"
	f.Data["ghost2"] = "y"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueExtraField, Strategy: FixStrip})

	if res.Applied != 2 {
		t.Fatalf("Applied=%d; want 2 (both ghosts in one form): %+v", res.Applied, res)
	}
	if res.ScannedAfter != 0 {
		t.Errorf("ScannedAfter=%d; want 0 (everything fixed): %+v", res.ScannedAfter, res)
	}
}

func TestFix_OutcomesSortedByFilename(t *testing.T) {
	a := cleanForm()
	a.Data["g"] = "1"
	b := cleanForm()
	b.Data["g"] = "2"
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{
		"b.meta.json": b,
		"a.meta.json": a,
	})

	res := h.runPlan(FixPlanItem{Kind: IssueExtraField, Strategy: FixStrip})

	if len(res.Outcomes) != 2 {
		t.Fatalf("Outcomes len=%d; want 2", len(res.Outcomes))
	}
	names := []string{res.Outcomes[0].Filename, res.Outcomes[1].Filename}
	if !sort.StringsAreSorted(names) {
		t.Errorf("outcomes not sorted: %v", names)
	}
}

func TestFix_Unreadable_AlwaysSkipped(t *testing.T) {
	// Even with Strip selected, an unreadable form contributes a
	// single skipped issue - the file can't be loaded so there's
	// nothing to mutate. Phase-2 in-app fix is impossible.
	h := newFixHarness(t, tplBasic(), map[string]*storage.Form{"broken.meta.json": nil})

	res := h.runPlan(FixPlanItem{Kind: IssueUnreadable, Strategy: FixSkip})

	if res.FormsSaved != 0 {
		t.Errorf("FormsSaved=%d on unreadable; want 0", res.FormsSaved)
	}
}

func TestFix_LoopExtraField_StrippedInsideLoopItem(t *testing.T) {
	tpl := tplWithLoop()
	form := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{
			"title": "hi",
			"items": []any{
				map[string]any{"name": "a", "qty": float64(1), "ghost": "x"},
			},
		},
	}
	h := newFixHarness(t, tpl, map[string]*storage.Form{"a.meta.json": form})

	res := h.runPlan(FixPlanItem{Kind: IssueExtraField, Strategy: FixStrip})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	items := h.loadSaved("a.meta.json").Data["items"].([]any)
	item0 := items[0].(map[string]any)
	if _, present := item0["ghost"]; present {
		t.Errorf("ghost still present inside loop item: %v", item0)
	}
}
