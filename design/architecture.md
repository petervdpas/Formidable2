# Architecture — Formidable2

Formidable2 is a **local-first desktop app** with three runtime surfaces:

1. **Wails IPC bindings** — primary path for the bundled UI to reach Go services.
2. **Loopback HTTP server** (`127.0.0.1:8383`) — Formidable's existing local API: a browseable wiki view + a REST collections API with OpenAPI/Swagger UI. Opt-in per module, started/stopped by the user. **This is NOT a sync transport** — never bound beyond loopback.
3. **Optional remote sync** — Git or GiGot (a remote sync server with bearer-token auth). Off by default.

See `formidable-findings.md` for the full picture of what the existing app actually is.

## Frontend stays event-driven (do not flatten this)

The current Formidable renderer is built around an **EventBus** — `EventBus.emit("form:save", payload)` resolves through 30 domain handler modules in `modules/handlers/`, some of which stay in renderer (cache, field, modal, theme, status, history, render, markdown) and some of which call `window.api.*` to cross to the main process. The original ships a `window.FGA` (form/context/util) global and a `window.CFA` (CodeField API) used by `code`-type fields to manipulate other fields by GUID.

The Wails port **preserves this whole shape on the frontend**. `modules/eventBus.js`, `modules/eventRouter.js`, the 30 handler modules, and FGA come over essentially untouched. The change is what the *cross-process* handlers do internally: instead of `window.api.config.loadUserConfig()` they call `import { Service as Config } from ".../bindings/.../config"` and `await Config.LoadUserConfig()`. The event-driven contract stays the same. CFA does **not** port — Formidable2 dropped the `code` and `latex` field types, so the CodeField API has no consumer.

## Module shape

Every domain feature lives in its own folder under `internal/modules/`. A module always has a **backend** and an **api** layer; **handlers** are opt-in and only present where local HTTP access makes sense.

```
internal/modules/<name>/
├── domain.go        # backend layer — pure logic, no transport awareness
├── service.go       # api layer — Wails Service{} bound to the frontend
├── handlers.go      # OPTIONAL — local HTTP routes (only when external local consumers need it)
├── types.go         # shared DTOs across the layers
└── domain_test.go
```

| Layer | File | Knows about | Calls |
|---|---|---|---|
| backend | `domain.go` | nothing transport-y | other domains via interfaces |
| api | `service.go` | Wails (`*application.App` if needed) | only its own domain |
| handlers (opt-in) | `handlers.go` | `net/http` | only its own domain |
| frontend | `frontend/src/...` | bindings + (optional) REST clients | api primarily; HTTP only where it earns its keep |

**Modules that expose HTTP handlers**: `templates`, `forms`, `csv` (export endpoints), `transform` (markdown/HTML for wiki render), and the wiki/index routes (which read from `templates`+`forms`+`config` via the runtime).

**Modules that should NOT expose handlers**: `system`, `sfr`, `encrypt`, `git`, `gigot`, `config`, `plugin`, `journal` — raw filesystem / command execution / crypto / sync transport / dynamic plugin runtime are too sensitive or too internal even for a loopback API surface.

## Composition root

```
internal/app/
├── app.go        # New(deps) constructs every module and wires deps
├── services.go   # WailsServices() []application.Service
└── routes.go     # RegisterRoutes(mux *http.ServeMux)
```

`main.go` stays slim — instantiate `app.New(...)`, hand the services to Wails, hand the routes to the internal server.

## Local HTTP server — two surfaces

```
internal/server/   # owns the http.Server lifecycle, calls app.RegisterRoutes
```

Replaces the Electron `internalServer.js`. **Loopback-only** (`127.0.0.1`), user-toggleable via the `enable_internal_server` config flag (default off). Default port `8383`. Two distinct surfaces:

### A. Wiki view (HTML)

Read-only browseable HTML of the user's templates and forms. Routes mirrored from `Formidable/controls/internalServer.js`:

- `GET /` — index of templates with sidebar-expression evaluations.
- `GET /template/:name` — list of forms in a template, with sidebar-expression badges and tags.
- `GET /template/:name/form/:filename` — form rendered as markdown → HTML.
- `GET /template/:name/extended-list` — JSON variant of the form list.
- `GET /storage/...` — image/file passthrough from the VFS storage path.
- `GET /assets/...` — static UI assets.
- `GET /favicon.ico`.
- `GET /miniexpr` — mini-expression playground.
- `GET /virtual` — VFS dump (dev only).

### B. REST collections API (JSON + Swagger)

CRUD over forms-as-collections, with an OpenAPI spec generated dynamically from each template's field schema:

- `GET /api/collections` — list collection-enabled templates.
- `GET /api/collections/:t/count` — count.
- `GET /api/collections/design/:t` — template schema for OpenAPI shape.
- `GET /api/collections/:t` / `GET /api/collections/:t/:id` — list / read.
- `POST /api/collections/:t` — create.
- `PUT /api/collections/:t/:id` / `PATCH /api/collections/:t/:id` / `PATCH /api/collections/:t/:id/field/:key` — update.
- `DELETE /api/collections/:t/:id` — delete.
- `POST /api/collections/:t/batch` — bulk.
- `GET /api/collections/:t/export.{ndjson,csv}` — export.
- `GET /api/openapi.json` — generated OpenAPI 3 spec.
- `GET /api/docs` — Swagger UI.

The wiki view is opened from the Formidable UI by spawning a **separate webview window** (Wails equivalent of Electron's `system:open-external variant=tab`) pointed at `http://127.0.0.1:8383/...` — matches today's UX where help/preview links open in their own window.

## Rules

1. **No transport in `domain.go`.** Domain types are plain Go; no `*application.App`, no `http.ResponseWriter`. Tests run without spinning up Wails or HTTP.
2. **Cross-module deps are interfaces, not concrete types.** `templates.NewManager` takes a `ConfigReader` interface, not `*config.Manager`. Wired in `internal/app/app.go`.
3. **api method names match old Electron IPC names** (PascalCase'd). Frontend porting becomes a binding-import swap, not a behavioral rewrite.
4. **One module = one folder, one Go package.** No utility dumping ground; if it's shared, lift to its own module (`internal/modules/util/` or split per concern).
5. **Frontend has no awareness of layering** — it imports a Wails binding and gets a typed method. Whether that hits a HTTP handler or Wails service is the runtime's problem.

## Skeleton example — `config` module

```go
// internal/modules/config/domain.go
package config

type Manager struct {
    fs       FS         // injected interface (system module)
    log      *slog.Logger
    mu       sync.RWMutex
    cached   *Config
}

func NewManager(fs FS, log *slog.Logger) *Manager { ... }
func (m *Manager) Load() (*Config, error)         { ... }
func (m *Manager) Save(cfg *Config) error         { ... }
func (m *Manager) VirtualStructure() (*VStruct, error) { ... }
```

```go
// internal/modules/config/service.go (api layer)
package config

type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

func (s *Service) LoadUserConfig() (*Config, error)       { return s.m.Load() }
func (s *Service) UpdateUserConfig(cfg *Config) error     { return s.m.Save(cfg) }
func (s *Service) GetVirtualStructure() (*VStruct, error) { return s.m.VirtualStructure() }
```

```go
// internal/modules/config/handlers.go (handlers layer)
package config

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/config",  s.httpGet)
    mux.HandleFunc("POST /api/config", s.httpUpdate)
}
```

```go
// internal/app/app.go (composition root)
type App struct {
    Config    *config.Service
    Templates *templates.Service
    // ...
}

func New(d Deps) *App {
    fs := system.NewManager(d.Logger)
    cfgM := config.NewManager(fs, d.Logger)
    tmplM := templates.NewManager(cfgM, fs, d.Logger)
    return &App{
        Config:    config.NewService(cfgM),
        Templates: templates.NewService(tmplM),
    }
}

func (a *App) WailsServices() []application.Service {
    return []application.Service{
        application.NewService(a.Config),
        application.NewService(a.Templates),
    }
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
    a.Config.RegisterRoutes(mux)
    a.Templates.RegisterRoutes(mux)
}
```

## What this buys vs. today's Formidable

- Renaming/moving a module is a one-folder operation; no `ipcRegistry.js` / `ipcRoutes.js` to keep in sync.
- Domain logic unit-testable without Wails or HTTP.
- Where handlers exist, api ↔ handlers parity is structural — both call the same domain method.
- Module boundaries enforced by the Go package system.
- Local-first stays local-first: the HTTP layer is loopback-only and per-module opt-in.
