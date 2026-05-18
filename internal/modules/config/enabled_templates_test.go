package config

import (
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

func TestIsTemplateEnabled_EmptyListMeansAllEnabled(t *testing.T) {
	m, _, _ := newTestManager(t)
	if !m.IsTemplateEnabled("anything.yaml") {
		t.Error("empty EnabledTemplates must report every template as enabled")
	}
}

func TestIsTemplateEnabled_PopulatedListOnlyAllowsListed(t *testing.T) {
	m, _, _ := newTestManager(t)
	withEnabled(t, m, "basic.yaml", "report.yaml")
	if !m.IsTemplateEnabled("basic.yaml") {
		t.Error("basic.yaml is in the list — should be enabled")
	}
	if m.IsTemplateEnabled("hidden.yaml") {
		t.Error("hidden.yaml is NOT in the list — should be disabled")
	}
}

func TestIsTemplateEnabled_EmptyNameIsFalse(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m.IsTemplateEnabled("") {
		t.Error("empty name must never be enabled, even with empty list")
	}
}

// ----- FilterEnabled ------------------------------------------------------

func TestFilterEnabled_EmptyListPassesInputThrough(t *testing.T) {
	m, _, _ := newTestManager(t)
	in := []string{"a.yaml", "b.yaml", "c.yaml"}
	got := m.FilterEnabled(in)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("empty list must return input unchanged, got %v", got)
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

	// Capture content (not mtime — Linux ext4 has 1s granularity, plus
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

func TestListEnabledTemplates_EmptyEnabledReturnsAllSorted(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	l.set([]string{"zeta.yaml", "alpha.yaml", "mid.yaml"})

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha.yaml", "mid.yaml", "zeta.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty enabled → all on disk, sorted; got %v want %v", got, want)
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

// TestListEnabledTemplates_PruneToEmptyFallsBackToAll covers the edge:
// user enabled exactly one template, then deleted that template outside
// the app (e.g. via Files). EnabledTemplates becomes empty post-prune,
// which by "empty = all enabled" means we expose every remaining file.
// Less surprising for the user than an empty picker.
func TestListEnabledTemplates_PruneToEmptyFallsBackToAll(t *testing.T) {
	m, _, _ := newTestManager(t)
	l := &fakeLister{}
	m.SetTemplateLister(l)
	withEnabled(t, m, "deleted.yaml")
	l.set([]string{"basic.yaml", "report.yaml"})

	got, err := m.ListEnabledTemplates()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"basic.yaml", "report.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pruned to empty → all live; got %v want %v", got, want)
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
	// current allowed list. We refuse to persist that state — same rule
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
		t.Error("kept.yaml is enabled — service should report true")
	}
	if svc.IsTemplateEnabled("hidden.yaml") {
		t.Error("hidden.yaml is NOT enabled — service should report false")
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
