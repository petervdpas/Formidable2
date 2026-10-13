# Customize via Plugins

Plugins extend Formidable with user-authored Lua scripts. A plugin is
a folder under `<AppRoot>/plugins/<id>/` containing:

| File                         | Purpose                                                       |
| ---------------------------- | ------------------------------------------------------------- |
| `plugin.json`                | Manifest: id, name, version, commands, run mode.              |
| `main.lua`                   | Lua source. Each command resolves to a global function.      |
| `form.json`                  | Optional input form rendered when `run_mode: "form"`.         |
| `i18n/<locale>.json`         | Optional per-locale translations (see below).                 |

## Where plugins live

The Plugins workspace lists every folder under `<AppRoot>/plugins/`.
Plugins shipped with Formidable (e.g. `test-plugin`) are seeded from
the binary on first launch; see "Upgrades and re-seeding" below.

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
delete it from disk and restart. The bundled copy is written back.

To re-seed an entire bundled plugin, delete the whole folder under
`<AppRoot>/plugins/<id>/` and restart.

## Editing during development

The Plugins workspace ships an editor for every plugin file:

- **Manifest**: name, description, run mode, command list.
- **Lua Source**: `main.lua`, with Ctrl+Enter fullscreen.
- **Form Editor**: fields rendered when `run_mode: "form"`.
- **i18n**: per-locale key/value table; switch locale via the chips.

Save persists every dirty file in one atomic pass; the rest of the
app picks up changes on the next refresh.

## API clients in a plugin

A plugin can carry an API client and use it from its script. On the plugin's
**API clients** tab you name a client and give it an OpenAPI or Swagger
document, pasted or loaded from a file.

- **Display name** and **Base URL** are optional. Left empty, the client takes
  the title and the server the document declares.
- **Save to plugin** stores the document inside the plugin, so it travels in
  the plugin zip. Saving creates no client.
- **Create API client** makes the client on this machine. Importing the plugin
  zip on another machine does the same there. An existing client is never
  overwritten: delete it under Information > API clients to create it again.
- Credentials never travel with a plugin. Enter them under Information > API
  clients after the client is created.

The script reaches only the clients the plugin lists. The calls:

| Call | What it does |
|---|---|
| `formidable.client.list()` | The plugin's clients. |
| `formidable.client.operations(id)` | Every operation in the document, with its method, parameters and request body. |
| `formidable.client.call(id, operation, params, body, content_type)` | Runs one operation by its id, whatever its method. |
| `formidable.client.items(id, resource, opts)` and `fetch(id, resource, item_id, opts)` | Read through a resource binding. |
| `formidable.client.snapshot(item, keys)` | The value an api-client field stores for an item. |
| `formidable.client.exists(id)` and `create(id, opts)` | Let the script create the client from a document in the plugin folder. |

`call` sends the method the operation declares: GET, POST, PUT, PATCH, DELETE
and the rest. A table as `body` is sent as JSON, or as a form when that is all
the operation accepts. A string is sent exactly as given, so a file read with
`formidable.fs.read(path)` arrives byte for byte; pass its type as
`content_type` (for example `application/pdf`) unless the operation declares
it. Multipart uploads are not supported. A failed call raises an error that
starts with its code, such as `forbidden` or `timeout`.

`formidable.client.attempt` takes the same arguments and does not raise for a
failed call. It returns `result, nil` or `nil, err`, where `err` holds `code`,
`message`, `status`, `delivery`, and `body` and `data` with what the service
answered. `delivery` is `not_sent`, `answered` or `unknown`. Use it for a
write: the script can read why the service refused, and after a timeout it
knows the outcome is unknown, so it can ask the service before sending again.
A write is never sent twice by Formidable itself: a redirect on a writing
call is not followed and comes back as the error `redirected`.

## PDF from a plugin

`formidable.pdf.export(template, datafile, opts)` makes the PDF of one form,
the same export the Storage workspace offers, and returns `path`, `bytes` and
`duration_ms`. `opts` is optional and takes `output_path`, `style`,
`cover_template`, `disable_cover` and `disable_theme`.
`formidable.pdf.active()` says whether PDF export is set up.

PDF export needs Chrome or Chromium and is activated under Information > PDF.
When it is not set up, `export` raises an error starting with
`engine_inactive`. Switch on **Requires PDF export** under Behavior to have
the run refused up front in that case.

## Templates in a plugin

A plugin can hold templates of its own, so its script has somewhere to store
what it fetches. On the plugin's **Shipped templates** tab:

- **Add template** takes over a template from the Templates workspace, or
  starts a new one from YAML you type.
- **Save to plugin** stores the copy inside the plugin, so it travels in the
  plugin zip. Saving creates no template.
- **Create template** makes the template in the open context folder. Importing
  the plugin zip does the same, and so does running the plugin: its templates
  are created before the script starts. A template that already exists is
  never replaced, so your own changes to it are safe.
- Under **Templates** in the manifest, the templates a plugin holds are offered
  for binding even before they exist, so a plugin can be tied to the template
  it brings. Run it once from the Plugins workspace to create that template.
- **Take over current version** copies the template as it is now back into the
  plugin.
- **Replace with plugin's version** shows when the template in the context
  folder differs from the plugin's copy. It overwrites the template with the
  plugin's version after you confirm. This is the only way a template is
  replaced: an import, a run or a script never does it. Records are kept.

A script can do the same with `formidable.template.install(file)`. It returns
`true` when the template was created and `false` when one of that name was
already there. Only templates the plugin holds can be installed.

When a plugin is started from the Storage workspace, the script's `ctx` holds
`ctx.template` (the open template's file name) and `ctx.datafile` (the open
record's file name, when one is open), so it can work on "this record"
without asking which one.
