# Migration Plan — Formidable (Electron) → Formidable2 (Wails 3)

Source: `/home/peter/Projects/Formidable` (Electron 41, vanilla JS, v2.0.10)
Target: this repo (Wails v3.0.0-alpha.84, vanilla-ts + Vite)

Formidable2 is **local-first**. Wails IPC is the primary surface; a small loopback HTTP API is opt-in per module for local tooling. Nothing networked.

Plugins are explicitly **deferred** — see Epic X in [backlog.md](backlog.md).

## Phase ordering

| Phase | Focus | Why this order |
|---|---|---|
| 0 | Foundation | Scaffold + `internal/` layout — done. |
| 1 | Core plumbing (config, system, sfr, logging, builtins) | Every other module reads paths from `config` and FS from `system`. |
| 2 | Frontend shell port | Move `controls/`, `modules/`, `templates/`, `i18n/`, CSS into Vite frontend. Adapt `renderer.js` to bindings. |
| 3 | Templates & Forms | The heart of the app — most user-visible features light up here. |
| 4 | Transform | Markdown / frontmatter / mini-expr. Decision: keep JS-side or port to Go. |
| 5 | Smaller services | csv, encrypt, help, journal — straightforward. |
| 6 | Git | ~30 methods. Hybrid go-git + shell-out for merge/rebase. |
| 7 | Gigot | **Remote** sync to a GiGot HTTP server with bearer-token auth. Reimplement the Electron client in Go (`net/http` + `crypto/sha1` for git blob hashing + `.formidable/sync.json` ledger). |
| 8 | Local HTTP server | Loopback-only (`127.0.0.1:8383`). Two real surfaces: wiki view (HTML) + REST collections API (JSON+OpenAPI+Swagger UI). Not optional — a first-class Formidable feature. |
| 9 | Packaging | deb / AppImage / NSIS / dmg, icons, file associations. |
| X | Plugins | Deferred. Decision needed: gopher-lua / goja / sandboxed iframe. |

## Decisions to lock per phase

- **Phase 1** — logging library (`log/slog` vs `zerolog`). Recommend slog (stdlib, structured).
- **Phase 4** — handlebars/markdown-it/gray-matter stay in JS frontend, OR port to Go (`text/template`, `goldmark`, custom frontmatter). Recommend **JS-side** for fidelity.
- **Phase 6** — pure go-git vs hybrid. Recommend **hybrid**: go-git for read paths and simple commits; shell out to `git` binary for merge/rebase/conflict tooling.
- **Phase 7** — GiGot's API shape is documented in the existing `controls/gigotManager.js`. Endpoints: `/api/health`, `/api/me`, `/api/repos/{repo}/{context|formidable|head|tree|files|commits|destinations|...}`. Auth: bearer token. Conflict policy: push-409 → skip pull, user resolves manually. Sync mode: push-then-pull on success.
- **Phase 8** — modules opting into HTTP handlers: `templates`, `forms`, `csv` (export only), `transform` (markdown/HTML for wiki). Wiki routes (`/`, `/template/...`, `/storage/...`) read from `config`+`templates`+`forms` via the runtime. Static assets bundled by `wails3 generate` or served from `frontend/dist`.
- **Phase X** — plugin runtime parking lot (out of scope right now).

## Cross-cutting

- Replace `nodeLogger.js` + `formidable.log` with `log/slog` writing to the same file location for continuity.
- `windowBounds.js` → preserve `window_bounds` config shape (`{width, height, x, y, maximized}`); Wails `WebviewWindowOptions` plus a debounced save-on-resize/move handler in `main.go`.
- Custom HTML menu (`modules/menuManager.js` builds DOM from `i18n` strings; native menu set to null in `main.js:107`) — port the renderer's menu builder verbatim. **Don't** swap to Wails native `application.Menu` unless we want the redesign cost.
- Theme bootstrap (preload's early-theme block) → top of `frontend/src/main.ts`. **Three themes** to preserve: `light · dark · purplish`.
- First-run setup: copy bundled `examples/` to user data dir on Linux/Mac (`~/.local/share/Formidable2/`) and seed `boot.json`+`user.json`. Mirrors `controls/setupManager.js`.
- Frontend port keeps `modules/eventBus.js` + `modules/eventRouter.js` + the 30 `modules/handlers/*.js` files essentially verbatim; only the IPC call sites change to import from `frontend/bindings/...`.

## Definition of "ported"

A module is ported when:
1. `internal/modules/<name>/` exists with `domain.go` / `service.go` / `types.go` (+ `handlers.go` if applicable).
2. Domain logic has tests against `t.TempDir()` for FS-touching paths.
3. Service registered in `internal/app/app.go`.
4. Frontend consumer imports from the new bindings path; old `window.api.<group>` calls removed.
5. Equivalent code in `/home/peter/Projects/Formidable/controls/` is no longer referenced by anything porting forward.
