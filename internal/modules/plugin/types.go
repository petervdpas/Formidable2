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

import "errors"

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

// Manifest is the parsed plugin.json. Field names mirror the JSON
// shape one-for-one; the `manifest_version` JSON field is required
// and equals ManifestSchemaVersion for now.
//
// Versionable surface: extra unknown fields are tolerated by
// json.Unmarshal so plugins authored against a newer Formidable
// don't silently break here — they just don't use the new fields.
type Manifest struct {
	ManifestVersion int       `json:"manifest_version"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Description     string    `json:"description,omitempty"`
	Author          string    `json:"author,omitempty"`
	Commands        []Command `json:"commands,omitempty"`
}

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

// ToastEvent is one user-facing toast a plugin script asked the
// frontend to show via formidable.toast.{info,success,warn,error}.
// Collected during a Run; surfaced on RunResult.Toasts so Vue can
// dispatch them through useToast verbatim.
type ToastEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
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
	ErrManifestVersion = errors.New("plugin: unsupported manifest_version")
	ErrManifestInvalid = errors.New("plugin: invalid manifest")
	ErrPluginNotFound  = errors.New("plugin: not found")
	ErrCommandNotFound = errors.New("plugin: command not found")
	ErrPluginExists    = errors.New("plugin: already exists")
)
