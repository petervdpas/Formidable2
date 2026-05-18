// Package plugin owns Formidable's user-extensibility surface: a
// sandboxed Lua runtime (gopher-lua) that runs scripts shipped in
// <AppRoot>/plugins/<id>/{plugin.json, main.lua}.
//
// Slice 1 (this file's scope) covers on-demand commands only —
// the user clicks "Run" in the Plugins workspace, the script
// executes once, returns a JSON-serialisable value. No event
// hooks, no in-Lua dialogs yet; those are slice 2 + 3.
//
// Tight contract: every public surface here is small and explicit
// so it can be evolved under semver without surprising plugin
// authors. Two version numbers protect that:
//
//   - ManifestSchemaVersion — the on-disk plugin.json shape. Bumped
//     when a field's meaning changes; older manifests stay loadable
//     until we drop them on a major release.
//   - LuaAPIVersion         — the in-VM `formidable.api_version`
//     constant. Plugins can `assert(formidable.api_version >= 1)`
//     before using the surface.
package plugin

import (
	"errors"
	"slices"
)

// ManifestSchemaVersion is the only manifest schema understood by
// this build. Bumped when plugin.json's required fields change
// shape. Manifests with a higher schema version are rejected with
// ErrManifestVersion; lower versions can be supported via
// migration in manifest.go when we ever bump.
const ManifestSchemaVersion = 1

// LuaAPIVersion is exposed inside the VM as
// `formidable.api_version`. Bumped when the `formidable.*` Lua
// surface changes in a non-additive way. Adding a new function to
// an existing namespace does NOT bump this; renaming or removing
// one does.
const LuaAPIVersion = 1

// Workspace* — the closed enum of workspace IDs a plugin may
// attach to via Manifest.Workspaces. An empty list (or omitted
// field) means "not attached to any workspace" — the plugin is
// still runnable from the Plugins workspace's Run modal but no
// topbar menu item is contributed elsewhere. A plugin may attach
// to several workspaces at once.
//
// Kept here rather than on the frontend so the backend stays the
// single source of truth: Service.ListWorkspaces() ships the list
// to Vue's manifest-editor dropdown. Adding a workspace = one new
// constant + entry in ValidWorkspaces.
const (
	WorkspaceStorage       = "storage"
	WorkspaceTemplates     = "templates"
	WorkspaceProfiles      = "profiles"
	WorkspaceCollaboration = "collaboration"
	WorkspaceInformation   = "information"
)

// ValidWorkspaces returns the closed enum of workspace IDs a
// plugin manifest may declare. Order matches the ribbon's natural
// reading order so the frontend dropdown is consistent without
// re-sorting. The Plugins workspace itself is intentionally
// excluded — that's the management view where every plugin lives
// regardless of attachment.
func ValidWorkspaces() []string {
	return []string{
		WorkspaceStorage,
		WorkspaceTemplates,
		WorkspaceProfiles,
		WorkspaceCollaboration,
		WorkspaceInformation,
	}
}

// isValidWorkspace reports whether ws is one of the known
// workspace IDs. Empty is allowed at validate-time (means "no
// attachment") and handled separately by validateManifest.
func isValidWorkspace(ws string) bool {
	return slices.Contains(ValidWorkspaces(), ws)
}

// Manifest is the parsed plugin.json. Field names mirror the JSON
// shape one-for-one; the `manifest_version` JSON field is required
// and equals ManifestSchemaVersion for now.
//
// Versionable surface: extra unknown fields are tolerated by
// json.Unmarshal so plugins authored against a newer Formidable
// don't silently break here — they just don't use the new fields.
//
// RunMode controls how the user interacts with the plugin:
//   - "" (default) / "modal" — Run modal lists each command as a
//     card; ctx is empty for every call.
//   - "form" — the plugin's form (form.json) is the entry point;
//     it renders at the top of the Run modal and every command
//     receives the current form values as ctx.
type Manifest struct {
	ManifestVersion        int      `json:"manifest_version"`
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	Description            string   `json:"description,omitempty"`
	Author                 string   `json:"author,omitempty"`
	RunMode                string   `json:"run_mode,omitempty"`
	RequiresInternalServer bool     `json:"requires_internal_server,omitempty"`
	// Workspaces lists the workspace IDs (from the Workspace* enum
	// in this file) where the plugin contributes a topbar menu
	// entry. Each entry must be a known workspace ID; nil/empty
	// means the plugin is unattached and only surfaces from the
	// Plugins workspace's Run modal.
	Workspaces []string `json:"workspaces,omitempty"`
	// Debug toggles the collapsible debug/output panel at the bottom
	// of the Run modal. Off by default — plugin authors flip it on
	// while iterating, then turn it off when shipping.
	Debug bool `json:"debug"`
	// Progress declares that the plugin reports progress via
	// formidable.progress.tick. Off by default — when true the Run
	// modal renders a live progress bar; otherwise it stays hidden
	// (a plugin that doesn't tick would otherwise show a permanently
	// empty bar). The Stop button is always available while a run is
	// in flight regardless of this flag — cancellation works either
	// way; this flag controls only the bar's visibility.
	Progress bool      `json:"progress"`
	Commands []Command `json:"commands,omitempty"`
}

// RunMode* — the closed enum of values RunMode accepts. Empty is
// also tolerated and behaves like RunModeModal so legacy manifests
// don't need a backfill.
const (
	RunModeModal = "modal"
	RunModeForm  = "form"
)

// Command is one user-runnable entry exposed by the plugin. `ID`
// is referenced by Manager.Run; `Fn` is the Lua function name
// inside main.lua that gets called. When Fn is empty the command
// ID itself is used as the function name (so a command with
// id="export" calls global function `export(ctx)` in Lua).
//
// HideOutput and HideLog let a command opt out of showing the
// corresponding panel in the Run dialog — useful for "fire and
// forget" actions whose return value is irrelevant. LogAsToast
// additionally surfaces every formidable.log.* line as a live
// toast, useful while developing a plugin. FormButton marks the
// command as a button to be rendered inside the plugin's form
// (when one exists) — the form runtime reads this when wiring its
// action bar; it's a manifest hint with no behavior yet on the
// Run modal side.
//
// All four boolean flags are written explicitly (no omitempty) so
// hand-editors see every available option at a glance and diffs
// stay legible (a change reads "true → false", not "added field").
type Command struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Fn         string `json:"fn,omitempty"`
	HideOutput bool   `json:"hide_output"`
	HideLog    bool   `json:"hide_log"`
	LogAsToast bool   `json:"log_as_toast"`
	FormButton bool   `json:"form_button"`
}

// Plugin is a discovered, validated plugin entry held by the
// Manager. Dir is the absolute path to the plugin folder; the
// runtime resolves main.lua as Dir + "/main.lua".
type Plugin struct {
	Manifest Manifest `json:"manifest"`
	Dir      string   `json:"dir"`
}

// PluginInfo is the immutable snapshot Lua scripts read via
// formidable.plugin.*. Built once per Run from the manifest plus
// the command being dispatched, so a script can branch on its
// own identity, mode, or current command without going through
// the bound Vue services.
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
	// Form is the parsed contents of form.json — an array of field
	// definitions when the plugin has one, or nil/empty when it
	// doesn't. Surfaces in Lua as `formidable.plugin.form` so
	// scripts can introspect the schema (and dump it via
	// formidable.json.encode for debugging).
	Form []map[string]any
}

// ToastEvent is one user-facing toast a plugin script asked the
// frontend to show via formidable.toast.{info,success,warn,error}.
// Collected during a Run; surfaced on RunResult.Toasts so Vue can
// dispatch them through useToast verbatim.
type ToastEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// ProgressEvent is one tick emitted by formidable.progress.tick. Done
// is the items completed so far; Total is the planned total (0 when
// the plugin doesn't know yet — Vue shows an indeterminate bar). Message
// is the optional per-item label. Unlike Log/Toast, progress events
// stream out *during* a Run (via the ProgressEmitter callback) — they
// are not buffered onto RunResult, so the UI can render a live bar
// instead of waiting for the script to finish.
type ProgressEvent struct {
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

// RunResult is the JSON-shaped envelope returned to Vue. Value
// holds the Lua function's return value after lvalue→Go
// conversion; LogLines collects formidable.log.* output emitted
// during the call so the workspace panel can show it next to the
// result; Toasts collects formidable.toast.* events for the
// frontend to surface as live notifications.
type RunResult struct {
	Value    any          `json:"value"`
	LogLines []string     `json:"logLines,omitempty"`
	Toasts   []ToastEvent `json:"toasts,omitempty"`
}

// ─────────────────────────────────────────────────────────────────
// Error sentinels — closed vocabulary the Service surfaces to Vue
// so the frontend can branch on Kind (not on message text).
// ─────────────────────────────────────────────────────────────────

var (
	ErrManifestVersion  = errors.New("plugin: unsupported manifest_version")
	ErrManifestInvalid  = errors.New("plugin: invalid manifest")
	ErrPluginNotFound   = errors.New("plugin: not found")
	ErrCommandNotFound  = errors.New("plugin: command not found")
	ErrPluginExists     = errors.New("plugin: already exists")
	ErrServerNotRunning = errors.New("plugin: requires the internal server, but it's stopped")
	// ErrPluginBusy is returned when Manager.Run is invoked while
	// another command is already in flight. Plugins are serialized
	// globally — gopher-lua VMs are per-call but their bound services
	// (KV writes, render, fs) aren't safe to interleave, and the UX
	// contract is "one plugin at a time" anyway. Frontend surfaces
	// this as Kind="busy" via Service.Run.
	ErrPluginBusy = errors.New("plugin: another command is currently running")
	// ErrPluginCancelled is returned when a run's context was
	// cancelled mid-VM (the user pressed Stop, the host shut down,
	// etc.). The runtime maps gopher-lua's "context canceled"
	// surface to this sentinel so the Service layer can branch on
	// Kind="cancelled" instead of inspecting the wrapped error text.
	ErrPluginCancelled = errors.New("plugin: cancelled")
)

// HTTPResponse is what formidable.api.fetch returns to Lua. Body
// is the raw response text (plugin authors decode JSON via
// formidable.json.decode); Headers maps lowercased header names
// to joined values.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers,omitempty"`
}

// HTTPClient is the interface the runtime needs to expose
// formidable.api to Lua scripts. The plugin module is intentionally
// unaware of *what* the client points at — production wraps the
// wiki HTTP server in app.go, but the contract is just "an HTTP
// transport." IsAvailable is queried in the Run preflight when
// manifest.RequiresInternalServer is true; tests pass a small fake.
type HTTPClient interface {
	IsAvailable() bool
	Fetch(method, path, body string, headers map[string]string) (HTTPResponse, error)
}
