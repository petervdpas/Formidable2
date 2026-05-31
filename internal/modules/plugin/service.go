// Service is the Wails-bound surface Vue talks to.
package plugin

import (
	"errors"

	"github.com/petervdpas/formidable2/internal/modules/formwidget"
)

// Service is the Wails-bound facade over Manager.
type Service struct{ m *Manager }

// NewService wraps a Manager; call Manager.Refresh once before the app runs so List is populated on first frame.
func NewService(m *Manager) *Service { return &Service{m: m} }

// ListResult is the Wails return shape for List; ID is duplicated at top level as the Vue v-for key.
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

// ListForWorkspace returns the plugins attached to workspace ws; an unknown id returns an empty slice.
func (s *Service) ListForWorkspace(ws string) []ListResult {
	plugins := s.m.ListForWorkspace(ws)
	out := make([]ListResult, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, ListResult{ID: p.Manifest.ID, Manifest: p.Manifest})
	}
	return out
}

// ListForTemplate returns workspace ws's plugins given the active template (workspace plugins plus template-scoped ones bound to it); empty template yields workspace plugins only.
func (s *Service) ListForTemplate(ws, template string) []ListResult {
	plugins := s.m.ListForTemplate(ws, template)
	out := make([]ListResult, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, ListResult{ID: p.Manifest.ID, Manifest: p.Manifest})
	}
	return out
}

// ListWorkspaces returns the closed enum of workspace ids a manifest may attach to.
func (s *Service) ListWorkspaces() []string {
	return ValidWorkspaces()
}

// RunResultDTO is the JSON envelope for Run; Vue branches on Kind, never on Message text. Toasts pass through formidable.toast.* output.
type RunResultDTO struct {
	Kind     string       `json:"kind"`
	Message  string       `json:"message,omitempty"`
	Value    any          `json:"value,omitempty"`
	LogLines []string     `json:"logLines,omitempty"`
	Toasts   []ToastEvent `json:"toasts,omitempty"`
}

// Create scaffolds a new plugin folder and returns the updated list.
func (s *Service) Create(id string) ([]ListResult, error) {
	if err := s.m.Create(id); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// Save writes the plugin and returns the updated list; pass "" formJSON to leave form.json untouched.
func (s *Service) Save(id string, manifest Manifest, luaSource, formJSON string) ([]ListResult, error) {
	if err := s.m.Save(id, manifest, luaSource, formJSON); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// GetForm returns the raw form.json text, or the empty-array placeholder when missing.
func (s *Service) GetForm(id string) (string, error) {
	return s.m.GetForm(id)
}

// ValidateWidget runs formwidget rules against one widget; also pins the formwidget types into the Wails bindings so the frontend can import Widget/Kind.
func (s *Service) ValidateWidget(w formwidget.Widget) error {
	return w.Validate()
}

// Delete removes the plugin folder and KV file, returning the updated list.
func (s *Service) Delete(id string) ([]ListResult, error) {
	if err := s.m.Delete(id); err != nil {
		return nil, err
	}
	return s.List(), nil
}

// ExportArchive bundles a plugin's folder into a zip at zipPath.
func (s *Service) ExportArchive(id, zipPath string) (ExportArchiveResult, error) {
	return s.m.ExportArchive(id, zipPath)
}

// ImportArchive unpacks a plugin-archive zip under <PluginsDir>/; the frontend uses ErrPluginArchiveExists to surface a "replace?" prompt.
func (s *Service) ImportArchive(zipPath string, overwrite bool) (ImportArchiveResult, error) {
	return s.m.ImportArchive(zipPath, overwrite)
}

// GetSource returns the plugin's main.lua content.
func (s *Service) GetSource(id string) (string, error) {
	return s.m.GetSource(id)
}

// LoadFormValues returns the stored values for fieldKeys, pre-populating the Run modal from the plugin's KV bag.
func (s *Service) LoadFormValues(pluginID string, fieldKeys []string) map[string]any {
	return s.m.LoadFormValues(pluginID, fieldKeys)
}

// SaveFormValues writes each (fieldKey, value) into the plugin's KV bag, readable from Lua via formidable.kv.get(fieldKey).
func (s *Service) SaveFormValues(pluginID string, values map[string]any) error {
	return s.m.SaveFormValues(pluginID, values)
}

// Run invokes a command; errors are classified into distinct Kinds so the frontend branches deterministically.
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
	case errors.Is(err, ErrPluginBusy):
		return RunResultDTO{Kind: "busy", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	case errors.Is(err, ErrPluginCancelled):
		return RunResultDTO{Kind: "cancelled", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	default:
		return RunResultDTO{Kind: "runtime_error", Message: err.Error(), LogLines: res.LogLines, Toasts: res.Toasts}
	}
}

// Cancel signals the currently-running plugin to abort; safe from any goroutine.
func (s *Service) Cancel() {
	s.m.Cancel()
}

// GetI18nMessages returns the merged plugin translations for locale, keys pre-namespaced `plugin.<id>.<key>` for direct vue-i18n merge.
func (s *Service) GetI18nMessages(locale string) map[string]string {
	return s.m.MessagesForLocale(locale)
}

// Plugin i18n editor surface: keys are stored verbatim on disk (no prefix); MessagesForLocale applies the `plugin.<id>.` prefix at read time.

// GetPluginI18n returns the raw on-disk map for <plugin>/i18n/<locale>.json (no auto-prefix), empty when missing.
func (s *Service) GetPluginI18n(id, locale string) (map[string]string, error) {
	return s.m.GetI18nFile(id, locale)
}

// SavePluginI18n writes the raw flat map for one locale; locale is validated against path-traversal.
func (s *Service) SavePluginI18n(id, locale string, msgs map[string]string) error {
	return s.m.SaveI18nFile(id, locale, msgs)
}

// DeletePluginI18n removes the locale file; a missing file is a silent no-op.
func (s *Service) DeletePluginI18n(id, locale string) error {
	return s.m.DeleteI18nFile(id, locale)
}

// ListPluginLocales returns the plugin's locale ids sorted, empty slice when it has no i18n/ folder.
func (s *Service) ListPluginLocales(id string) ([]string, error) {
	return s.m.ListI18nLocales(id)
}
