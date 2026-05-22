package config

import (
	"fmt"
	"slices"
	"sort"
)

// TemplateLister is the narrow surface needed by ReconcileEnabledTemplates
// + ListEnabledTemplates to find out which template files actually exist
// on disk. template.Manager satisfies it directly; the composition root
// wires the dependency via SetTemplateLister so config doesn't import
// template.
type TemplateLister interface {
	ListTemplates() ([]string, error)
}

// SetTemplateLister installs the live template-folder lister used for
// self-healing the EnabledTemplates slice. Pass nil to disable
// reconciliation - IsTemplateEnabled/FilterEnabled still work against
// the cached config slice.
func (m *Manager) SetTemplateLister(l TemplateLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tplLister = l
}

// IsTemplateEnabled reports whether the given template filename is in the
// active profile's EnabledTemplates list. Empty/nil list semantically means
// "all templates enabled" - the design's opt-in switch - so this returns
// true for any non-empty name in that case. Empty name is always false.
//
// Reads cached config; does NOT reconcile. Callers wanting the post-prune
// answer should hit ListEnabledTemplates first (or just consume that
// list).
func (m *Manager) IsTemplateEnabled(name string) bool {
	if name == "" {
		return false
	}
	cfg, err := m.LoadUserConfig()
	if err != nil || cfg == nil {
		return false
	}
	if len(cfg.EnabledTemplates) == 0 {
		return true
	}
	return slices.Contains(cfg.EnabledTemplates, name)
}

// FilterEnabled returns the subset of `filenames` that are enabled in the
// active profile. Empty/nil EnabledTemplates → input returned unchanged
// (the "all enabled" semantic). Preserves input order; does not dedupe.
func (m *Manager) FilterEnabled(filenames []string) []string {
	cfg, err := m.LoadUserConfig()
	if err != nil || cfg == nil {
		return filenames
	}
	if len(cfg.EnabledTemplates) == 0 {
		return filenames
	}
	allow := make(map[string]struct{}, len(cfg.EnabledTemplates))
	for _, e := range cfg.EnabledTemplates {
		allow[e] = struct{}{}
	}
	out := make([]string, 0, len(filenames))
	for _, f := range filenames {
		if _, ok := allow[f]; ok {
			out = append(out, f)
		}
	}
	return out
}

// AutoEnableNewTemplate appends filename to the active profile's
// EnabledTemplates list IFF the profile has opted into curation (the
// list is non-empty). Wired from the composition root onto
// template.Manager.AddCreationObserver so a newly-created template
// from the editor becomes visible in the (filtered) editor sidebar
// immediately, instead of being hidden until the user toggles it in
// Settings → Templates.
//
// Why guarded on "opted into curation":
//   - Empty/nil EnabledTemplates means "all enabled" - adding the
//     filename would flip the profile into curation mode, which is a
//     bigger semantic change than the user asked for. Stay no-op.
//   - Populated list means the user explicitly curated. They just
//     created a new template; they want to use it. Append.
//
// Idempotent: a duplicate entry would never appear (we check membership
// first), so accidental re-fires from the observer chain are safe.
// Empty filename is rejected so a programmatic caller can't pollute
// the list with garbage.
func (m *Manager) AutoEnableNewTemplate(filename string) error {
	if filename == "" {
		return nil
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("auto-enable: load: %w", err)
	}
	if len(cfg.EnabledTemplates) == 0 {
		return nil
	}
	if slices.Contains(cfg.EnabledTemplates, filename) {
		return nil
	}
	next := append(append([]string(nil), cfg.EnabledTemplates...), filename)
	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": next,
	}); err != nil {
		return fmt.Errorf("auto-enable: persist: %w", err)
	}
	return nil
}

// normalizeSelectedTemplate inspects cfg in-place and clears
// SelectedTemplate + SelectedDataFile when the user's pick is no longer
// in the enabled set. Returns true iff anything mutated.
//
// Rules:
//   - SelectedTemplate empty: noop.
//   - EnabledTemplates empty (opt-in not used): noop, since "all are
//     enabled" implies SelectedTemplate is fine.
//   - Otherwise: if SelectedTemplate isn't present in EnabledTemplates,
//     wipe both SelectedTemplate and SelectedDataFile. SelectedDataFile
//     references a form under the now-orphaned template; leaving it
//     would have the storage workspace dangling on a record whose
//     schema is no longer visible.
//
// This is the "auto-clear" behavior the user enabled on 2026-05-18,
// overriding the design doc's "do not auto-mutate SelectedTemplate".
func normalizeSelectedTemplate(cfg *Config) bool {
	if cfg.SelectedTemplate == "" {
		return false
	}
	if len(cfg.EnabledTemplates) == 0 {
		return false
	}
	if slices.Contains(cfg.EnabledTemplates, cfg.SelectedTemplate) {
		return false
	}
	cfg.SelectedTemplate = ""
	cfg.SelectedDataFile = ""
	return true
}

// PruneEnabledTemplates removes from EnabledTemplates any entry not in
// `existing`. Persists if anything changed. Returns the slice of removed
// filenames (for logging / future audit; current callers ignore it).
//
// `existing` is treated as the source of truth for "what's on disk right
// now" - passing nil prunes every entry, which is correct: nothing on
// disk means nothing to enable. Callers must therefore pass the live
// template list, not a possibly-stale snapshot.
//
// Empty/nil EnabledTemplates is a no-op (nothing to prune; the "all
// enabled" semantic doesn't carry stale entries).
func (m *Manager) PruneEnabledTemplates(existing []string) ([]string, error) {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("prune: load: %w", err)
	}
	if len(cfg.EnabledTemplates) == 0 {
		return nil, nil
	}

	keep := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		keep[e] = struct{}{}
	}

	pruned := make([]string, 0, len(cfg.EnabledTemplates))
	var removed []string
	for _, e := range cfg.EnabledTemplates {
		if _, ok := keep[e]; ok {
			pruned = append(pruned, e)
		} else {
			removed = append(removed, e)
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}

	if _, err := m.UpdateUserConfig(map[string]any{
		"enabled_templates": pruned,
	}); err != nil {
		return nil, fmt.Errorf("prune: persist: %w", err)
	}
	return removed, nil
}

// ReconcileEnabledTemplates reads the live template folder via the wired
// TemplateLister and prunes EnabledTemplates against it. Returns the
// post-prune slice; persists only if anything changed.
//
// Without a TemplateLister this is a no-op that returns the current
// EnabledTemplates unchanged - keeps tests + non-template contexts
// runnable without panicking.
func (m *Manager) ReconcileEnabledTemplates() ([]string, error) {
	m.mu.RLock()
	lister := m.tplLister
	m.mu.RUnlock()

	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("reconcile: load: %w", err)
	}
	if lister == nil {
		return append([]string(nil), cfg.EnabledTemplates...), nil
	}
	if len(cfg.EnabledTemplates) == 0 {
		return nil, nil
	}

	existing, err := lister.ListTemplates()
	if err != nil {
		return nil, fmt.Errorf("reconcile: list: %w", err)
	}
	if _, err := m.PruneEnabledTemplates(existing); err != nil {
		return nil, err
	}

	// Re-read post-prune so callers see the persisted state, not the
	// pre-prune cache. PruneEnabledTemplates went through UpdateUserConfig,
	// which already ran normalizeSelectedTemplate - so SelectedTemplate is
	// guaranteed-coherent at this point. The reload here just surfaces
	// the post-prune EnabledTemplates slice.
	cfg2, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("reconcile: reload: %w", err)
	}
	return append([]string(nil), cfg2.EnabledTemplates...), nil
}

// ListEnabledTemplates is the public read for the use-side picker. It
// reconciles against the live folder first, then returns the templates
// the user is allowed to pick from:
//
//   - Empty EnabledTemplates (the default / "opt-in not used") →
//     every template on disk.
//   - Populated EnabledTemplates → intersection of live folder with
//     the (post-prune) slice, preserving the live folder's order so
//     the picker order is deterministic and matches the editor.
//
// Without a TemplateLister wired, returns an empty slice + nil - the
// frontend treats that as "nothing to pick from", which is the safe
// fallback when the host hasn't wired the dependency.
func (m *Manager) ListEnabledTemplates() ([]string, error) {
	m.mu.RLock()
	lister := m.tplLister
	m.mu.RUnlock()
	if lister == nil {
		return nil, nil
	}
	existing, err := lister.ListTemplates()
	if err != nil {
		return nil, fmt.Errorf("list-enabled: list: %w", err)
	}
	// Stable order regardless of OS readdir order. Cheap; lists are tiny.
	sort.Strings(existing)

	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("list-enabled: load: %w", err)
	}
	if len(cfg.EnabledTemplates) == 0 {
		return existing, nil
	}
	// Reconcile first so stale entries can't survive into the picker.
	if _, err := m.PruneEnabledTemplates(existing); err != nil {
		return nil, err
	}
	cfg2, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("list-enabled: reload: %w", err)
	}
	if len(cfg2.EnabledTemplates) == 0 {
		// Pruning emptied the slice (every enabled template was deleted
		// out from under us). Per "empty = all enabled" we expose every
		// remaining file - same as never having opted in. The settings
		// panel will reflect the now-empty slice.
		return existing, nil
	}
	allow := make(map[string]struct{}, len(cfg2.EnabledTemplates))
	for _, e := range cfg2.EnabledTemplates {
		allow[e] = struct{}{}
	}
	out := make([]string, 0, len(existing))
	for _, f := range existing {
		if _, ok := allow[f]; ok {
			out = append(out, f)
		}
	}
	return out, nil
}
