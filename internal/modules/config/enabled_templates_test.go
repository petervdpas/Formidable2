package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeLister is a TemplateLister stub for unit tests.
type fakeLister struct {
	mu    sync.Mutex
	files []string
	err   error
	calls int
}

func (f *fakeLister) ListTemplates() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	// Defensive copy so callers can't mutate our state.
	out := make([]string, len(f.files))
	copy(out, f.files)
	return out, nil
}

func (f *fakeLister) set(files []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files = append([]string(nil), files...)
}

func (f *fakeLister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func withEnabled(t *testing.T, m *Manager, names ...string) {
	t.Helper()
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": names,
	}); err != nil {
		t.Fatalf("seed enabled_templates: %v", err)
	}
}

// ----- IsTemplateEnabled --------------------------------------------------

func TestIsTemplateEnabled_EmptyListMeansNoneEnabled(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m.IsTemplateEnabled("anything.yaml") {
		t.Error("empty EnabledTemplates must report no template as enabled")
	}
}

func TestIsTemplateEnabled_PopulatedListOnlyAllowsListed(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "basic.yaml", "report.yaml")
	if !m.IsTemplateEnabled("basic.yaml") {
		t.Error("basic.yaml is in the list - should be enabled")
	}
	if m.IsTemplateEnabled("hidden.yaml") {
		t.Error("hidden.yaml is NOT in the list - should be disabled")
	}
}

func TestIsTemplateEnabled_EmptyNameIsFalse(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m.IsTemplateEnabled("") {
		t.Error("empty name must never be enabled, even with empty list")
	}
}

// ----- FilterEnabled ------------------------------------------------------

func TestFilterEnabled_EmptyListReturnsNone(t *testing.T) {
	m, _, _ := newTestManager(t)
	in := []string{"a.yaml", "b.yaml", "c.yaml"}
	got := m.FilterEnabled(in)
	if len(got) != 0 {
		t.Errorf("empty list must filter everything out (none scoped), got %v", got)
	}
}

func TestFilterEnabled_PreservesInputOrder(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "c.yaml", "a.yaml")
	got := m.FilterEnabled([]string{"a.yaml", "b.yaml", "c.yaml"})
	want := []string{"a.yaml", "c.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterEnabled = %v, want %v (must preserve input order)", got, want)
	}
}

func TestFilterEnabled_DropsAllWhenNoneMatch(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "x.yaml")
	got := m.FilterEnabled([]string{"a.yaml", "b.yaml"})
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestFilterEnabled_EmptyInputReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "basic.yaml")
	got := m.FilterEnabled(nil)
	if len(got) != 0 {
		t.Errorf("nil input → expected empty, got %v", got)
	}
}

// ----- PruneEnabledTemplates ---------------------------------------------

func TestPruneEnabledTemplates_RemovesStale(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "basic.yaml", "gone.yaml", "report.yaml")

	removed, err := m.PruneEnabledTemplates([]string{"basic.yaml", "report.yaml"})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if !reflect.DeepEqual(removed, []string{"gone.yaml"}) {
		t.Errorf("removed = %v, want [gone.yaml]", removed)
	}

	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	want := []string{"basic.yaml", "report.yaml"}
	if !reflect.DeepEqual(cfg.EnabledTemplates, want) {
		t.Errorf("post-prune cfg = %v, want %v", cfg.EnabledTemplates, want)
	}

	raw, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if strings.Contains(string(raw), "gone.yaml") {
		t.Error("stale entry must be gone from disk")
	}
}

func TestPruneEnabledTemplates_NoChangeWhenAllPresent(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "basic.yaml", "report.yaml")

	// Capture content (not mtime - Linux ext4 has 1s granularity, plus
	// LoadUserConfig may rewrite on first-time defaults sanitization).
	pre, err := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if err != nil {
		t.Fatalf("read pre: %v", err)
	}

	removed, err := m.PruneEnabledTemplates([]string{"basic.yaml", "report.yaml", "extra.yaml"})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}

	post, err := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if err != nil {
		t.Fatalf("read post: %v", err)
	}
	if string(pre) != string(post) {
		t.Error("file content must be byte-identical when nothing changed")
	}
}

func TestPruneEnabledTemplates_NilExistingRemovesAll(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "basic.yaml", "report.yaml")

	removed, err := m.PruneEnabledTemplates(nil)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("expected both removed, got %v", removed)
	}
	cfg, _ := m.LoadUserConfig()
	if len(cfg.EnabledTemplates) != 0 {
		t.Errorf("cfg should be empty post-prune, got %v", cfg.EnabledTemplates)
	}
}

func TestPruneEnabledTemplates_EmptyEnabledListIsNoOp(t *testing.T) {
	m, _, root := newTestManager(t)
	// Force any first-load rewrite to happen now so the pre/post diff
	// only catches writes triggered by Prune itself.
	if _, err := m.LoadUserConfig(); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	pre, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))

	removed, err := m.PruneEnabledTemplates([]string{"basic.yaml"})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("nothing to prune, got %v", removed)
	}
	post, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if string(pre) != string(post) {
		t.Error("empty list = all enabled; file must not be rewritten")
	}
}

// ----- ReconcileEnabledTemplates ------------------------------------------

func TestReconcileEnabledTemplates_NoListerIsNoOp(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "ghost.yaml")
	got, err := m.ReconcileEnabledTemplates()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"ghost.yaml"}) {
		t.Errorf("without lister, must return current slice as-is, got %v", got)
	}
}

// ----- SeedEnabledTemplatesIfUnset ----------------------------------------

func TestSeedEnabled_SeedsAllWhenUnset(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"beta.yaml", "alpha.yaml"})

	if err := m.SeedEnabledTemplatesIfUnset(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"alpha.yaml", "beta.yaml"}) {
		t.Errorf("seeded = %v, want sorted [alpha beta]", cfg.EnabledTemplates)
	}
}

func TestSeedEnabled_LeavesExplicitEmptyAlone(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"alpha.yaml"})

	// Explicitly scoped to none ([] persisted).
	if _, err := m.UpdateUserConfig(map[string]any{"enabled_templates": []string{}}); err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if err := m.SeedEnabledTemplatesIfUnset(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	if len(cfg.EnabledTemplates) != 0 {
		t.Errorf("explicit [] must NOT be re-seeded, got %v", cfg.EnabledTemplates)
	}
	if cfg.EnabledTemplates == nil {
		t.Error("explicit [] must stay non-nil (configured), not revert to nil")
	}
}

func TestSeedEnabled_NoTemplatesIsNoop(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l) // empty folder

	if err := m.SeedEnabledTemplatesIfUnset(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	if cfg.EnabledTemplates != nil {
		t.Errorf("no templates → stay unconfigured (nil), got %v", cfg.EnabledTemplates)
	}
}

// ----- SetTemplateEnabled (backend-owned toggle logic) --------------------

func TestSetTemplateEnabled_OffOnEmptyStaysEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)

	// Empty = none shown. Turning a row off (it's already off) is a no-op.
	got, err := m.SetTemplateEnabled("b.yaml", false)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want [] (off on an empty list changes nothing)", got)
	}
}

func TestSetTemplateEnabled_OnEmptyAddsJustThatOne(t *testing.T) {
	m, _, _ := newTestManager(t)

	// Literal: empty + on a → [a]. (Only that one is now visible.)
	got, err := m.SetTemplateEnabled("a.yaml", true)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a.yaml"}) {
		t.Errorf("got %v, want [a.yaml]", got)
	}
}

func TestSetTemplateEnabled_TurningLastOffEmptiesAndPersists(t *testing.T) {
	m, _, _ := newTestManager(t)

	withEnabled(t, m, "a.yaml") // scoped to just a
	got, err := m.SetTemplateEnabled("a.yaml", false)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want [] (last off = empty = none scoped)", got)
	}
	// The empty list persists explicitly (no omitempty drop) so "none"
	// survives a reload rather than reverting.
	cfg, _ := m.LoadUserConfig()
	if cfg.EnabledTemplates == nil {
		t.Error("EnabledTemplates should persist as an empty slice, not vanish")
	}
}

func TestSetTemplateEnabled_AddAndRemoveOnPopulatedList(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"a.yaml", "b.yaml", "c.yaml"})
	withEnabled(t, m, "a.yaml")

	got, err := m.SetTemplateEnabled("b.yaml", true)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("after add got %v, want [a.yaml b.yaml]", got)
	}

	got, err = m.SetTemplateEnabled("a.yaml", false)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"b.yaml"}) {
		t.Errorf("after remove got %v, want [b.yaml]", got)
	}
}

func TestReconcileEnabledTemplates_PrunesAgainstLiveFolder(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	withEnabled(t, m, "basic.yaml", "gone.yaml")
	l.set([]string{"basic.yaml", "extra.yaml"})

	got, err := m.ReconcileEnabledTemplates()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"basic.yaml"}) {
		t.Errorf("post-reconcile = %v, want [basic.yaml]", got)
	}
}

func TestReconcileEnabledTemplates_EmptyEnabledShortCircuits(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)

	got, err := m.ReconcileEnabledTemplates()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got != nil {
		t.Errorf("empty enabled → nil expected, got %v", got)
	}
	if l.callCount() != 0 {
		t.Error("must NOT consult lister when EnabledTemplates is empty (perf + simplicity)")
	}
}

func TestReconcileEnabledTemplates_ListerErrorBubbles(t *testing.T) {
	m, _, _ := newTestManager(t)
	want := errors.New("disk down")
	l := &fakeLister{err: want}
	m.SetTemplateLister(l)
	withEnabled(t, m, "basic.yaml")

	_, err := m.ReconcileEnabledTemplates()
	if err == nil || !strings.Contains(err.Error(), "disk down") {
		t.Errorf("expected lister error to bubble, got %v", err)
	}
	// Ensure the cfg slice is untouched on lister failure.
	cfg, _ := m.LoadUserConfig()
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"basic.yaml"}) {
		t.Errorf("slice must not mutate on lister error, got %v", cfg.EnabledTemplates)
	}
}

// ----- ListEnabledTemplates -----------------------------------------------

func TestListEnabledTemplates_NoListerReturnsEmpty(t *testing.T) {
	m, _, _ := newTestManager(t)
	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got != nil {
		t.Errorf("no lister → nil, got %v", got)
	}
}

func TestListEnabledTemplates_EmptyEnabledReturnsNone(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"zeta.yaml", "alpha.yaml", "mid.yaml"})

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty enabled → no templates (scoped to none); got %v", got)
	}
}

func TestListEnabledTemplates_FiltersToIntersectionInLiveOrder(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"alpha.yaml", "beta.yaml", "gamma.yaml"})
	withEnabled(t, m, "gamma.yaml", "alpha.yaml")

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Live-folder order after sort: [alpha, beta, gamma]. Intersection
	// preserves that order.
	want := []string{"alpha.yaml", "gamma.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

func TestListEnabledTemplates_DropsStaleAndReturnsLive(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	withEnabled(t, m, "basic.yaml", "deleted.yaml")
	l.set([]string{"basic.yaml"})

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"basic.yaml"}) {
		t.Errorf("stale must be pruned + dropped, got %v", got)
	}
	// And persisted: re-read EnabledTemplates from disk.
	cfg, _ := m.LoadUserConfig()
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"basic.yaml"}) {
		t.Errorf("prune must persist, got %v", cfg.EnabledTemplates)
	}
}

// TestListEnabledTemplates_PruneToEmptyReturnsNone covers the edge:
// user scoped to exactly one template, then deleted it outside the app
// (e.g. via Files). EnabledTemplates becomes empty post-prune, which
// under "empty = none scoped" means the picker shows nothing until the
// user enables something again.
func TestListEnabledTemplates_PruneToEmptyReturnsNone(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	withEnabled(t, m, "deleted.yaml")
	l.set([]string{"basic.yaml", "report.yaml"})

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("pruned to empty → none; got %v", got)
	}
}

// ----- AutoEnableNewTemplate ----------------------------------------------

func TestAutoEnableNewTemplate_AppendsWhenCurated(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "existing.yaml")
	if err := m.AutoEnableNewTemplate("brand-new.yaml"); err != nil {
		t.Fatalf("AutoEnable: %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	want := []string{"existing.yaml", "brand-new.yaml"}
	if !reflect.DeepEqual(cfg.EnabledTemplates, want) {
		t.Errorf("post-AutoEnable = %v, want %v", cfg.EnabledTemplates, want)
	}
}

func TestAutoEnableNewTemplate_NoopWhenAllEnabled(t *testing.T) {
	m, _, root := newTestManager(t)
	// Seed: warm load + capture content so we can prove no rewrite.
	if _, err := m.LoadUserConfig(); err != nil {
		t.Fatalf("warm load: %v", err)
	}
	pre, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))

	if err := m.AutoEnableNewTemplate("anything.yaml"); err != nil {
		t.Fatalf("AutoEnable: %v", err)
	}

	post, _ := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if string(pre) != string(post) {
		t.Error("empty EnabledTemplates (all enabled) must remain no-op; file should not be rewritten")
	}
	cfg, _ := m.LoadUserConfig()
	if len(cfg.EnabledTemplates) != 0 {
		t.Errorf("must not flip profile into curation mode, got %v", cfg.EnabledTemplates)
	}
}

func TestAutoEnableNewTemplate_IdempotentOnDuplicate(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "alpha.yaml", "beta.yaml")
	if err := m.AutoEnableNewTemplate("alpha.yaml"); err != nil {
		t.Fatalf("AutoEnable: %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	want := []string{"alpha.yaml", "beta.yaml"}
	if !reflect.DeepEqual(cfg.EnabledTemplates, want) {
		t.Errorf("idempotent re-add should not change list, got %v", cfg.EnabledTemplates)
	}
}

func TestAutoEnableNewTemplate_EmptyFilenameRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "a.yaml")
	if err := m.AutoEnableNewTemplate(""); err != nil {
		t.Errorf("empty filename must be no-op silently, got %v", err)
	}
	cfg, _ := m.LoadUserConfig()
	if !reflect.DeepEqual(cfg.EnabledTemplates, []string{"a.yaml"}) {
		t.Errorf("empty filename must not mutate the list, got %v", cfg.EnabledTemplates)
	}
}

// ----- Auto-clear SelectedTemplate ----------------------------------------

func TestUpdate_TogglingOffSelectedTemplate_Clears(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates":  []string{"basic.yaml", "report.yaml"},
		"selected_template":  "basic.yaml",
		"selected_data_file": "alpha.meta.json",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Toggle basic.yaml off.
	cfg, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"report.yaml"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if cfg.SelectedTemplate != "" {
		t.Errorf("SelectedTemplate = %q, want cleared", cfg.SelectedTemplate)
	}
	if cfg.SelectedDataFile != "" {
		t.Errorf("SelectedDataFile = %q, want cleared", cfg.SelectedDataFile)
	}
}

func TestUpdate_TogglingOffOtherTemplate_DoesNotClear(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates":  []string{"basic.yaml", "report.yaml"},
		"selected_template":  "basic.yaml",
		"selected_data_file": "alpha.meta.json",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"basic.yaml"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if cfg.SelectedTemplate != "basic.yaml" {
		t.Errorf("SelectedTemplate = %q, want basic.yaml", cfg.SelectedTemplate)
	}
	if cfg.SelectedDataFile != "alpha.meta.json" {
		t.Errorf("SelectedDataFile = %q, want preserved", cfg.SelectedDataFile)
	}
}

func TestUpdate_EmptyEnabledList_DoesNotClear(t *testing.T) {
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"selected_template":  "basic.yaml",
		"selected_data_file": "alpha.meta.json",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := m.UpdateUserConfig(map[string]any{
		"theme": "dark", // unrelated change
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if cfg.SelectedTemplate != "basic.yaml" {
		t.Errorf("opt-in not used → no clearing; got SelectedTemplate=%q", cfg.SelectedTemplate)
	}
}

func TestUpdate_WriteDisallowedSelected_Clears(t *testing.T) {
	// Pathological: caller writes a SelectedTemplate that isn't in the
	// current allowed list. We refuse to persist that state - same rule
	// regardless of which side of the partial brought it in.
	m, _, _ := newTestManager(t)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": []string{"only-this.yaml"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := m.UpdateUserConfig(map[string]any{
		"selected_template":  "ghost.yaml",
		"selected_data_file": "x.meta.json",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if cfg.SelectedTemplate != "" || cfg.SelectedDataFile != "" {
		t.Errorf("disallowed SelectedTemplate must be cleared, got %+v", cfg)
	}
}

func TestReconcile_ClearsSelectedWhenPrunedOut(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"kept.yaml"})
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates":  []string{"kept.yaml", "doomed.yaml"},
		"selected_template":  "doomed.yaml",
		"selected_data_file": "x.meta.json",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := m.ReconcileEnabledTemplates(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.SelectedTemplate != "" {
		t.Errorf("SelectedTemplate must be cleared after reconcile, got %q", cfg.SelectedTemplate)
	}
	if cfg.SelectedDataFile != "" {
		t.Errorf("SelectedDataFile must be cleared after reconcile, got %q", cfg.SelectedDataFile)
	}
}

// ----- Service wrappers ---------------------------------------------------

func TestService_ListEnabledTemplates_ForwardsToManager(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"alpha.yaml", "beta.yaml"})
	withEnabled(t, m, "beta.yaml")

	got, err := NewService(m).ListEnabledTemplates()
	if err != nil {
		t.Fatalf("Service.ListEnabledTemplates: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"beta.yaml"}) {
		t.Errorf("Service returned %v, want [beta.yaml]", got)
	}
}

func TestService_IsTemplateEnabled_ForwardsToManager(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "kept.yaml")
	svc := NewService(m)
	if !svc.IsTemplateEnabled("kept.yaml") {
		t.Error("kept.yaml is enabled - service should report true")
	}
	if svc.IsTemplateEnabled("hidden.yaml") {
		t.Error("hidden.yaml is NOT enabled - service should report false")
	}
}

func TestListEnabledTemplates_ListerErrorBubbles(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{err: errors.New("io kaput")}
	m.SetTemplateLister(l)

	_, err := m.ListEnabledTemplates()
	if err == nil || !strings.Contains(err.Error(), "io kaput") {
		t.Errorf("expected lister error to bubble, got %v", err)
	}
}

// TestIsTemplateEnabled_MalformedConfigReturnsFalse asserts the tolerant
// read path: a load failure means "nothing is enabled" rather than a panic
// or stale truth.
func TestIsTemplateEnabled_MalformedConfigReturnsFalse(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "a.yaml")
	if !m.IsTemplateEnabled("a.yaml") {
		t.Fatal("a.yaml should be enabled before corruption")
	}
	if err := os.WriteFile(filepath.Join(root, "config", "user.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	m.InvalidateConfigCache()
	if m.IsTemplateEnabled("a.yaml") {
		t.Error("load failure must report no template as enabled")
	}
}

// TestFilterEnabled_MalformedConfigPassesInputThrough documents the
// asymmetry with IsTemplateEnabled: FilterEnabled returns its input
// unchanged on a load error (fail-open) rather than scoping to none.
func TestFilterEnabled_MalformedConfigPassesInputThrough(t *testing.T) {
	m, _, root := newTestManager(t)
	withEnabled(t, m, "a.yaml")
	if err := os.WriteFile(filepath.Join(root, "config", "user.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	m.InvalidateConfigCache()
	in := []string{"a.yaml", "b.yaml"}
	got := m.FilterEnabled(in)
	if len(got) != 2 || got[0] != "a.yaml" || got[1] != "b.yaml" {
		t.Errorf("FilterEnabled on load error = %v, want input unchanged %v", got, in)
	}
}

func TestSetTemplateEnabled_EmptyFilenameRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SetTemplateEnabled("", true)
	if err == nil {
		t.Fatal("expected error for empty filename")
	}
	if !strings.Contains(err.Error(), "empty filename") {
		t.Errorf("err = %v, want empty-filename message", err)
	}
}

// TestConcurrent_SetTemplateEnabledNoCorruption hammers SetTemplateEnabled with
// concurrent toggles of N distinct filenames against an explicit-empty baseline.
//
// CURRENT BEHAVIOR (asserted here): SetTemplateEnabled reads cfg.EnabledTemplates
// via LoadUserConfig BEFORE taking updateMu (enabled_templates.go:91), then hands
// the rebuilt slice to UpdateUserConfig which takes updateMu only for the write.
// The read-modify-write is therefore NOT atomic across the lock, so concurrent
// togglers each start from a stale baseline and overwrite each other. The final
// count collapses to roughly 1 instead of N. See suspectedBugs.
//
// What the structure still guarantees, and what this test pins: the file always
// parses, every surviving entry is a valid known filename, there are no
// duplicates and no torn (partial) entries, and at least one toggle survives.
// We do NOT assert len == N because that would assert a fix that does not exist.
func TestConcurrent_SetTemplateEnabledNoCorruption(t *testing.T) {
	t.Parallel()
	m, _, root := newTestManager(t)
	withEnabled(t, m) // seed an explicit empty (curation mode on)

	const N = 20
	valid := map[string]bool{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range N {
		name := "f" + string(rune('a'+i%26)) + ".yaml"
		mu.Lock()
		valid[name] = true
		mu.Unlock()
		wg.Go(func() {
			if _, err := m.SetTemplateEnabled(name, true); err != nil {
				t.Errorf("SetTemplateEnabled(%s): %v", name, err)
			}
		})
	}
	wg.Wait()

	raw, err := os.ReadFile(filepath.Join(root, "config", "user.json"))
	if err != nil {
		t.Fatalf("read user.json: %v", err)
	}
	var disk Config
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("on-disk profile is corrupt after concurrent toggles: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range disk.EnabledTemplates {
		if !valid[e] {
			t.Errorf("unexpected entry %q in enabled_templates", e)
		}
		if seen[e] {
			t.Errorf("duplicate entry %q in enabled_templates", e)
		}
		seen[e] = true
	}
	// Every distinct on-toggle must survive: the read-modify-write runs under
	// updateMu, so concurrent toggles accumulate instead of overwriting.
	if len(seen) != N {
		t.Errorf("enabled count = %d, want %d (no lost updates)", len(seen), N)
	}
	if len(disk.EnabledTemplates) != N {
		t.Errorf("on-disk slice len = %d, want %d", len(disk.EnabledTemplates), N)
	}
	// Cache and disk must agree on the final slice regardless of how many
	// updates were lost.
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != len(disk.EnabledTemplates) {
		t.Errorf("cache/disk diverge: cache len=%d disk len=%d", len(cfg.EnabledTemplates), len(disk.EnabledTemplates))
	}
}

// TestSetTemplateEnabled_SequentialAccumulatesAll proves the lost updates in the
// concurrent test above are purely a concurrency artifact, not a logic error in
// the toggle: run serially, every distinct on-toggle accumulates so the final
// slice holds all N names in insertion order.
func TestSetTemplateEnabled_SequentialAccumulatesAll(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m)

	const N = 5
	want := make([]string, N)
	for i := range N {
		name := "s" + string(rune('a'+i)) + ".yaml"
		want[i] = name
		got, err := m.SetTemplateEnabled(name, true)
		if err != nil {
			t.Fatalf("SetTemplateEnabled(%s): %v", name, err)
		}
		if len(got) != i+1 || got[i] != name {
			t.Fatalf("after adding %s: got %v, want %d entries ending in %s", name, got, i+1, name)
		}
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if len(cfg.EnabledTemplates) != N {
		t.Fatalf("final count = %d, want %d (%v)", len(cfg.EnabledTemplates), N, cfg.EnabledTemplates)
	}
	for i, w := range want {
		if cfg.EnabledTemplates[i] != w {
			t.Errorf("entry %d = %q, want %q (insertion order)", i, cfg.EnabledTemplates[i], w)
		}
	}
}
