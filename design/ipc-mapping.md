# IPC Mapping - Electron `preload.js` → Wails Service Modules

Maps each `window.api.*` and `window.electron.*` group from `/home/peter/Projects/Formidable/preload.js` to the corresponding `internal/modules/<name>/` package. The "HTTP handlers?" column flags whether the module additionally exposes a **loopback-only** local API. Most modules don't.

## `window.api.*`

| Electron group | Module | HTTP handlers? | Notes |
|---|---|---|---|
| `encrypt` | DEFERRED - not ported (see F-502 in backlog) | n/a | Original used static-IV AES-CBC with key-in-plaintext; never used in practice. Frontend call sites will be removed during the renderer port. |
| `internalServer` | (not a module) | n/a | Lifecycle controls live on `internal/server`; service exposes start/stop/status. |
| `plugin` | `internal/modules/plugin` | tbd | DEFERRED - Epic X. |
| `help` | `internal/modules/help` | yes | Static-ish topic list and content. |
| `git` | `internal/modules/git` | yes | ~30 methods. Hybrid `go-git/v5` + shell-out for merge/rebase. |
| `gigot` | `internal/modules/gigot` | no | **Remote sync** to a GiGot HTTP server with bearer auth. Wails-only - sync transport never goes through the local server. |
| `journal` | `internal/modules/journal` | yes | `pending`, `cursor`. |
| `config` | `internal/modules/config` | yes | Profiles, paths, virtual structure (TTL cache lives here). |
| `templates` | `internal/modules/template` | yes | List/load/save/delete/validate, descriptors, item fields. **Storage format is YAML** at `<context>/templates/<name>.yaml`. Module name singular per data-model concept. |
| `forms` | `internal/modules/storage` | yes | List/load/save/delete + image save. **Storage format is JSON `.meta.json` files** under `<context>/storage/<template-name>/`, not YAML. Module renamed `storage` (the disk concept), not `forms` (the IPC group). |
| `csv` | `internal/modules/csv` | yes | `encoding/csv` for preview/import/write. |
| `transform` | `internal/modules/transform` | yes | Markdown/HTML/frontmatter/mini-expr. JS-side rendering preferred (see migration-plan). |
| `system` | `internal/modules/system` | **no** | Raw filesystem + `proxyFetchRemote`. Keep Wails-only - too sensitive for HTTP. |
| `dialog` | (built-in) | n/a | `application.OpenFileDialog` etc. Wired in `frontend/src/main.ts`. |

## `window.electron.*`

| Electron group | Wails equivalent |
|---|---|
| `shell.openPath` / `openExternal` | `application.Application.OpenURL` / OS file manager |
| `app.quit` | `app.Quit()` |
| `devtools.toggle` | webview window method |
| `window.{reload,minimize,maximize,close}` | `WebviewWindow.*` methods |
| `clipboard.{writeText,readText}` | `application.Clipboard` |
| `sfr.*` | `internal/modules/sfr` (SingleFileRepository; Wails-only, no HTTP) |

## Frontend bindings paths

After scaffolding modules, generated bindings land at:

```
frontend/bindings/github.com/petervdpas/formidable2/internal/modules/<name>/
```

Old usage:
```js
window.api.config.loadUserConfig()
```

New usage:
```ts
import { Service as Config } from "../bindings/github.com/petervdpas/formidable2/internal/modules/config";
await Config.LoadUserConfig();
```

## Cross-module dependency sketch

```
config  ←  templates  ←  forms ──┐
   ↑          ↑           ↑      │
   └─── transform ────────┘      │
        (config provides paths;  │
         templates/forms read    │
         from system+config)     │
                                 │
internal/server  ────────────────┘  (wiki + REST mount points)

system   (FS primitive - used by everything else)
sfr      (SingleFileRepository; depends on system)
git      (depends on system; reads repo path from config)
gigot    (depends on system, config, journal; pure HTTP client to remote GiGot server)
journal  (depends on config; append-only log + per-backend cursor)
csv      (depends on system, forms)
encrypt  (depends on OS keyring; standalone)
help     (depends on system; mostly static markdown)
```

Wired in `internal/app/app.go`. Deps cross module boundaries as **interfaces** declared in the consumer (`config.Reader`, `system.FS`, `journal.Emitter`), not as concrete types.

## Form storage format note

Templates: YAML at `<context>/templates/<name>.yaml`.
Forms: JSON at `<context>/storage/<template-name>/<form>.meta.json`.
Images: arbitrary binary at `<context>/storage/<template-name>/images/`.

The Wails port preserves these formats exactly so existing user data continues to work without migration.
