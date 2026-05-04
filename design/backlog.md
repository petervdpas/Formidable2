# Backlog — Formidable2

DevOps-style backlog for the Electron → Wails 3 migration.
See [architecture.md](architecture.md), [migration-plan.md](migration-plan.md), [ipc-mapping.md](ipc-mapping.md), and [formidable-findings.md](formidable-findings.md) for design context.

## Conventions

**Story ID** — `F-<NNN>`, grouped by epic (1xx = Epic 1, 2xx = Epic 2, …).
**Status** — `TODO` · `IN PROGRESS` · `BLOCKED` · `IN REVIEW` · `DONE` · `DEFERRED`
**Size** — `S` (≤ half day) · `M` (1–2 days) · `L` (3–5 days) · `XL` (split it).

### Definition of Done (per story)

- [ ] Code merged and `go build ./...` (root pkg) is green.
- [ ] `wails3 dev` boots and feature smoke-tested in the window.
- [ ] Domain layer has tests where logic is non-trivial (`t.TempDir()` for FS).
- [ ] Bindings regenerate cleanly (`wails3 generate bindings`).
- [ ] Frontend consumer updated; old `window.api.*` references removed for this module.
- [ ] Backlog story updated with status + linked PR/commit.

---

## Epic 0 — Foundation

| ID | Title | Size | Status |
|---|---|---|---|
| F-001 | Scaffold Wails 3 vanilla-ts project | S | DONE |
| F-002 | Pin Vite to IPv4 (`server.host: 127.0.0.1`) | S | DONE |
| F-003 | Move greet demo into `internal/services/greet` | S | DONE |
| F-004 | Save context to memory (Formidable→Wails3 port) | S | DONE |
| F-005 | Author design docs (architecture, migration-plan, ipc-mapping, backlog) | S | DONE |
| F-006 | Deep-read of Formidable + findings doc + source-docs copy | M | DONE |
| F-007 | Git init + first commit + push to private GitHub remote (user) | S | DONE |
| F-008 | Remove greet demo module + frontend wiring | S | DONE |

---

## Epic 1 — Core Plumbing

Goal: every module can be built on top of `system`, `config`, `sfr`, and `slog`.

### F-101 — `system` module skeleton  [size: M] [DONE]
**As a** developer, **I want** a `system` module that wraps filesystem operations, **so that** other modules can depend on a clean FS interface instead of `os` directly.

**Acceptance**
- [ ] `internal/modules/system/{domain,service,types}.go` created (no `handlers.go` — Wails-only).
- [ ] Methods cover the old `api.system.*`: `loadFile`, `saveFile`, `deleteFile`, `ensureDirectory`, `emptyFolder`, `copyFolder`, `copyFile`, `fileExists`, `resolvePath`, `getAppRoot`, `openExternal`, `executeCommand`, `proxyFetchRemote`.
- [ ] `FS` interface exposed for other modules to depend on.
- [ ] `domain_test.go` covers happy-path + error cases for file ops using `t.TempDir()`.
- [ ] Service registered in `internal/app/app.go`; binding regenerates as `system.Service`.

### F-102 — `config` module skeleton  [size: L] [TODO]
**As a** developer, **I want** a `config` module that mirrors the old `configManager.js`, **so that** profile and path resolution is centralized.

**Acceptance**
- [ ] Module folder, Wails-only (no `handlers.go`).
- [ ] Config struct mirrors `Formidable/schemas/config.schema.js` exactly: profile_name, theme, language, context_mode, context_folder, selected_template, selected_data_file, author_*, encryption_key, use_git, git_*, remote_backend, gigot_*, enable_internal_server, internal_server_port, window_bounds, *_sidebar_width, status_buttons, history, plus all the UI flags.
- [ ] Three themes supported: `light`, `dark`, `purplish`.
- [ ] Boot config (`boot.json`) preserved separately from user config (`user.json`).
- [ ] Methods mirror `api.config.*`: `loadUserConfig`, `updateUserConfig`, `invalidateConfigCache`, `dirtyVirtualStructure` (called after FS mutations), `getVirtualStructure`, `getContextPath`, `getTemplatesFolder`, `getStorageFolder`, `getTemplateStorageInfo`, `getTemplateStorageFolder`, `getTemplateMetaFiles`, `getTemplateImageFiles`, `getSingleTemplateEntry`, `switchUserProfile`, `listUserProfiles`, `currentProfileFilename`, `exportUserProfile`, `importUserProfile`, `deleteUserProfile`.
- [ ] Virtual structure TTL cache preserved (default 2000ms; invalidated by `dirtyVirtualStructure`).
- [ ] `ConfigReader` interface exposed for downstream modules.
- [ ] Tests for load/save/cache invalidation, profile switching, virtual-structure rebuild.

### F-103 — `sfr` module skeleton  [size: S] [TODO]
**As a** developer, **I want** the storage-from-relative helpers, **so that** the frontend can read/write within profile-relative paths.

**Acceptance**
- [ ] Module folder, Wails-only (no HTTP).
- [ ] Methods: `listFiles`, `loadFromBase`, `saveFromBase`, `deleteFromBase`.
- [ ] Path traversal protection (no `..` escape outside base).
- [ ] Tests cover normal + traversal-attempt paths.

### F-104 — Structured logging  [size: S] [TODO]
**As a** developer, **I want** `log/slog` writing JSON to `formidable.log` in the same folder as the old app, **so that** I keep observability continuity.

**Acceptance**
- [ ] `internal/log` (or part of `internal/app`) initialises slog with rotation-friendly file output.
- [ ] Log level configurable from env (`FORMIDABLE_LOG_LEVEL`).
- [ ] `nodeLogger.js` references in ported frontend swap to `console.*` or a thin `log` binding.

### F-105 — Composition root scaffold  [size: M] [DONE]
**As a** developer, **I want** `internal/app/{app,services,routes}.go` in place, **so that** new modules drop into a single registration list.

**Acceptance**
- [ ] `app.New(deps) *App` constructor.
- [ ] `WailsServices() []application.Service`.
- [ ] `RegisterRoutes(mux)` callable but no-op at first.
- [ ] `main.go` slims to ~30 lines: build app, hand services to Wails, run.

### F-106 — Wire built-ins  [size: S] [TODO]
**As a** frontend dev, **I want** dialog/clipboard/window controls/shell open accessible from `main.ts`, **so that** I don't need a Go module for trivial Wails APIs.

**Acceptance**
- [ ] `frontend/src/lib/builtins.ts` exports thin wrappers around `application.OpenFileDialog`, clipboard read/write, window reload/min/max/close, shell open external/path, devtools toggle, app quit.
- [ ] All `window.electron.{shell,app,devtools,window,clipboard}` call sites in ported code use these wrappers.

---

## Epic 2 — Frontend Shell

### F-201 — Move static assets + index.html  [size: M] [TODO]
- Move `assets/`, CSS, images, fonts, `i18n/`, `examples/templates/` into `frontend/src/` and `frontend/public/`.
- Port `index.html` to Vite's `frontend/index.html` (preserve all modal containers, status bar, sidebar splits).
- Update CSP to allow Wails asset-server URL plus `127.0.0.1:*` for the local server.
- Verify CodeMirror, EasyMDE, Sortable, Font Awesome assets load.

### F-202 — Theme bootstrap  [size: S] [TODO]
- Port the early-theme block from `preload.js` to the top of `frontend/src/main.ts`.
- **Three themes**: `light`, `dark`, `purplish`. Maintain `theme-<name>` class semantics + the linked-stylesheet `disabled` toggle.

### F-203 — Window state persistence  [size: S] [TODO]
- Read `window_bounds` from config on startup; apply via `WebviewWindowOptions`.
- Debounced (150ms) save on resize/move/unmaximize, matching `main.js:34-54`.
- Save final bounds on close.
- Re-clamp via screen change events.

### F-204 — Custom HTML menu  [size: M] [TODO]
- Port `modules/menuManager.js` and `modules/handlers/*` for menu actions verbatim. Menu is built in DOM from i18n strings, not Wails `application.Menu`.
- Native menu disabled (matches Electron's `Menu.setApplicationMenu(null)` in `main.js:107`).

### F-205 — Port EventBus + handlers + global APIs  [size: L] [TODO]
- Copy `modules/eventBus.js`, `modules/eventRouter.js`, `modules/globalAPI.js`, `modules/codeFieldAPI.js` to `frontend/src/modules/` essentially verbatim.
- Copy all 30 files in `modules/handlers/` to `frontend/src/modules/handlers/`.
- Copy `modules/pollers/` to `frontend/src/modules/pollers/`.
- Replace `window.api.*` call sites with binding imports as each backend module lands.

### F-206 — Frontend utilities  [size: M] [TODO]
- Move `utils/*.js` (60+ files) into `frontend/src/utils/`.
- These are renderer-only — no IPC dependencies.
- Move `controls/calculator.js`, `expressionHelpers.js`, `miniExprParser.js`, `htmlRenderer.js`, `markdownRenderer.js`, `pageGenerator.js`, `apiClient.js` here too if they have no Node dependencies (most don't, but markdown renderer uses `markdown-it` which is fine in browser).

### F-207 — UI modules  [size: L] [TODO]
- Move `modules/*.js` (formRenderer, formActions, formUI, sidebarManager, settingsManager, templateEditor, statusButtons, contextManager, etc.) into `frontend/src/modules/`.
- Wire dependencies via the same `bind*Dependencies` pattern from `renderer.js:258-282`.

### F-208 — Boot order  [size: S] [TODO]
- Port `renderer.js` boot sequence to `frontend/src/main.ts`. Order:
  1. DOM ready
  2. App info via Wails (replaces `getAppInfo` IPC)
  3. `initEventRouter()`
  4. `exposeGlobalAPI()` + `exposeCodeFieldAPI()`
  5. Config load via EventBus
  6. Theme + i18n
  7. Menu + status bar
  8. Modal setup
  9. Sidebar managers + template selector
  10. Bind handler dependencies
  11. Initial data load
  12. `EventBus.emit("boot:initialize", config)`

---

## Epic 3 — Template & Storage

### F-301 — `template` module  [size: L] [TODO]
- Mirrors `controls/templateManager.js` (526 lines).
- Owns `<context>/templates/<name>.yaml` files.
- Methods (frontend-visible names match old `api.templates.*`): `ListTemplates`, `LoadTemplate`, `SaveTemplate`, `DeleteTemplate`, `ValidateTemplate`, `GetTemplateDescriptor`, `GetItemFields`, `SeedBasicIfEmpty`, `EnsureTemplateDirectory`.
- Schema validation matches `Formidable/schemas/template.schema.js` + `field.schema.js` (20 field types, type-specific normalization for code/latex/api/textarea, etc.).
- HTTP routes for read paths.
- Depends on F-101 (system), F-102 (config).

### F-302 — `storage` module  [size: L] [TODO]
- Mirrors `controls/formManager.js`.
- Owns `<context>/storage/<template-name>/` — form `.meta.json` files + `images/` subfolder.
- **Storage format**: JSON `.meta.json` (NOT YAML). Filename derived from `slugify(data[item_field])` with numeric collision suffix; falls back to GUID.
- Methods (frontend-visible names match old `api.forms.*`): `EnsureFormDir`, `ListForms`, `ExtendedListForms`, `LoadForm`, `SaveForm`, `SaveImageFile`, `DeleteForm`.
- HTTP routes for read + create.
- Every mutation calls `config.dirtyVirtualStructure()` to invalidate the VFS cache.
- Depends on F-101, F-102, F-301.

### F-303 — Frontend wire-up: editor flow  [size: M] [TODO]
- Switch the template editor and form renderer to use new `template.Service` + `storage.Service` bindings.
- End-to-end smoke test: open a template, render a form, save a form, reload, verify content.

---

## Epic 4 — Transform

### F-401 — `transform` module — frontmatter & mini-expr  [size: M] [TODO]
- Methods: `parseFrontmatter`, `buildFrontmatter`, `filterFrontmatter`, `parseMiniExpr`.
- Decision: keep markdown/HTML rendering JS-side; only frontmatter and mini-expr cross to Go.
- Depends on F-101.

### F-402 — Frontend rendering kept in JS  [size: S] [TODO]
- Confirm `markdown-it`, `handlebars`, `highlight.js`, `easymde`, `codemirror`, `expr-eval`, `gray-matter` are imported as npm deps in `frontend/package.json`.
- No Wails crossing for `renderMarkdownTemplate` / `renderHtmlPreview` — done in renderer.

### F-403 — Decision review (Go vs JS rendering)  [size: S] [TODO]
- After 4-401/4-402 land, evaluate latency and fidelity. If JS rendering is fine, close the option to port; otherwise spike a Go rewrite.

---

## Epic 5 — Smaller Services

### F-501 — `csv` module  [size: M] [TODO]
- Methods: `csvPreview`, `csvImportRow`, `csvWrite`. Use `encoding/csv`.
- HTTP routes.

### F-502 — `encrypt` module  [size: M] [TODO]
- Methods: `encrypt`, `decrypt`, `encryptionAvailable`.
- OS keyring for key material via `zalando/go-keyring` (already a Wails CLI transitive dep).
- Wails-only (no HTTP).

### F-503 — `help` module  [size: S] [TODO]
- Methods: `listHelpTopics`, `getHelpTopic`. Read from embedded `help/` markdown.
- HTTP routes.

### F-504 — `journal` module  [size: S] [TODO]
- Methods: `journalPending`, `journalCursor`. Mirrors `controls/changeJournal.js`.
- HTTP routes.

---

## Epic 6 — Git

### F-601 — `git` module — read paths  [size: L] [TODO]
- `isGitRepo`, `getGitRoot`, `gitStatus`, `gitStatusFresh`, `gitRemoteInfo`, `gitBranches`, `gitDiffNameOnly`, `gitDiffFile`, `gitLog`, `gitConflicts`, `gitProgressState`.
- Pure `go-git/v5`.

### F-602 — `git` module — write paths  [size: L] [TODO]
- `gitPull`, `gitPush`, `gitCommit`, `gitDiscard`, `gitFetch`, `gitSetUpstream`, `gitAddAll`, `gitAddPaths`, `gitResetPaths`, `gitCommitPaths`, `gitCheckout`, `gitResetHard`, `gitRevert`.
- `go-git/v5` where reasonable.

### F-603 — `git` module — merge/rebase  [size: L] [TODO]
- `gitMerge`, `gitMergeAbort`, `gitMergeContinue`, `gitRebaseStart`, `gitRebaseContinue`, `gitRebaseAbort`, `gitChooseOurs`, `gitChooseTheirs`, `gitMarkResolved`, `gitRevertResolution`, `gitContinueAny`, `gitSync`.
- **Shell out to `git` binary** — go-git's merge/rebase support is too thin.

### F-604 — `git` module — tooling integration  [size: S] [TODO]
- `gitMergetool`, `gitOpenInVscode`. Pure shell-outs.

### F-605 — Frontend wire-up: git control modal  [size: M] [TODO]
- Switch `gitControlModal.js` and `controls/gitManager.js` consumers to new bindings.

---

## Epic 7 — Gigot (remote sync HTTP client)

GiGot is a **remote** HTTP service Formidable optionally syncs to. The Wails port reimplements the existing `controls/gigotManager.js` HTTP client in Go. No spike needed — endpoints + ledger format are documented.

### F-701 — `gigot` HTTP client + ledger  [size: L] [TODO]
- `internal/modules/gigot/client.go` — bearer-auth `net/http` client matching the routes in `Formidable/controls/gigotManager.js:15-27`: `/api/health`, `/api/me`, `/api/repos/{repo}/{context|formidable|head|tree|files|commits|destinations|...}`, `/api/repos/{repo}/destinations/{id}/sync`.
- `internal/modules/gigot/blob.go` — `gitBlobSha1(buf) string` — SHA1("blob "+len+"\0"+content), exact same hash git uses for tree entries.
- `internal/modules/gigot/ledger.go` — read/write `<context>/.formidable/sync.json` with `{version, lastSync, files: {path: blobSha}}`. Atomic via tmp+rename.
- Tracks `lastKnownLoad` from response header `X-GiGot-Load: low|medium|high`.

### F-702 — `gigot` module wiring  [size: M] [BLOCKED on F-701]
- Methods from `api.gigot.*`: `gigotPing`, `gigotMe`, `gigotContext`, `gigotFormidable`, `gigotHead`, `gigotListDestinations`, `gigotSyncDestination`, `gigotPushLocal`, `gigotPullLocal`, `gigotSyncLocal`, `gigotLog`, `gigotLastKnownLoad`.
- `pushLocal` orchestration: walk `templates/`+`storage/`+root allowlist → diff against ledger → fetch `/head` for parent_version → POST `/api/repos/{repo}/commits` → reconcile ledger from server's `changes[]`.
- `pullLocal` orchestration: GET `/tree` → apply server-side deletes → fetch any blobs whose SHA differs from local → write atomically (tmp+rename) → rebuild ledger from tree.
- `sync = pushLocal then pullLocal`; on push 409 skip pull and surface conflict.
- Each successful push emits `journal.recordSync({backend: "gigot", version, pushed})`. Each successful pull emits `journal.recordRemoteSeen("gigot", version)`.
- Wails-only (no HTTP handlers — sync transport is internal).

### F-703 — Frontend wire-up: gigot sync modal  [size: M] [BLOCKED on F-702]
- `modules/gigotSyncModal.js` and `modules/handlers/gigotHandler.js` ported with binding swaps.
- `modules/pollers/gigotQuickStatusPoller.js` and `gigotAutoSyncPoller.js` keep working unchanged (they emit events; the events resolve to bindings now).

---

## Epic 8 — Local HTTP Server (loopback `127.0.0.1:8383`)

This is **not deferrable** — Formidable's local server is a real product surface (wiki view + REST collections API + OpenAPI), not legacy bloat.

### F-801 — `internal/server` runtime  [size: M] [TODO]
- `net/http` server with graceful shutdown.
- Bind address forced to `127.0.0.1` (loopback only).
- Reads enabled flag + port from config (`enable_internal_server`, `internal_server_port`, default 8383).
- Tracks open sockets, destroys them on shutdown (matches `Formidable/controls/internalServer.js:482-515`).

### F-802 — Wiki view routes  [size: M] [BLOCKED on F-801]
- `GET /` — index of templates with sidebar-expression evaluations.
- `GET /template/:name` — list of forms in a template (with sidebar expressions and tags).
- `GET /template/:name/form/:filename` — rendered form (markdown → HTML).
- `GET /template/:name/extended-list` — JSON variant.
- `GET /storage/...` — image/file passthrough from VFS storage path with diagnostic 404 page.
- `GET /assets/...` — static UI assets (no-cache in dev, 30d in prod).
- `GET /favicon.ico`.
- `GET /miniexpr` — mini-expression playground.
- `GET /virtual` — VFS dump (dev only).
- `GET /debug/images` — image config diagnostic.
- Page rendering via a Go port of `controls/pageGenerator.js` (Handlebars-ish template wrapping).

### F-803 — REST collections API  [size: L] [BLOCKED on F-801, F-301, F-302]
- All routes from `Formidable/controls/apiCollections.js`:
  - `GET /api/collections` (list collection-enabled templates)
  - `GET /api/collections/:t/count`
  - `GET /api/collections/design/:t`
  - `GET /api/collections/:t` / `GET /api/collections/:t/:id`
  - `POST /api/collections/:t`
  - `PUT /api/collections/:t/:id`
  - `PATCH /api/collections/:t/:id`
  - `PATCH /api/collections/:t/:id/field/:key`
  - `DELETE /api/collections/:t/:id`
  - `POST /api/collections/:t/batch`
  - `GET /api/collections/:t/export.{ndjson,csv}`
- Filename derivation logic for new items matches `apiCollections.js:25-38` (slugify on `item_field` value, numeric suffix, GUID fallback).

### F-804 — OpenAPI generation  [size: M] [BLOCKED on F-803]
- `GET /api/openapi.json` — generates OpenAPI 3 spec dynamically by walking each template's fields and mapping types (`fieldToProperty` in `apiCollections.js:54-187`). Each field type → JSON Schema property:
  - `text/textarea/latex/code/guid` → `string`
  - `number` → `number`
  - `boolean` → `boolean`
  - `date` → `string` format `date`
  - `dropdown/radio` → `string` enum
  - `multioption/list/tags` → `array` of `string`
  - `range` → `number` with min/max/multipleOf from options
  - `table` → `array` of objects keyed by column ids
  - `image/link` → `string` format `uri`
  - `api` → `oneOf: [string id, {id, ...mappedFields}]`

### F-805 — Swagger UI at `/api/docs`  [size: S] [BLOCKED on F-804]
- Embed swagger-ui distribution and serve at `/api/docs` pointing at `/api/openapi.json`.

### F-806 — Server lifecycle controls (`internalServer` group)  [size: S] [BLOCKED on F-801]
- `startInternalServer(port)`, `stopInternalServer()`, `getInternalServerStatus()` exposed as a thin `internal/modules/server-control` Wails service that toggles the runtime.

### F-807 — Wiki window opening  [size: S] [BLOCKED on F-802]
- Wails equivalent of `system:open-external variant=tab`: spawn a separate `WebviewWindow` pointed at `http://127.0.0.1:<port>/...` instead of opening in the system browser. Used for wiki preview links from inside Formidable.

---

## Epic 9 — Packaging

### F-900 — First-run setup  [size: M] [TODO]
- Mirror `controls/setupManager.js`:
  - Linux/Mac packaged: redirect data to `~/.local/share/Formidable2/` (XDG_DATA_HOME or fallback).
  - Windows: portable `./user-data` in process.cwd().
  - Create `config/boot.json` and `config/user.json` with defaults if missing.
  - Copy bundled `examples/` to user data dir on first run.
  - Set `context_folder` default to `./examples`.
- Templates seeded via `template.SeedBasicIfEmpty()`.

### F-901 — Linux deb + AppImage  [size: M] [TODO]
- Verify Wails-generated `.desktop`, icon, AppImage build.
- Maintain old `productname` / category metadata.

### F-902 — Windows NSIS  [size: M] [TODO]
- Match old Formidable NSIS options (perMachine: false, allowToChangeInstallationDirectory).

### F-903 — macOS dmg  [size: M] [TODO]
- Unsigned dmg, matches old config.

### F-904 — File associations  [size: S] [TODO]
- Decide which extensions Formidable2 should claim (`.fmd`? template files?). Update `build/config.yml`.

---

## Epic X — Plugins (DEFERRED)

Out of scope for this migration round. Parking lot.

The current plugin system has TWO runtimes:

1. **Backend plugins** (e.g. `BackTest`) — Node.js modules `require()`-d in main process; register IPC handlers via `plugin.json` `ipc` map. **No direct port to Go**: Go cannot run Node modules.
2. **Frontend plugins** (e.g. `PandocPrint`) — ESM modules loaded by the renderer, use `FGA.plugin.*`, `button`, `modal`, `dom`, `string`. **Port-friendly**: same renderer environment in Wails.

Decisions to make before scoping seriously:

### F-X01 — Backend plugin runtime selection  [size: M] [DEFERRED]
- Options:
  - **`goja`** — Pure-Go JS interpreter. Existing plugins using `require()`/`https` would still need re-coding (no Node runtime), but JS source survives.
  - **`gopher-lua`** — Different language. Breaks compatibility with existing 2 backend plugins.
  - **Spawn Node child process** — Keeps full compatibility but adds a runtime dependency users must install.
  - **Drop backend plugins entirely** — Keep only frontend plugins; require any backend logic to come through dedicated Wails services.
- Outcome: ADR in `design/`.

### F-X02 — Frontend plugin loader  [DEFERRED]
- Port `FGA.plugin.*` and the dynamic loader. Mostly renderer-side work; no Wails service needed beyond plugin discovery (list folders, read manifests, dispatch frontend code).

### F-X03 — `plugin` Wails service skeleton  [BLOCKED on F-X01]

### F-X04 — Migration story for existing plugins  [BLOCKED on F-X01, F-X02]
- BackTest and PandocPrint are the active plugins. PandocPrint is frontend-only and shells out via `executeCommand` IPC to `pandoc` — likely just works after the frontend plugin loader lands. BackTest is a demo, can be ignored.

---

## Spike & Tech-Debt Backlog

| ID | Title | Status |
|---|---|---|
| F-S01 | `go build ./...` fails inside `build/ios/` (scaffold quirk). Investigate excluding via Go workspace or build tag. | TODO |
| F-S02 | Verify `wails3 dev` startup time after each phase; target < 5s for cached runs. | TODO |
| F-S03 | Decide testing baseline: stdlib `testing` only vs `testify`. Recommend stdlib. | TODO |
