// Package plugin's Service is the Wails-bound surface Vue talks
// to. Methods are intentionally small: List, Run, Refresh. The
// async dialog mechanism (slice 3) and event-hook firing (slice
// 2) are NOT here yet — slice 1 is on-demand commands only.
package plugin

import (
	"errors"
)

// Service is the Wails-bound facade over Manager. Held by app.App
// and registered as a Wails service in main.go.
type Service struct{ m *Manager }

// NewService wraps a Manager. Manager.Refresh should be called
// once before the Wails app runs so List returns populated data
// on first frame.
func NewService(m *Manager) *Service { return &Service{m: m} }

// ListResult is the Wails return shape for List. Manifest is the
// full parsed plugin.json (so Vue can show name, version, command
// labels). ID is duplicated at the top level for convenience —
// Vue list components use it as the v-for key without digging
// into the nested manifest.
type ListResult struct {
	ID       string   `json:"id"`
	Manifest Manifest `json:"manifest"`
}

// List returns every discovered plugin, sorted by id.
func (s *Service) List() []ListResult {
	plugins := s.m.List()
	out := make([]ListResult, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, ListResult{ID: p.Manifest.ID, Manifest: p.Manifest})
	}
	return out
}

// Refresh re-scans the plugins folder. Returns the new list so
// Vue can update without a follow-up call.
func (s *Service) Refresh() ([]ListResult, error) {
	if err := s.m.Refresh(); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// RunResultDTO is the JSON envelope for Run. Kind is "ok" on
// success or one of the error sentinels' kinds — Vue branches on
// Kind, never on Message text. Toasts pass through whatever
// formidable.toast.* emitted during the call so the workspace can
// dispatch them to useToast.
type RunResultDTO struct {
	Kind     string       `json:"kind"`
	Message  string       `json:"message,omitempty"`
	Value    any          `json:"value,omitempty"`
	LogLines []string     `json:"logLines,omitempty"`
	Toasts   []ToastEvent `json:"toasts,omitempty"`
}

// Create scaffolds a new plugin folder and returns the new list.
// Errors map to the editor sentinels (ErrManifestInvalid,
// ErrPluginExists) — the caller surfaces them as i18n'd toasts.
func (s *Service) Create(id string) ([]ListResult, error) {
	if err := s.m.Create(id); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// Save writes plugin.json + main.lua + form.json for an existing
// plugin and returns the updated list. formJSON is the raw text
// of the field schema; pass "" to leave form.json untouched.
func (s *Service) Save(id string, manifest Manifest, luaSource, formJSON string) ([]ListResult, error) {
	if err := s.m.Save(id, manifest, luaSource, formJSON); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// GetForm returns the raw form.json text for an existing plugin.
// If form.json is missing (legacy or hand-edited folder), returns
// the default empty-array placeholder so callers don't have to
// special-case the absence.
func (s *Service) GetForm(id string) (string, error) {
	return s.m.GetForm(id)
}

// Delete removes the plugin folder and KV file, returning the
// updated list.
func (s *Service) Delete(id string) ([]ListResult, error) {
	if err := s.m.Delete(id); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// GetSource returns the plugin's main.lua content. Used by the
// workspace to populate the Lua editor when a plugin is selected.
func (s *Service) GetSource(id string) (string, error) {
	return s.m.GetSource(id)
}

// Run invokes a command. ctx is an optional plain-JSON table that
// arrives in Lua as the function's single argument. Errors are
// classified rather than returned directly so the frontend can
// branch deterministically: a missing plugin / unknown command /
// load error / runtime error each get a distinct kind.
func (s *Service) Run(pluginID, commandID string, ctx map[string]any) RunResultDTO {
	res, err := s.m.Run(pluginID, commandID, ctx)
	if err == nil {
		return RunResultDTO{
			Kind:     "ok",
			Value:    res.Value,
			LogLines: res.LogLines,
			Toasts:   res.Toasts,
		}
	}
	switch {
	case errors.Is(err, ErrPluginNotFound):
		return RunResultDTO{Kind: "plugin_not_found", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	case errors.Is(err, ErrCommandNotFound):
		return RunResultDTO{Kind: "command_not_found", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	case errors.Is(err, ErrServerNotRunning):
		return RunResultDTO{Kind: "server_not_running", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	default:
		return RunResultDTO{Kind: "runtime_error", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	}
}
