# Formidable — The Dynamic Form & Template Designer

**Formidable** is a desktop app for designing YAML-based form templates,
filling them out, and rendering the result through a Handlebars +
Markdown pipeline. It's the Wails 3 + Go + Vue 3 successor to the
original Electron build — same templates, same field types, same
"auditability by design" stance, but a smaller native binary, a
proper Go backend, and a real REST API.

![Formidable](build/appicon.png)

> 💠 Dedicated to **Elly** — who lived with strength, humor and clarity.
> _"Sleep, don't weep — My sweet love"_ — Damien Rice
>
> 🌌 And to **Aaron Swartz** — who refused to back down when it mattered.
> _"We are all made of stardust, and we are all made of stories."_ — Aaron Swartz

---

## Status

Alpha. Working today:

- Template designer + form editor (17 stored field types, full Handlebars rendering, live slideout preview)
- Per-profile context folders, profile switching, VFS resolution
- Internal HTTP server with two surfaces:
  - **Wiki** — `/`, `/template/{stem}`, `/template/{stem}/form/{datafile}`
  - **REST API** — full CRUD on collection-enabled templates + Swagger UI at `/api/docs/`
- SQLite-backed per-profile index for sub-millisecond list/search
- Append-only system-level journal recording every FS mutation (groundwork for git/gigot sync)

WIP:

- Frontend journal viewer + git/gigot sync triggers (backend journal records; no UI yet)
- Plugin system (placeholder workspace; not ported)
- Per-template ACL on the API write surface
- Per-user data dir resolver for installed builds (XDG/AppData paths)

---

## Architecture

Composition root in `internal/app/app.go` wires every module once at boot.

**Backend (`internal/modules/`)**

| Module          | Role                                                                  |
| --------------- | --------------------------------------------------------------------- |
| `system`        | Atomic FS primitives (`temp+fsync+rename`); journals every mutation   |
| `sfr`           | Sanitize-Filename-Read wrapper for safe content r/w                   |
| `config`        | User config, boot profile, virtual file structure resolution          |
| `journal`       | Append-only change log; emits `journal:changed`                       |
| `template`      | YAML template load/save/validate; 17 field types in a registry        |
| `storage`       | Form `.meta.json` r/w, image bytes, per-template storage layout       |
| `form`          | Storage workspace orchestration (template + storage + config defaults)|
| `index`         | Per-profile SQLite cache of template/form metadata                    |
| `dataprovider`  | Read-only facade composing index + render — used by wiki/api          |
| `render`        | raymond Handlebars + goldmark + chroma; per-target URL strategies     |
| `nav`           | `formidable://` URL resolver; persists active selection               |
| `wiki`          | Runtime HTTP server hosting `/`, `/template/...`, `/storage/...`      |
| `api`           | `/api/collections/*` REST + OpenAPI 3.0.3 spec + Swagger UI           |
| `csv`           | CSV import/export helpers                                             |
| `i18n`          | Backend-rooted localization served to vue-i18n                        |
| `dialog`        | Native file/folder pickers via Wails                                  |

**Frontend (`frontend/src/`)**

Vue 3 SPA with workspaces for **Templates**, **Storage**, **Profiles**,
**Settings**, **Information** (server lifecycle, monitoring), and
**Plugins** (placeholder). Calls the backend via Wails service bindings
generated from each module's `Service` type — no IPC, no `window.api`
shim. UI strings flow through vue-i18n from `internal/modules/i18n/locales/`.

**Render pipeline**

Transport-neutral. Each consumer constructs its own `render.Manager`
with a `(imageURLFunc, formidableLinkURLFunc)` pair so the same
Handlebars output can target the in-app slideout (data: URLs +
`formidable://` for the Vue interceptor), the wiki HTTP server
(`/storage/.../images/...` + `/template/.../form/...`), or future
exports (Azure DevOps Wiki, GitHub Wiki, plain MD) without a single
branch inside the render module.

---

## Field types

17 stored types + 4 system/container, all driven from
`internal/modules/template/field_registry.go`:

- **Identity**: `guid`
- **Text**: `text`, `textarea`, `latex`, `code`
- **Numeric**: `number`, `range`
- **Boolean**: `boolean`
- **Choice**: `dropdown`, `radio`, `multioption`
- **Date**: `date`
- **Collections**: `list`, `table`, `tags`
- **Media**: `image`, `link`
- **API-linked**: `api` (selection from a sibling collection with mapped fields)
- **Containers**: `loopstart` / `loopstop` (and the `looper` virtual type — repeating sections, max nesting depth 2)

Templates with `enable_collection: true` AND a `guid` field become
addressable via `/api/collections/{stem}/{guid}` and surface in
`/api/collections`.

---

## REST API

`GET /api/docs/` for the live Swagger UI; `GET /api/openapi.json` for
the spec (rebuilt per request from current templates). Endpoints:

```
GET    /api/guid                                     — server-minted UUID v4
GET    /api/collections                              — list collection-enabled templates
GET    /api/collections/{tpl}                        — paged list (limit/offset/q/tags + ETag)
GET    /api/collections/{tpl}/count                  — total
GET    /api/collections/{tpl}/design                 — fields with normalized options
GET    /api/collections/{tpl}/{id}                   — full meta+data (ETag)
HEAD   /api/collections/{tpl}/{id}                   — ETag-only freshness check
POST   /api/collections/{tpl}                        — create (?upsert=true to overwrite)
PUT    /api/collections/{tpl}/{id}                   — replace (?upsert=true to create)
PATCH  /api/collections/{tpl}/{id}                   — shallow-merge (optional If-Match → 412)
PATCH  /api/collections/{tpl}/{id}/field/{key}       — single-field update
DELETE /api/collections/{tpl}/{id}                   — 204
POST   /api/collections/{tpl}/batch?mode=...         — bulk create|replace|merge
GET    /api/collections/{tpl}/export.ndjson          — full streamed export
GET    /api/collections/{tpl}/export.csv             — id/filename/title/tags
```

`{id}` is always the GUID value (the field whose `type: guid`),
never the filename. Filenames are derived from the template's
`item_field` via slugify with collision suffix (`brood.meta.json`,
`brood-2.meta.json`, …) and fall back to `<guid>.meta.json`.

POST auto-mints a GUID server-side when the body's `data[guidKey]` is
empty, so callers don't need a UUID library.

---

## Template syntax

raymond (vendored fork in `third_party/raymond/`) + goldmark + chroma:

```handlebars
# {{field "title"}}

{{#if (fieldRaw "check")}}
Enabled
{{else}}
Disabled
{{/if}}

## Tags
{{#each (fieldRaw "tags")}}
- {{this}}
{{/each}}

## Table
{{#if (fieldRaw "rows")}}
| Col1 | Col2 |
|------|------|
{{#each (fieldRaw "rows")}}
| {{this.0}} | {{this.1}} |
{{/each}}
{{/if}}
```

Helpers: `field`, `fieldRaw`, `fieldMeta`, `tags`, `stats`, plus the
math/comparison families. See `internal/modules/render/helpers.go`.

---

## Build & run

Requires Go 1.25+, Node 20+, and the Wails 3 CLI:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

Then via [Task](https://taskfile.dev/):

```bash
task dev          # build + run; clean exit on window close
task dev:watch    # hot-reload via vite + wails3 dev
task build        # build the binary into ./bin/Formidable
task package      # platform installer (.deb / .rpm / .dmg / .exe)
```

Linux packaging is .deb / .rpm / Arch via nfpm; the binary runs as
`/usr/local/bin/Formidable` and the `.desktop` entry as `Formidable`.

---

## Tests

TDD throughout — every behavior has either a unit test or a Gherkin
scenario before the implementation lands.

```bash
go test -race ./...                              # whole tree, race detector on
go test -race -v ./internal/modules/api/...      # one module, verbose
```

The api module alone ships 104 godog scenarios + 682 steps; the wiki
module 39; render 88; storage and template each well into the dozens.
Feature files live under each module's `features/` folder.

---

## Layout

```
internal/
  app/                       — composition root (single boot wiring)
  log/                       — slog setup
  modules/                   — domain modules (table above)
frontend/
  src/                       — Vue 3 SPA
  bindings/                  — generated Wails service stubs
build/                       — Wails per-platform targets, nfpm.yaml, appicon.png
third_party/raymond/         — vendored Handlebars fork (whitespace + options-only patches)
Examples/                    — sample templates & storage (basic, loopie, recepten)
design/                      — port findings, source docs (read-only reference)
main.go                      — Wails app entry point
Taskfile.yml                 — task entry points
```

---

## License

MIT © 2026 Peter van de Pas

---

## Acknowledgments

- [Wails](https://wails.io/) — Go + webview desktop framework
- [Vue 3](https://vuejs.org/) — frontend framework
- [raymond](https://github.com/aymerick/raymond) — Handlebars-for-Go (vendored fork)
- [goldmark](https://github.com/yuin/goldmark) — CommonMark in Go
- [chroma](https://github.com/alecthomas/chroma) — syntax highlighting
- [godog](https://github.com/cucumber/godog) — Gherkin BDD for Go
- [Swagger UI](https://swagger.io/tools/swagger-ui/) — bundled API explorer
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pure-Go SQLite

---

## Links

- **Repo**: https://github.com/petervdpas/Formidable2
- **Original (Electron) Formidable**: https://github.com/petervdpas/Formidable
- **GiGot** (token-based remote sync, future integration): https://github.com/petervdpas/GiGot
