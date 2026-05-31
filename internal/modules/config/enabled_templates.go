package config

import (
	"fmt"
	"slices"
	"sort"
)

// TemplateLister reports which template files exist on disk.
type TemplateLister interface {
	ListTemplates() ([]string, error)
}

// SetTemplateLister installs the lister used to self-heal EnabledTemplates;
// nil disables reconciliation (the read methods still work off cached config).
func (m *Manager) SetTemplateLister(l TemplateLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tplLister = l
}

// IsTemplateEnabled reports whether name is in the active profile's
// EnabledTemplates. Empty list means none visible. Reads cached config, does
// not reconcile.
func (m *Manager) IsTemplateEnabled(name string) bool {
	if name == "" {
		return false
	}
	cfg, err := m.LoadUserConfig()
	if err != nil || cfg == nil {
		return false
	}
	return slices.Contains(cfg.EnabledTemplates, name)
}

// FilterEnabled returns the enabled subset of filenames, input order preserved.
// Empty EnabledTemplates returns nil (scoped to none).
func (m *Manager) FilterEnabled(filenames []string) []string {
	cfg, err := m.LoadUserConfig()
	if err != nil || cfg == nil {
		return filenames
	}
	if len(cfg.EnabledTemplates) == 0 {
		return nil
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

// AutoEnableNewTemplate appends filename only when the profile already curates
// (non-empty list). An empty list means "all enabled", so appending would
// silently flip the profile into curation mode. No-op on empty/present.
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

// SetTemplateEnabled toggles filename's membership and returns the new slice.
// Off-ing the last entry leaves [] (none visible), which persists.
func (m *Manager) SetTemplateEnabled(filename string, on bool) ([]string, error) {
	if filename == "" {
		return nil, fmt.Errorf("set-enabled: empty filename")
	}
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("set-enabled: load: %w", err)
	}

	current := cfg.EnabledTemplates
	next := make([]string, 0, len(current)+1)
	for _, f := range current {
		if f != filename {
			next = append(next, f)
		}
	}
	if on {
		next = append(next, filename)
	}

	if _, err := m.UpdateUserConfig(map[string]any{"enabled_templates": next}); err != nil {
		return nil, fmt.Errorf("set-enabled: persist: %w", err)
	}
	return next, nil
}

// SeedEnabledTemplatesIfUnset seeds a never-configured profile (EnabledTemplates
// nil) with all templates. An explicitly-emptied [] is left alone, so "turn
// everything off" sticks. No-op without a lister or any templates on disk.
func (m *Manager) SeedEnabledTemplatesIfUnset() error {
	cfg, err := m.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("seed-enabled: load: %w", err)
	}
	if cfg.EnabledTemplates != nil {
		return nil
	}
	m.mu.RLock()
	lister := m.tplLister
	m.mu.RUnlock()
	if lister == nil {
		return nil
	}
	all, err := lister.ListTemplates()
	if err != nil {
		return fmt.Errorf("seed-enabled: list: %w", err)
	}
	if len(all) == 0 {
		return nil
	}
	sort.Strings(all)
	if _, err := m.UpdateUserConfig(map[string]any{"enabled_templates": all}); err != nil {
		return fmt.Errorf("seed-enabled: persist: %w", err)
	}
	return nil
}

// normalizeSelectedTemplate clears SelectedTemplate (and SelectedDataFile, which
// points under it) when the pick is no longer enabled. Returns true if mutated.
// Auto-clear chosen 2026-05-18, overriding the design doc's do-not-auto-mutate.
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

// PruneEnabledTemplates drops EnabledTemplates entries not in existing, persists,
// and returns the removed ones. Pass the LIVE template list: nil prunes every
// entry. No-op on empty EnabledTemplates.
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

// ReconcileEnabledTemplates prunes EnabledTemplates against the live folder and
// returns the post-prune slice. Without a lister it returns the current slice.
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

	cfg2, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("reconcile: reload: %w", err)
	}
	return append([]string(nil), cfg2.EnabledTemplates...), nil
}

// ListEnabledTemplates reconciles against the live folder, then returns the
// enabled templates in live-folder order (deterministic). Empty EnabledTemplates
// or no wired lister returns nil.
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
	sort.Strings(existing)

	cfg, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("list-enabled: load: %w", err)
	}
	if len(cfg.EnabledTemplates) == 0 {
		return nil, nil
	}
	if _, err := m.PruneEnabledTemplates(existing); err != nil {
		return nil, err
	}
	cfg2, err := m.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("list-enabled: reload: %w", err)
	}
	if len(cfg2.EnabledTemplates) == 0 {
		return nil, nil
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
