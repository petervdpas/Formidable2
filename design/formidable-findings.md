# Formidable - Findings From a Deep Read

> **Port status (Formidable2, 2026-05-07):** This document is a faithful snapshot of the **original Formidable**. The Formidable2 port has since diverged in some places. Most relevant here: the `code` and `latex` field types described below (sections referring to CodeMirror code execution, `run_mode` / `allow_run` / `input_mode` / `api_mode` / `api_pick`, the `window.CFA` CodeField API, `use_fenced` / `rows` on latex, etc.) have been **removed from Formidable2**. Treat those passages as historical context for the source app, not as the current Formidable2 design.

What Formidable actually is, what it does, and what about it I had wrong before. Sourced from a full read of `/home/peter/Projects/Formidable/{docs,controls,modules,schemas,plugins,main.js,preload.js,renderer.js,index.html}`. Citations are `path:line`.

## 1. What it is

**Formidable** is a desktop app for building and filling **schema-driven forms** - think "structured-data wiki authoring tool". You define **templates** (YAML files, ~20 field types) and create **forms** (form instances) that match those templates. Forms render as live UI in the app and as a browseable HTML wiki via an optional local HTTP server. Per `Formidable/docs/README.md:5-7`:

> Formidable is an Electron-based form management system built on an event-driven architecture with a powerful plugin system, schema-based templates, and dynamic form rendering capabilities.

Workflow: pick or create a template → create form instances → fill them in via the rendered UI → save as JSON to disk → optionally sync via Git or GiGot → optionally browse the resulting "wiki" in a local browser.

## 2. The data model - templates and forms

### Templates (the schema)

YAML files at `<context>/templates/<name>.yaml`. Defined in `Formidable/schemas/template.schema.js:5-14`:

```js
defaults: {
  name: "",                  // display name
  filename: "",              // "<name>.yaml"
  item_field: "",            // for collections - primary identifier field
  markdown_template: "",     // template string with {{key}} placeholders
  sidebar_expression: "",    // mini-expression for sidebar display
  enable_collection: false,  // single doc vs many instances
  fields: [],                // array of field definitions
}
```

### Field types - 20 of them

From `Formidable/schemas/field.schema.js:1-22`:

`guid · loopstart · loopstop · text · boolean · dropdown · multioption · radio · textarea · latex · number · range · date · list · table · image · link · tags · code · api`

Each has its own normalization in `field.schema.js:120-279`. Notable:

- **`code` fields run JavaScript in the renderer** (CodeMirror editor + run button). `field.schema.js:185-228` - `run_mode: manual|load|save`, `allow_run`, `input_mode: safe|raw`, `api_mode: frozen|raw`, `api_pick: []` whitelist.
- **`api` fields** reference rows from another template with `enable_collection: true`. `field.schema.js:243-275` - store either the selected ID string or `{id, ...mappedFields}` per the `map: [{key, path, mode}]` declaration. Modes: `static · editable · live-fill · live-edit`.
- **`loopstart` / `loopstop`** wrap repeating sections. The fields between them appear N times based on form data.
- **`latex`** uses CodeMirror with `use_fenced` and `rows`. Multiline default is forced to YAML block-scalar style.

### Forms (instances)

JSON files (not YAML) at `<context>/storage/<template-name>/<form>.meta.json`. Filename derived from `slugify(form.data[item_field])` with numeric collision suffix, falling back to GUID. See `Formidable/controls/apiCollections.js:25-38`.

Per template, the storage folder also has an `images/` subdir for image fields.

### Storage layout (under `context_folder`)

```
<context_folder>/
├── templates/
│   ├── basic.yaml
│   └── ...
├── storage/
│   └── <template-name>/
│       ├── <form>.meta.json
│       ├── ...
│       └── images/
├── .changes.log              ← append-only journal of FS mutations
├── .changes.cursor           ← per-backend sync cursor
├── .formidable/sync.json     ← gigot's client-side ledger (version + blob SHAs)
└── .gitignore                ← auto-patched to ignore *.log + .changes.*
```

The context folder is **portable** - defaults to `./examples` on first run (`Formidable/controls/setupManager.js:52`); `./` for in-project use.

## 3. The event-driven architecture (the heart of the app)

I underestimated this. The frontend isn't IPC-driven, it's **event-driven** - IPC is just one of the things events resolve to.

### EventBus

Renderer-side message broker at `Formidable/modules/eventBus.js`. Methods (per `Formidable/docs/EVENTBUS-SYSTEM.md`):

- `EventBus.on(event, handler)` - register listener
- `EventBus.emit(event, payload)` - fire-and-forget, parallel handlers
- `EventBus.emitWithResponse(event, payload)` - first handler wins, returns its value
- `EventBus.once / off` - one-shot / unregister

Naming: `namespace:action[:detail]` - `form:save`, `form:context:get`, `field:get-by-guid`, `template:list`, `vfs:reload`, `ui:toast`, `theme:toggle`, `boot:initialize`, etc.

### Handlers (renderer-side)

**30 handler modules** in `Formidable/modules/handlers/` (per `docs/HANDLER-PATTERN.md:14-43`):

```
cacheHandlers · codeFieldHandlers · collectionHandlers · configHandlers · contextHandlers ·
fieldHandlers · fileHandlers · formHandlers · gitHandlers · helpHandlers · historyHandlers ·
internalServerHandlers · loggingHandlers · markdownHandlers · modalHandlers · optionHandlers ·
pluginHandlers · profileHandlers · renderHandlers · screenHandlers · settingsHandlers ·
sidebarHandlers · systemHandlers · templateHandlers · themeHandlers · toastHandlers · uiHandlers
```

All registered in `Formidable/modules/eventRouter.js`. A handler may resolve fully in the renderer (e.g. `field:set-value`) or cross to main via `window.api.*` (e.g. `template:save` → `window.api.templates.saveTemplate`).

### Pollers

`Formidable/modules/pollers/` - periodic background tasks: `gigotQuickStatusPoller`, `gigotAutoSyncPoller`, `gitQuickStatusPoller`, `pendingChangesPoller`, `demoThemePulsePoller`. They emit events; UI updates via handlers.

### Three frontend global APIs (`renderer.js:79-88`)

- **`window.EventBus`** - direct bus access.
- **`window.FGA`** (Formidable Global API) - `form.*`, `context.*`, `util.*`, `plugin.*` etc.
- **`window.CFA`** (CodeField API) - used by code-type fields to manipulate other fields by GUID. All methods async, all routed through EventBus events: `field:get-by-guid`, `field:set-value`, etc.

### Field GUID system

Every rendered field gets a `data-field-guid` attribute (crypto.randomUUID). This solves the "two fields with the same key inside a loop" problem - code fields can target the precise instance by GUID rather than by key. See `Formidable/docs/FIELD-GUID-SYSTEM.md`.

## 4. The IPC bridge (preload → main)

The main process is *purely* a backend. The renderer is the orchestrator. `preload.js` exposes ~13 namespaces over Electron's `contextBridge`:

| Namespace | Backend module | Purpose |
|---|---|---|
| `window.api.encrypt` | `controls/encryption.js` | OS keyring crypto |
| `window.api.internalServer` | `controls/internalServer.js` | Start/stop the local HTTP server |
| `window.api.plugin` | `controls/pluginManager.js` | Load/list/run/CRUD plugins; declarative IPC |
| `window.api.help` | `controls/helpManager.js` | Static help topics |
| `window.api.git` | `controls/gitManager.js` | ~30 git ops |
| `window.api.gigot` | `controls/gigotManager.js` | **Remote** sync to a GiGot server |
| `window.api.journal` | `controls/changeJournal.js` | Pending changes / cursor |
| `window.api.config` | `controls/configManager.js` | Profiles, paths, virtual structure |
| `window.api.templates` | `controls/templateManager.js` | Template CRUD + validate + descriptor + item fields |
| `window.api.forms` | `controls/formManager.js` | Form CRUD + extended-list + image save |
| `window.api.csv` | `controls/csvManager.js` | CSV preview/import/write |
| `window.api.transform` | `controls/markdownRenderer.js`, `htmlRenderer.js`, `miniExprParser.js` | Render markdown/HTML, frontmatter, mini-expr |
| `window.api.system` | `controls/fileManager.js` + a few inlines in `ipcRegistry.js` | Filesystem, exec, fetch-remote |
| `window.api.dialog` | electron `dialog.*` | Choose dir/file/save |
| `window.electron.*` | electron primitives | shell open, app quit, devtools, window controls, clipboard, sfr (single-file repo) |

Plus **dynamic plugin IPC**: plugins declare `ipc: {handlerKey: fnName}` in `plugin.json`; preload fetches the map via `get-plugin-ipc-map` and binds `window.api.plugin.<PluginName>.<handlerKey>` at runtime. Routes are `plugin:<name>:<key>`.

The full master IPC table is `Formidable/controls/ipcRegistry.js:51-609` - ~150 routes total. A few I missed in my first pass:

- `system:open-external` with `variant: "tab"` opens a **new Electron BrowserWindow** loading the URL in-process (sandboxed, contextIsolation, no node integration). This is how the wiki view in the local HTTP server gets its own window. `ipcRegistry.js:88-131`.
- `proxy-fetch-remote` lives on `pluginManager.fetchRemoteContent` (not on fileManager). `ipcRegistry.js:142-153`.
- `seed-basic-template-if-empty` for first-run template seeding.

## 5. The local HTTP server - TWO purposes

`Formidable/controls/internalServer.js` (528 lines, default port 8383). User-toggleable via `enable_internal_server` config flag and the `internalServer` API namespace.

### A. Browseable wiki view

Read-only HTML pages for the templates and forms in the current context, intended to be opened in a regular browser tab (or in an in-app `system:open-external variant=tab` window):

- `GET /` - index of all templates with sidebar expression evaluations (`internalServer.js:188-223`)
- `GET /template/:name` - list of forms in that template, with sidebar expressions and tags (`:235-324`)
- `GET /template/:name/form/:filename` - rendered form (markdown-converted-to-HTML via `serverDataProvider.loadAndRenderForm`) (`:346-414`)
- `GET /template/:name/extended-list` - JSON variant of the form list (`:326-343`)
- `GET /storage/...` - serves images and files dynamically from the VFS storage path, with a diagnostic 404 page (`:97-145`)
- `GET /assets/...` - static UI assets (no-cache in dev, 30d in prod) (`:147-177`)
- `GET /favicon.ico` (`:183-185`)
- `GET /miniexpr` - mini-expression parser playground (`:417-472`)
- `GET /virtual` - JSON dump of VFS (dev only) (`:226-232`)
- `GET /debug/images` - image-config diagnostic page (`:42-95`)

### B. REST API for collections (with OpenAPI/Swagger UI)

Mounted by `Formidable/controls/apiCollections.js:209` (`mountApiCollections(app)`, 1698 lines). Provides programmatic CRUD over forms-as-collections:

| Method | Route | Purpose |
|---|---|---|
| GET | `/api/collections` | list collection-enabled templates |
| GET | `/api/collections/:t/count` | item count |
| GET | `/api/collections/design/:t` | template schema (used to build OpenAPI properties) |
| GET | `/api/collections/:t` | list items |
| GET | `/api/collections/:t/:id` | get one |
| POST | `/api/collections/:t` | create |
| PUT | `/api/collections/:t/:id` | replace |
| PATCH | `/api/collections/:t/:id` | partial update |
| PATCH | `/api/collections/:t/:id/field/:key` | single-field patch |
| DELETE | `/api/collections/:t/:id` | delete |
| POST | `/api/collections/:t/batch` | bulk ops |
| GET | `/api/collections/:t/export.ndjson` | NDJSON export |
| GET | `/api/collections/:t/export.csv` | CSV export |
| GET | `/api/openapi.json` | full OpenAPI spec generated from templates |

`/api/docs` serves Swagger UI (via `swagger-ui-express`).

OpenAPI properties are built by walking each template's `fields[]` and mapping each field type to a JSON Schema property (`apiCollections.js:54-207`). This is the source of truth for "what's a collection's API shape" - derived dynamically from the template.

### Why the user said "Formidable could use handlers"

This is the local API. The wiki view is read-only-HTML; the collections REST API is an honest programmatic surface. Both run on `127.0.0.1:8383`. A Wails port that drops this would be losing real functionality.

## 6. GiGot - remote sync, NOT local-only sync (correction)

I had this wrong. `Formidable/controls/gigotManager.js` opens with:

> Backend for the GiGot remote-sync option. Sibling of gitManager.js. **Talks to a GiGot server over HTTP using a subscription token.** Stateless; caller passes a conn object built from the profile config.

Endpoints (`gigotManager.js:15-27`):

```
GET  /api/health
GET  /api/me
GET  /api/repos/{repo}/context
GET  /api/repos/{repo}/formidable
GET  /api/repos/{repo}/head
GET  /api/repos/{repo}/tree
GET  /api/repos/{repo}/files/{filePath}
GET  /api/repos/{repo}/log
POST /api/repos/{repo}/commits
GET  /api/repos/{repo}/destinations
POST /api/repos/{repo}/destinations/{destId}/sync
```

Authenticated with bearer token. Server-side concept of "destinations" lets the GiGot server mirror a repo to additional targets (likely GitHub etc.).

**The `-Local` suffix on `pushLocal/pullLocal/syncLocal` refers to the LOCAL-FILESYSTEM side of the operation, not "local-only sync".** GiGot syncs the local context folder ↔ a remote GiGot server.

Smart bits:
- **Git blob SHA1 for cheap diffing** - `gigotManager.js:87-93` computes `SHA1("blob "+len+"\0"+content)`, the same hash git uses. Lets the client compare local bytes to remote tree entries without downloading.
- **Client-side ledger** at `<context>/.formidable/sync.json` - `{version, lastSync, files: {path: blobSha}}`. Lets steady-state sync skip the `/tree` fetch (just `/head` + `/commits`).
- **Pull is wholesale-overwrite** (`gigotManager.js:630-726`): server is authoritative, ledger is rebuilt from the new tree.
- **Sync = push then pull** (`gigotManager.js:731-766`); on push 409 (conflict), skips pull deliberately so the user can resolve.

The `-Local` semantics are still local-first in spirit: Formidable **works fine without GiGot**. Remote sync is opt-in (`remote_backend` config flag).

## 7. The change journal

`Formidable/controls/changeJournal.js` (321 lines).

**Append-only log** at `<context>/.changes.log` - JSON-per-line entries: `{ts, op: create|update|delete|sync|baseline, path, ...meta}`. Tracks only paths under `templates/` and `storage/`.

**Cursor file** at `<context>/.changes.cursor` - per-backend `{ts, version}`. `recordSync()` advances both ts (via the journal sync marker) and version. `recordRemoteSeen()` is for pulls - only updates `version`, not `ts` (because pull is inbound, not outbound).

**`pending()`** computes pending pre-sync changes since the last sync marker for the active backend, dedupes by path with the latest op winning.

**Why it exists**: lets pollers (`pendingChangesPoller`) badge "you have N unpushed changes" without scanning the FS or asking git/gigot every tick. Architectural rule (`gigotManager.js:570-581`): "the journal cursor only advances from one place per backend" - `pushLocal()` is the funnel for gigot, `git.commit/push` for git.

`changeJournal.init()` writes `baseline` entries on first run for existing files so they're not later mistaken for "new since last sync".

## 8. Plugin system

`Formidable/controls/pluginManager.js`. Each plugin is a folder under `<appRoot>/plugins/`:

```
plugins/<name>/
├── plugin.json     (manifest)
├── plugin.js       (code; optional for frontend-only)
├── settings.json   (optional, persisted plugin settings)
└── i18n/           (optional, plugin translations)
```

Targets (`pluginManager.js:31-91`, `schemas/plugin.schema.js`):

- **`backend`** - `plugin.js` is `require()`-d in main process. Has `run(context)` and named handlers exposed via `ipc: {key: fnName}` map → automatically registered as `plugin:<name>:<key>` IPC routes.
- **`frontend`** - ESM module, no `run()` required at backend (loaded later in renderer via `FGA.plugin.*`).
- **`both`** - split backend/frontend.

Active plugins (`Formidable/plugins/`):
- **BackTest** - backend demo, has `echoHandler` IPC (`plugins/BackTest/plugin.js`).
- **PandocPrint** - frontend, calls `executeCommand` IPC to shell out to `pandoc` for PDF generation (`plugins/PandocPrint/plugin.js`, 278 lines).
- **WikiWonder** - likely wiki integration (didn't read).

**Plugin loading is dynamic** - `pluginManager.reloadPlugins()` clears the require cache and re-registers handlers. The renderer's `bindPluginIpcMethods()` (in `preload.js:44-60`) re-fetches `getPluginIpcMap()` and rebinds `window.api.plugin.<name>.<method>`.

This is the **biggest porting challenge** - Wails has no `require()` and Go can't run Node modules. Out of scope for now per user direction.

## 9. Boot flow (electron main + renderer)

### Main process (`Formidable/main.js`)

1. Windows-only: portable `userData` at `process.cwd()/user-data` (`main.js:18-21`)
2. App ready:
   - Set `appRoot` to bundled dir (`main.js:134-136`)
   - First-run setup: copy bundled `examples/` to user dir on Linux/Mac (`setupManager.runSetup`, `main.js:139`). User-data dir is `~/.local/share/Formidable/` on Linux/Mac when packaged (`setupManager.js:14-21`).
   - Reset `appRoot` to user-writable dir if applicable (`main.js:142-144`)
   - `configManager.initialize()` → load `config/boot.json` + `config/user.json`
   - Set spell-checker language from config (`main.js:153-161`)
   - `pluginManager.loadPlugins()` (`main.js:163`)
   - `registerIpcHandlers()` (`main.js:164`)
   - `templateManager.ensureTemplateDirectory()` + `seedBasicTemplateIfEmpty()` (`main.js:168-169`)
   - `nodeLogger.setLoggingEnabled / setWriteEnabled` from config
   - `createWindow()` - re-reads config for fresh window bounds, applies `nativeTheme`, hides until `ready-to-show`, sets versioned title after load, persists bounds on resize/move (debounced 150ms) (`main.js:76-127`)
3. `screen.on("display-added"/"display-removed"/"display-metrics-changed", applySafeBounds)` - re-clamp window bounds when displays change (`main.js:182-194`)

### Renderer (`Formidable/renderer.js`)

1. DOM ready
2. `getAppInfo()` IPC → set window title (`renderer.js:58-67`)
3. `initEventRouter()` - register all 100+ event handlers (`renderer.js:80`)
4. `exposeGlobalAPI()` + `exposeCodeFieldAPI()` - populate `window.FGA` and `window.CFA` (`renderer.js:86-88`)
5. Load config via `EventBus.emit("config:load", cb)` (callback-style!) (`renderer.js:91-93`)
6. **Three themes**: `light · dark · purplish` (I had two before). Apply to `<html dataset.theme>`, classlist, and toggle stylesheet `disabled` flags (`renderer.js:95-110`)
7. `loadLocale(config.language || "en")` then `translateDOM()`
8. Apply visibility flags via events (e.g. `screen:paste:visibility`)
9. `buildMenu("app-menu", handleMenuAction)` - custom HTML menu, not native Electron menu (Electron menu is set to null in `main.js:107`)
10. `initStatusHandler` + status bar buttons
11. Setup all modals: profile, settings, workspace, entry, template, git, gigot-sync, plugin, help, about, csv-import, csv-export
12. Build sidebar managers (`createTemplateListManager`, `createStorageListManager`)
13. Build template selector / dropdown
14. `bindContextDependencies / bindTemplateDependencies / bindFormDependencies / bindListDependencies / bindLinkDependencies` - pass UI refs into handler modules
15. Initial data load: template options, list, dropdown
16. Final kick: `EventBus.emit("boot:initialize", config)` - context handler picks this up to switch between template/storage view per `config.context_mode`

## 10. Configuration

Two files in `<appRoot>/config/`:

- **`boot.json`** - boot-level (theme, language, basic prefs). Defaults in `schemas/boot.schema.js`.
- **`user.json`** - user profile (currently active). All UI prefs, sidebar widths, current selections, encryption key, git settings, gigot settings, internal-server toggle/port, window bounds.

`schemas/config.schema.js` defaults (paraphrased from `:3-53`):

```js
{
  profile_name, theme: "light", show_icon_buttons, show_paste_buttons,
  use_expressions, show_meta_section, loop_state_collapsed, field_state_collapsed,
  font_size: 14, development_enable, logging_enabled, enable_plugins,
  context_mode: "template",      // "template" | "storage"
  context_folder: "./",          // PORTABLE
  selected_template, selected_data_file,
  author_name, author_email, language: "en",
  encryption_key,
  use_git, git_root, git_branch,
  remote_backend: "none",        // "none" | "git" | "gigot"
  gigot_base_url, gigot_repo_name, gigot_token,
  enable_internal_server: false, internal_server_port: 8383,
  window_bounds: {width, height, x?, y?, maximized?},
  template_sidebar_width: 300, storage_sidebar_width: 300,
  status_buttons: { reloader, charpicker, gitquick, gigotload },
  history: { enabled, persist, max_size: 20, stack: [], index: -1 }
}
```

**Profiles**: each profile is a separate `user.json`-shaped file. `switchUserProfile(filename)` swaps which is active. Export/import to file. (`ipcRegistry.js:446-502`).

### Virtual File System (VFS)

Built by `configManager.getVirtualStructure()`. Maps the disk layout into `{context, templates, storage, templateStorageFolders: {[name]: {filename, path, metaFiles, imageFiles}}}`. Cached in memory with a TTL; `dirtyVirtualStructure()` invalidates after any FS mutation. See `Formidable/docs/VFS-SYSTEM.md`.

## 11. Things I had wrong

| Earlier assumption | Reality |
|---|---|
| Forms are `.yaml` | Forms are `.meta.json` (only templates are YAML) |
| GiGot is local-only sync | GiGot is **remote sync** to a GiGot server over HTTP with bearer auth |
| Internal server is optional bloat | Internal server is two real local APIs: a **wiki view** + a **REST collections API with OpenAPI**, opened in-app via `system:open-external variant=tab` for a separate window |
| 2 themes | 3 themes: `light · dark · purplish` |
| Frontend = thin shell over IPC | Frontend is **event-driven** - EventBus is the primary abstraction; IPC is one of the things events resolve to (renderer-side handlers may stay in renderer or call `window.api.*`) |
| Plugin IPC = static | Plugin IPC is **dynamic** - preload fetches `getPluginIpcMap()` and rebinds at runtime; reloads clear require cache |
| `fileManager` exposes everything | A few system IPC calls live in `pluginManager` (`fetchRemoteContent`) and inline in `ipcRegistry.js` (`exec`, `system:open-external`, dialogs) |

## 12. Implications for the Wails port

The migration story changes shape in a few important ways:

### A. EventBus must come over to the renderer untouched

The Wails port's frontend keeps `modules/eventBus.js`, `modules/eventRouter.js`, the 30 handler modules, and the FGA/CFA globals. **What changes is only what each handler does for IPC** - `window.api.*` calls become bindings imports (e.g. `import { Service as Templates } from ".../bindings/.../templates"`). Renderer-side handlers (cache, field, modal, status, theme, history, screen, ui, toast, render, markdown, codeField) stay 100% client.

### B. Wails services map to backend handlers, not to the whole IPC table

Several IPC routes today are renderer-resolvable and should NOT round-trip to Go: clipboard, window controls, dialogs (Wails has built-ins), shell open. The actual Wails `service.go` files only need the genuinely-backend ones - config, templates, forms, system FS, encrypt, csv, transform, git, gigot, journal, plugin (deferred), internalServer-control, help, sfr.

### C. The "handlers" debate resolves cleanly

The user wanted handlers (HTTP) for the local API. The local API is **already a real thing in Formidable** - wiki view + REST collections + OpenAPI. So the Wails port:

- **Keeps a `internal/server` HTTP server** (loopback `127.0.0.1:8383`).
- **Keeps the wiki routes** (`/`, `/template/...`, `/storage/...`, `/assets/...`, `/api/docs`).
- **Keeps the REST collections API** (`/api/collections/...` + `/api/openapi.json`).
- **Modules opt into HTTP handlers** only where it makes sense for the wiki/REST surface - `templates`, `forms`, `csv` (export endpoints), `transform` (markdown/HTML for wiki rendering). NOT `system`, `sfr`, `encrypt`, `git`, `gigot`, `config`, `plugin`.

So my earlier four-layer architecture is essentially right, just narrower in handler scope.

### D. Form storage format

Forms are JSON, not YAML. Templates are YAML. The Go side needs:
- `gopkg.in/yaml.v3` for templates
- `encoding/json` for forms
- A mini-expression parser port (`controls/miniExprParser.js` is 304 lines - must be ported)
- A Handlebars-ish renderer for `markdown_template` (or leave that to JS frontend; only the wiki view needs it server-side)

### E. The "context_folder" is the heart of everything

Almost every backend module reads paths from config's `context_folder` then `templates/`/`storage/<template>/...`. The `config` module is genuinely the foundation - F-102 ordering was right.

### F. Plugin runtime is a real fork-in-the-road

The current plugin system runs Node.js modules in the main process AND ESM modules in the renderer. For Wails:
- Frontend plugins (PandocPrint-style) port over essentially as-is - they're just ESM that uses `FGA.*` APIs.
- Backend plugins (BackTest-style with `require('https')` etc.) cannot run in Go. Options at decision time: `goja` (JS interpreter in Go), `gopher-lua` (different language; breaks compatibility with existing plugins), spawn a Node child process, or sandboxed iframe + postMessage.
- Deferred per user. But the choice should be made before scoping Epic X seriously - it determines whether the existing two backend plugins survive the port.

### G. GiGot is a real network client

Epic 7 isn't a spike - it's a clear porting job. The Go side reimplements the HTTP client against the GiGot REST API: `/api/me`, `/api/repos/.../...`. Use `crypto/sha1` for git blob hashing; `net/http` for the client; reuse the same `.formidable/sync.json` ledger format so existing users' state continues to work.

### H. CSP and Wails

`index.html:6-15` has a strict CSP that allows `http://localhost:* http://127.0.0.1:* https://localhost:* ws://localhost:*` - required for the internal server URLs to load images/fetch from. Wails injects its own asset-server URL; we need to add it to the CSP, or rely on Wails' built-in CSP overrides.

## 13. What this means for the backlog

A few stories need rework. I'll fold these corrections into `backlog.md`:

- **F-102 (config)** - add `enable_internal_server`, `internal_server_port`, `gigot_*`, `remote_backend`, `status_buttons`, `history` to the config domain; make sure `dirtyVirtualStructure` semantics are preserved.
- **F-302 (forms)** - clarify forms are JSON `.meta.json`, not YAML.
- **Epic 4 (transform)** - split into frontmatter/mini-expr (cross-process needed for wiki render) vs full markdown render (only needed by internal server's wiki view).
- **Epic 7 (gigot)** - drop "spike" framing. It's a known shape. Re-scope as concrete porting story with the HTTP client + ledger format.
- **Epic 8 (internal server)** - cannot be DEFERRED. It's a first-class part of Formidable. Keep at full scope: wiki routes + REST collections API + Swagger UI + dynamic OpenAPI from templates.
- **Epic 2 (frontend port)** - add a story for porting `eventBus.js` + `eventRouter.js` + the 30 handler modules verbatim. Adjust IPC call sites only.
- **Epic 9 (packaging)** - add Linux user-data dir story (`~/.local/share/Formidable2/`) and the first-run examples-copy.

## 14. Single biggest realization

Formidable is **not "an Electron app with templates and forms"**. It's an **event-driven editor + a local wiki/API server** for schema-driven content, with optional git/remote sync. The Wails port is preserving the *event-driven editor* shape on the frontend, and porting the *file system + sync + local server* shape to Go. The two halves stay split exactly the way Electron split them; only the language under each half changes.
