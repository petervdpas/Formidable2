# Plugins

Plugins extend Formidable with user-authored Lua scripts. A plugin is
a folder under `<AppRoot>/plugins/<id>/` containing:

| File                         | Purpose                                                       |
| ---------------------------- | ------------------------------------------------------------- |
| `plugin.json`                | Manifest: id, name, version, commands, run mode.              |
| `main.lua`                   | Lua source — each command resolves to a global function.      |
| `form.json`                  | Optional input form rendered when `run_mode: "form"`.         |
| `i18n/<locale>.json`         | Optional per-locale translations (see below).                 |

## Where plugins live

The Plugins workspace lists every folder under `<AppRoot>/plugins/`.
Plugins shipped with Formidable (e.g. `test-plugin`) are seeded from
the binary on first launch — see "Upgrades and re-seeding" below.

## Authoring i18n

Add a `i18n` key to a form field to enable translation, then ship
flat key/value files under `<plugin>/i18n/<locale>.json`. Keys are
auto-prefixed with `plugin.<id>.` at runtime.

```yaml
- key: schema
  type: text
  label: "Schema"
  description: "DB schema."
  i18n: form.schema
```

```json
{
  "form.schema.label": "Schema",
  "form.schema.description": "Database schema."
}
```

Top-level keys `name`, `description`, and `commands.<id>.label` are
read for the plugin's display name, description, and command labels.

The Plugins workspace has an **i18n** tab for editing locale files
directly without leaving the app.

## Calling i18n from Lua

```lua
local label = formidable.i18n.t("form.schema.label")
```

The active locale is resolved from the user's profile; missing keys
fall back to the literal key string.

## Upgrades and re-seeding

The scaffold writes seed files to disk **only when the target path
is missing**. User edits are never clobbered. To re-seed a single
file (e.g. after an upgrade that shipped a new `form.json` shape),
delete it from disk and restart — the bundled copy is written back.

To re-seed an entire bundled plugin, delete the whole folder under
`<AppRoot>/plugins/<id>/` and restart.

## Editing during development

The Plugins workspace ships an editor for every plugin file:

- **Manifest** — name, description, run mode, command list.
- **Lua Source** — `main.lua`, with Ctrl+Enter fullscreen.
- **Form Editor** — fields rendered when `run_mode: "form"`.
- **i18n** — per-locale key/value table; switch locale via the chips.

Save persists every dirty file in one atomic pass; the rest of the
app picks up changes on the next refresh.
