// Package plugin owns Formidable's extensibility surface: a sandboxed gopher-lua runtime that runs
// scripts at <AppRoot>/plugins/<id>/{plugin.json, main.lua}.
//
// Two version numbers gate compatibility:
//   - ManifestSchemaVersion: the on-disk plugin.json shape; bumped when a field's meaning changes.
//   - LuaAPIVersion: the in-VM `formidable.api_version` constant, for `assert(formidable.api_version >= 1)`.
package plugin

import (
	"errors"
	"slices"
)

// ManifestSchemaVersion is the only manifest schema this build understands; higher versions are rejected with ErrManifestVersion.
const ManifestSchemaVersion = 1

// LuaAPIVersion is exposed as `formidable.api_version`; bumped on non-additive changes (rename/remove), not on adding a function.
const LuaAPIVersion = 1

// Workspace* is the closed enum a plugin may attach to via Manifest.Workspaces; an empty list leaves it runnable only from the Plugins Run modal.
// Kept backend-side so ValidWorkspaces is the single source of truth for Vue's manifest-editor dropdown.
const (
	WorkspaceStorage       = "storage"
	WorkspaceTemplates     = "templates"
	WorkspaceProfiles      = "profiles"
	WorkspaceCollaboration = "collaboration"
	WorkspaceInformation   = "information"
)

// ValidWorkspaces returns the workspace IDs a manifest may declare, in ribbon reading order. The Plugins workspace is excluded (it lists every plugin regardless).
func ValidWorkspaces() []string {
	return []string{
		WorkspaceStorage,
		WorkspaceTemplates,
		WorkspaceProfiles,
		WorkspaceCollaboration,
		WorkspaceInformation,
	}
}

// isValidWorkspace reports whether ws is a known workspace ID (empty is handled separately by validateManifest).
func isValidWorkspace(ws string) bool {
	return slices.Contains(ValidWorkspaces(), ws)
}

// Manifest is the parsed plugin.json. Unknown fields are tolerated so newer-authored plugins don't break.
// RunMode: "" / "modal" lists each command as a card (empty ctx); "form" makes form.json the entry point (every command receives the form values as ctx).
type Manifest struct {
	ManifestVersion        int    `json:"manifest_version"`
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Version                string `json:"version"`
	Description            string `json:"description,omitempty"`
	Author                 string `json:"author,omitempty"`
	RunMode                string `json:"run_mode,omitempty"`
	RequiresInternalServer bool   `json:"requires_internal_server,omitempty"`
	// Workspaces lists where the plugin contributes a topbar entry; nil/empty leaves it Run-modal-only.
	Workspaces []string `json:"workspaces,omitempty"`
	// Templates narrows a workspace attachment to exact template filenames; non-empty makes the plugin template-scoped (shows only while a listed template is selected). It only narrows, never broadens.
	Templates []string `json:"templates,omitempty"`
	// Debug toggles the collapsible debug/output panel in the Run modal.
	Debug bool `json:"debug"`
	// Maximizable adds the expand/restore button to the run window.
	Maximizable bool      `json:"maximizable"`
	Commands    []Command `json:"commands,omitempty"`
}

// RunMode* is the closed enum RunMode accepts; empty behaves like RunModeModal so legacy manifests need no backfill.
const (
	RunModeModal = "modal"
	RunModeForm  = "form"
)

// Command is one user-runnable entry. Fn is the Lua function name; empty Fn uses ID (id="export" calls global `export(ctx)`).
// HideOutput/HideLog hide the Run-dialog panels; LogAsToast mirrors log lines as live toasts; FormButton renders it in the form's action bar.
// The boolean flags are written explicitly (no omitempty) so diffs read "true → false", not "added field".
type Command struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Fn         string `json:"fn,omitempty"`
	HideOutput bool   `json:"hide_output"`
	HideLog    bool   `json:"hide_log"`
	LogAsToast bool   `json:"log_as_toast"`
	FormButton bool   `json:"form_button"`
	// OnChange runs (run_mode "form") whenever a form field changes; ctx carries the form values plus `changed` = the changed field's key, so Lua can steer other fields via formidable.run.options. Not a button.
	OnChange bool `json:"on_change"`
}

// Plugin is a discovered, validated plugin entry; Dir is the absolute plugin folder.
type Plugin struct {
	Manifest Manifest `json:"manifest"`
	Dir      string   `json:"dir"`
}

// PluginInfo is the immutable per-Run snapshot Lua scripts read via formidable.plugin.*.
type PluginInfo struct {
	ID                     string
	Name                   string
	Version                string
	Author                 string
	Description            string
	Mode                   string // "form" or "modal"
	Command                string // command id currently running
	RequiresInternalServer bool
	Debug                  bool
	// Form is the parsed form.json (field definitions, or nil), surfaced in Lua as `formidable.plugin.form`.
	Form []map[string]any
}

// ToastEvent is one toast a script requested via formidable.toast.*, collected during a Run.
type ToastEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// RunBarEvent is one tick from formidable.run.bar(done, total); Total == 0 means indeterminate. Cleared at the start of every Run.
type RunBarEvent struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// RunStatusEvent is one message from formidable.run.status(text), for a statusmessage widget. Cleared at the start of every Run.
type RunStatusEvent struct {
	Text string `json:"text"`
}

// RunChartEvent is one spec from formidable.run.chart(spec); Spec is {type, title, result}, fed to StatChart. Cleared at the start of every Run.
type RunChartEvent struct {
	Spec map[string]any `json:"spec"`
}

// RunOptionsEvent is from formidable.run.options(fieldKey, opts): it overlays a field's option list at runtime.
// Options entries are {value, label} maps or bare strings; the frontend re-selects a valid value if the current one drops out.
type RunOptionsEvent struct {
	Field   string `json:"field"`
	Options []any  `json:"options"`
}

// RunResult is the JSON envelope returned to Vue: the converted Value plus collected LogLines and Toasts.
type RunResult struct {
	Value    any          `json:"value"`
	LogLines []string     `json:"logLines,omitempty"`
	Toasts   []ToastEvent `json:"toasts,omitempty"`
}

// Error sentinels: the closed vocabulary the Service surfaces to Vue so the frontend branches on Kind, not message text.
var (
	ErrManifestVersion  = errors.New("plugin: unsupported manifest_version")
	ErrManifestInvalid  = errors.New("plugin: invalid manifest")
	ErrPluginNotFound   = errors.New("plugin: not found")
	ErrCommandNotFound  = errors.New("plugin: command not found")
	ErrPluginExists     = errors.New("plugin: already exists")
	ErrServerNotRunning = errors.New("plugin: requires the internal server, but it's stopped")
	// ErrPluginBusy: plugins run one at a time. VMs are per-call but their bound services (KV/render/fs) aren't safe to interleave.
	ErrPluginBusy = errors.New("plugin: another command is currently running")
	// ErrPluginCancelled: the runtime maps gopher-lua's "context canceled" to this so the Service can branch on Kind="cancelled".
	ErrPluginCancelled = errors.New("plugin: cancelled")
)

// HTTPResponse is what formidable.api.fetch returns to Lua; Headers maps lowercased header names to joined values.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers,omitempty"`
}

// HTTPClient is the transport exposed as formidable.api; IsAvailable is queried in the Run preflight when RequiresInternalServer is set.
type HTTPClient interface {
	IsAvailable() bool
	Fetch(method, path, body string, headers map[string]string) (HTTPResponse, error)
}
