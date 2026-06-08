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

func TestFix_Coerce_TableDateUsesInferredFormat(t *testing.T) {
	// 25-12-2025 pins the column to DD-MM, so the ambiguous 10-11-2025
	// resolves to 2025-11-10 (Nov 10), NOT 2025-10-11. This is the whole
	// point of inference over blind per-value guessing.
	f := dateTableForm("10-11-2025", "25-12-2025")
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixCoerce})

	if res.Applied != 2 {
		t.Fatalf("Applied=%d; want 2: %+v", res.Applied, res)
	}
	rows := h.loadSaved("a.meta.json").Data["history"].([]any)
	if got := rows[0].([]any)[1]; got != "2025-11-10" {
		t.Errorf("history[0][1]=%v; want 2025-11-10 (DD-MM inferred)", got)
	}
	if got := rows[1].([]any)[1]; got != "2025-12-25" {
		t.Errorf("history[1][1]=%v; want 2025-12-25", got)
	}
}

func TestFix_DateAnomaly_CoerceIsSkipped(t *testing.T) {
	// DD-MM column with a slash outlier: the outlier is an anomaly and
	// must NOT be coerced (left for a manual fix).
	f := dateTableForm("25-12-2025", "11/06/2025")
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	h.runPlan(
		FixPlanItem{Kind: IssueBadDateFormat, Strategy: FixCoerce},
		FixPlanItem{Kind: IssueDateAnomaly, Strategy: FixCoerce},
	)

	rows := h.loadSaved("a.meta.json").Data["history"].([]any)
	if got := rows[0].([]any)[1]; got != "2025-12-25" {
		t.Errorf("conformant cell history[0][1]=%v; want 2025-12-25", got)
	}
	if got := rows[1].([]any)[1]; got != "11/06/2025" {
		t.Errorf("anomaly history[1][1]=%v; want untouched 11/06/2025", got)
	}
}

func TestFix_DateAnomaly_ClearEmpties(t *testing.T) {
	f := dateTableForm("25-12-2025", "11/06/2025")
	h := newFixHarness(t, tplWithTable(), map[string]*storage.Form{"a.meta.json": f})

	res := h.runPlan(FixPlanItem{Kind: IssueDateAnomaly, Strategy: FixClear})

	if res.Applied != 1 {
		t.Fatalf("Applied=%d; want 1: %+v", res.Applied, res)
	}
	got := h.loadSaved("a.meta.json").Data["history"].([]any)[1].([]any)[1]
	if got != "" {
		t.Errorf("history[1][1]=%v; want empty string", got)
	}
}

func TestFix_Coerce_BadNumberInTableCell(t *testing.T) {
	f := tableForm()
	f.Data["history"].([]any)[0].([]any)[1] = "2025-11-10" // keep the date valid
	f.Data["history"].([]any)[0].([]any)[2] = "12"         // number col holds a string
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

// applyStrategy carries a guard per strategy that refuses to act when the
// issue kind does not match. FixTemplate never feeds a mismatched pair (its
// plan keys strategies by kind), so these defensive branches are only
// reachable by calling applyStrategy directly.
func TestApplyStrategy_StrategyKindMismatchGuards(t *testing.T) {
	tpl := tplBasic()
	cases := []struct {
		name  string
		iss   Issue
		strat FixStrategy
	}{
		{"strip wrong kind", Issue{Kind: IssueTypeMismatch, Path: "title"}, FixStrip},
		{"fill_default wrong kind", Issue{Kind: IssueExtraField, Path: "title"}, FixFillDefault},
		{"mint_uuid wrong kind", Issue{Kind: IssueExtraField, Path: "title"}, FixMintUUID},
		{"mint_uuid wrong path", Issue{Kind: IssueMetaMissing, Path: "meta.created"}, FixMintUUID},
		{"sync_guid wrong kind", Issue{Kind: IssueExtraField, Path: "id"}, FixSyncGuid},
		{"restamp wrong kind", Issue{Kind: IssueExtraField, Path: "meta.created"}, FixRestamp},
		{"restamp unsupported meta path", Issue{Kind: IssueMetaBadFormat, Path: "meta.bogus"}, FixRestamp},
		{"seed_facet wrong kind", Issue{Kind: IssueExtraField, Path: "meta.facets.flag"}, FixSeedFacet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draft := cloneForm(cleanForm())
			applied, note, err := applyStrategy(tpl, draft, tc.iss, tc.strat)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if applied {
				t.Fatalf("expected guard to refuse (applied=false), got applied=true")
			}
			if note == "" {
				t.Fatalf("expected an explanatory note on the guarded skip")
			}
		})
	}
}

func TestApplyStrategy_UnhandledStrategyErrors(t *testing.T) {
	draft := cloneForm(cleanForm())
	_, _, err := applyStrategy(tplBasic(), draft, Issue{Kind: IssueExtraField, Path: "title"}, FixStrategy("bogus"))
	if err == nil {
		t.Fatal("expected error for unhandled strategy")
	}
}

func TestApplyStrategy_SyncGuid_BothEmptyIsSkipped(t *testing.T) {
	draft := &storage.Form{
		Meta: storage.FormMeta{Created: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}, Updated: storage.AuditEntry{At: "2026-05-11T09:00:00Z"}},
		Data: map[string]any{"id": ""},
	}
	applied, note, err := applyStrategy(tplWithGuid(), draft, Issue{Kind: IssueGuidUnsynced, Path: "id"}, FixSyncGuid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied || note == "" {
		t.Fatalf("both-empty guid sync must skip with a note; applied=%v note=%q", applied, note)
	}
}

func TestApplyStrategy_FillDefault_NoTemplateFieldIsSkipped(t *testing.T) {
	draft := cloneForm(cleanForm())
	applied, note, err := applyStrategy(tplBasic(), draft, Issue{Kind: IssueMissingField, Path: "ghostfield"}, FixFillDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied || note == "" {
		t.Fatalf("missing template field must skip with a note; applied=%v note=%q", applied, note)
	}
}

func TestDefaultForFieldType_PerType(t *testing.T) {
	cases := []struct {
		typ  string
		want any
	}{
		{"boolean", false},
		{"number", float64(0)},
		{"range", float64(50)},
		{"multioption", []any{}},
		{"list", []any{}},
		{"table", []any{}},
		{"tags", []any{}},
		{"api", nil},
		{"text", ""},
		{"weird-unknown", ""},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			got := defaultForFieldType(tc.typ)
			if arr, ok := tc.want.([]any); ok {
				gotArr, ok := got.([]any)
				if !ok || len(gotArr) != len(arr) {
					t.Fatalf("%s default = %v (%T); want empty []any", tc.typ, got, got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("%s default = %v (%T); want %v", tc.typ, got, got, tc.want)
			}
		})
	}
}

func TestCoerceForFieldType_EdgeCases(t *testing.T) {
	// link from a bare string becomes the canonical map.
	if got, ok := coerceForFieldType("link", "https://x"); !ok {
		t.Fatalf("link string coerce failed")
	} else if m, _ := got.(map[string]any); m["href"] != "https://x" || m["text"] != "" {
		t.Fatalf("link string coerce = %v; want {href:..,text:\"\"}", got)
	}
	// link map passes through.
	in := map[string]any{"href": "a", "text": "b"}
	if got, ok := coerceForFieldType("link", in); !ok || got.(map[string]any)["text"] != "b" {
		t.Fatalf("link map passthrough failed: %v ok=%v", got, ok)
	}
	// number from int.
	if got, ok := coerceForFieldType("number", 7); !ok || got != float64(7) {
		t.Fatalf("number from int = %v ok=%v; want float64(7)", got, ok)
	}
	// boolean string variants.
	for _, s := range []string{"yes", "1", "on", "TRUE"} {
		if got, ok := coerceForFieldType("boolean", s); !ok || got != true {
			t.Fatalf("boolean %q = %v ok=%v; want true", s, got, ok)
		}
	}
	for _, s := range []string{"no", "0", "off"} {
		if got, ok := coerceForFieldType("boolean", s); !ok || got != false {
			t.Fatalf("boolean %q = %v ok=%v; want false", s, got, ok)
		}
	}
	// boolean from a non-bool/non-string is unsafe.
	if _, ok := coerceForFieldType("boolean", 1); ok {
		t.Fatalf("boolean from int must be unsafe")
	}
	// date that parses no layout is unsafe; non-string date is unsafe.
	if _, ok := coerceForFieldType("date", "not-a-date"); ok {
		t.Fatalf("unparseable date must be unsafe")
	}
	if _, ok := coerceForFieldType("date", 42); ok {
		t.Fatalf("non-string date must be unsafe")
	}
	// list from delimited string drops blanks and splits on , and ;.
	if got, ok := coerceForFieldType("list", "a, ;b ;; c"); !ok {
		t.Fatalf("list coerce failed")
	} else if arr := got.([]any); len(arr) != 3 {
		t.Fatalf("list coerce = %v; want 3 items", arr)
	}
	// list from a wrong scalar type is unsafe.
	if _, ok := coerceForFieldType("tags", 5); ok {
		t.Fatalf("tags from int must be unsafe")
	}
	// structurally rich types never auto-coerce.
	if _, ok := coerceForFieldType("table", "[1,2]"); ok {
		t.Fatalf("table must not auto-coerce")
	}
	if _, ok := coerceForFieldType("api", "x"); ok {
		t.Fatalf("api must not auto-coerce")
	}
}

func TestFix_DuplicateGuid_RemintsDataAndMirrorsMeta(t *testing.T) {
	tpl := &template.Template{
		Name: "ent", Filename: "ent.yaml",
		Fields: []template.Field{{Key: "id", Type: "guid"}, {Key: "title", Type: "text"}},
	}
	forms := map[string]*storage.Form{
		"a.meta.json": {Meta: storage.FormMeta{ID: "shared"}, Data: map[string]any{"id": "shared", "title": "A"}},
		"b.meta.json": {Meta: storage.FormMeta{ID: "shared"}, Data: map[string]any{"id": "shared", "title": "B"}},
	}
	h := newFixHarness(t, tpl, forms)
	res := h.runPlan(FixPlanItem{Kind: IssueDuplicateGuid, Strategy: FixMintUUID})
	if res.Applied != 1 {
		t.Fatalf("Applied=%d, want 1: %+v", res.Applied, res)
	}
	// The duplicate (b) gets a fresh guid in the DATA field, with meta.id mirroring it.
	b := h.loadSaved("b.meta.json")
	bid, _ := b.Data["id"].(string)
	if bid == "shared" || bid == "" {
		t.Fatalf("duplicate data guid not re-minted: %q", bid)
	}
	if b.Meta.ID != bid {
		t.Fatalf("meta.id must mirror the data guid field: meta=%q data=%q", b.Meta.ID, bid)
	}
	// The canonical (a) is left untouched.
	a := h.loadSaved("a.meta.json")
	if a.Data["id"] != "shared" || a.Meta.ID != "shared" {
		t.Fatalf("canonical a.meta.json should be untouched: data=%v meta=%q", a.Data["id"], a.Meta.ID)
	}
}
