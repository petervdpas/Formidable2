# PDF Export Pipeline — Design

Replace the external `gomnirun + pandoc + eisvogel` pipeline (used outside Formidable for PDF generation) with an in-process Go pipeline built on [picoloom v2](https://github.com/alnah/picoloom).

Picoloom is a small, opinionated Go library + CLI for Markdown → PDF using headless Chrome (no LaTeX). BSD-3-Clause. v2.1.2 at time of writing.

See [architecture.md](architecture.md) for module conventions and [migration-plan.md](migration-plan.md) for phase ordering. This feature does not block the migration but lights up once `render` and `wiki` are stable, both of which already shipped.

---

## Decisions (settled)

| Topic | Decision |
|---|---|
| Engine | picoloom v2 (Go library, headless Chrome via go-rod). No LaTeX, no pandoc. |
| Replacement scope | Full replacement of pandoc+eisvogel. No backend interface, no dual-engine support. |
| Module | `internal/modules/pdf/` — peer to `render`, `wiki`, `api`. Singular data-model name per project convention. |
| Lifecycle | Lazy opt-in. Service refuses to render until activated. |
| Activation surface | Information page panel, alongside Wiki/API status panels. Same UX pattern as those services. |
| Chrome runtime | Probe `ROD_BROWSER_BIN` → standard install paths → managed Chromium download (~150 MB zip, ~530 MB unpacked — see Stage 0 findings) on user confirmation. Slim installer, no bundled Chromium. |
| Override priority | `frontmatter > form meta > template manifest > global config`. |
| Activation persistence | Per-machine state file at `<AppRoot>/config/pdf-state.json`, owned by the pdf module via `system.Manager`. **Not** in `user.json` — `browser_bin` is machine-specific and would break under gigot/git sync. (Settled 2026-05-14; earlier draft of this doc said `config.Manager` under a `pdf:` block — that was wrong.) |
| Frontmatter schema | Nested, mirrors `picoloom.Input` (`cover:`, `toc:`, `watermark:`, `page:`, `pageBreaks:`, `signature:`, `footer:`). All four override layers share this schema. |
| Frontmatter origin | Lives in template source. Survives the raymond Handlebars pass, so `cover.title: "{{form.title}}"` resolves before picoloom strips it. |
| Render integration | Picoloom's md→html→pdf path replaces the goldmark+chroma half of `render` for PDF output only. Wiki/API paths still use full `render`. Pipeline forks after raymond expansion. |
| Renderer scoping | One `pdf.Renderer` per export call, per the existing "one render.Manager per export target" rule. |
| Output write | Atomic, via `system.SaveFile` (temp + fsync + rename). |
| i18n | All user-facing strings (activation panel, errors, settings) routed through `internal/modules/i18n`. No hardcoded UI text. |

## Open questions (settle as we go)

- **Themes** — picoloom ships 8 embedded CSS themes (`default`, `technical`, `creative`, `academic`, `corporate`, `legal`, `invoice`, `manuscript`). Whitelist 2–3 as Formidable defaults + custom CSS path, or expose all 8 in a template field?
- **Export action UI** — slideout button, template-level export menu, both, or a workspace action? Mirror whichever Formidable already does for CSV/markdown export.
- **Frontmatter ↔ template manifest boundary** — picoloom-specific knobs (watermark text, page breaks, theme name) could live in either layer. Default: keep frontmatter for per-document overrides, manifest for template-wide defaults.
- **Batch export** — picoloom exposes a `ConverterPool`. Worth wiring up for "export all forms in this template", but not required for v1.
- **Chromium update story** — managed Chromium pinned to a go-rod-default revision. Update cadence and security-patch policy: TBD. Likely a "re-download" button in the activation panel.

---

## Pipeline

```
template source (.md.hbs with frontmatter)
        │
        ▼
   raymond expansion        ← form data, helpers, partials
        │
        ▼
markdown + concrete frontmatter
        │
        ├──► (existing) goldmark + chroma → HTML → wiki / API surface
        │
        └──► (new)  pdf.Renderer
                       │
                       ▼
              parse frontmatter
              merge with manifest + form meta + global config
                       │
                       ▼
              build picoloom.Input{Markdown, Cover, TOC, ...}
                       │
                       ▼
              picoloom.Convert (Chrome PDF)
                       │
                       ▼
              system.SaveFile(<output>.pdf)
```

## Module layout

```
internal/modules/pdf/
├── domain.go         # Renderer, Activator, frontmatter parser, Input builder
├── service.go        # Wails Service{}: Activate, Status, ExportPDF
├── types.go          # Frontmatter struct, Status enum, ActivationState
├── activate.go       # browser-bin probe + managed download orchestration
├── input.go          # frontmatter + manifest + meta + config → picoloom.Input
├── domain_test.go
├── input_test.go
├── activate_test.go
└── features/
    ├── activate.feature
    ├── export.feature
    └── frontmatter_overrides.feature
```

No `handlers.go` — PDF generation is Wails-only. Not exposed on the loopback HTTP server.

## Frontmatter schema (working draft)

Picoloom **strips** YAML frontmatter before conversion, so the `pdf` module reads it first, maps known keys, then hands the (still-frontmattered, picoloom-stripped) markdown to picoloom. Schema mirrors `picoloom.Input` 1:1 so all four override layers can use the same shape.

```yaml
---
style: technical                  # picoloom embedded theme name OR custom path

page:
  size: a4                        # letter | a4 | legal
  orientation: portrait           # portrait | landscape
  margin: 0.75                    # inches, 0.25–3.0

cover:
  enabled: true
  title: "{{form.documentTitle}}" # raymond expands before strip
  subtitle: "..."
  author: "{{form.author}}"
  organization: "Fontys"
  date: auto:long                 # iso | european | us | long | auto:FORMAT
  logo: ./assets/logo.png
  showDepartment: false

toc:
  enabled: true
  title: "Contents"
  minDepth: 2                     # 1–6, skip H1 by default
  maxDepth: 3

footer:
  enabled: true
  showPageNumber: true
  position: right                 # left | center | right
  showDocumentID: false
  text: ""

signature:
  enabled: false
  imagePath: ""
  links: []

watermark:
  enabled: false
  text: DRAFT
  color: "#888888"
  opacity: 0.10
  angle: -45

pageBreaks:
  enabled: true
  beforeH1: true
  beforeH2: false
  beforeH3: false
  orphans: 2
  widows: 2
---
```

Eisvogel-specific keys from the old pipeline (`titlepage-rule-height`, `listings`, `book`, `classoption`, `caption-justification`, `footnotes-pretty`) are **dropped** — they're LaTeX-only. Picoloom replaces their visual outcome via CSS themes.

---

## Stages

Each stage follows TDD per project convention: tests/Gherkin first, implementation after.

### Stage 0 — Vendor + dependency proof

**Goal**: prove picoloom integrates cleanly into Formidable's build, before any module work.

- Add `github.com/alnah/picoloom/v2` to `go.mod`. Verify resolves on Linux/Win/Mac.
- Stand up a throwaway `cmd/pdf-poc/main.go` that converts a fixed markdown string to PDF and writes to `/tmp/poc.pdf`. Run on dev box.
- Confirm go-rod's Chrome probe behavior on a fresh machine without Chrome installed (managed download path).
- Confirm Chrome probe behavior with Chrome installed (no download).
- Note picoloom's binary footprint impact on the Formidable2 build artifact.

**Definition of done**: PoC produces a valid PDF; build size delta documented; throwaway code removed.

#### Stage 0 — findings (2026-05-14)

- **Versions pinned**: `github.com/alnah/picoloom/v2 v2.1.2`, transitive `github.com/go-rod/rod v0.116.2`.
- **Go directive bump**: `go get` raised `go 1.25.0` → `go 1.25.4` (one of the ysmood deps requires it).
- **Binary size delta**: minimal `cmd/` main 4.8 MB → with picoloom import 25.4 MB (+20.6 MB). Most of that is go-rod + its CDP/JS-injection blobs.
- **Managed-download first run**: ~80s on dev network. Zip is ~140 MB, unpacks to **533 MB** at `~/.cache/rod/browser/chromium-1321438`. **The design doc's "~55 MB" figure is wrong** — that's roughly the compressed delta. Activation UX should say something like "downloads Chromium (~150 MB compressed, ~530 MB on disk)".
- **Warm-cache render**: 830 ms managed / 410 ms system Chromium for a small (1-page) document. Both fast enough that Stage 5's progress UI can be a simple toast unless documents grow large.
- **Important deviation from this doc's Stage 2 probe order**: go-rod does **not** auto-probe system Chrome paths. Default is "always managed download". To use system Chromium it requires either `ROD_BROWSER_BIN=/usr/bin/chromium` in env, or `launcher.New().Bin(path)` (rod-level, not exposed by picoloom). picoloom's `Option` set (`WithTimeout`, `WithStyle`, `WithAssetPath`, `WithAssetLoader`, `WithTemplateSet`) does not include a browser-bin override — confirmed against pkg.go.dev. Stage 2's `activate.go` must do the system-path scan itself and set `ROD_BROWSER_BIN` before any `picoloom.NewConverter()` call. If we need finer control (e.g. surfacing browser version), we'd construct go-rod's `launcher` ourselves and feed picoloom a pre-built browser — but that requires picoloom to expose an `Option` we don't currently have. File an upstream issue if needed.
- **PoC output**: valid PDF 1.4, 1 page, 44 KB. Headings, links, table, code fence, blockquote, footnotes + backref all render. Default theme is borderless — themes will be settled in Stage 6.

### Stage 1 — Module skeleton + Wails service

**Goal**: `pdf.Service` exists, registered, callable from frontend, returns `ErrPDFNotActivated` for every call.

- Scaffold `internal/modules/pdf/{domain,service,types,activate,input}.go` + tests + features dir.
- Wails Service surface (working draft):
  - `Activate(ctx, opts ActivateOpts) (Status, error)`
  - `Status(ctx) Status`
  - `Deactivate(ctx) error`
  - `ExportPDF(ctx, formGUID string, opts ExportOpts) (Result, error)`
- `Status` struct: `{ Active bool, BrowserBin string, Source: "system"|"managed"|"unset", Version string }`.
- Persist activation state in `<AppRoot>/config/pdf-state.json` (per-machine; see "Activation persistence" row above). Stage 1 ships the in-memory Manager only; Stage 2 adds the store via `system.Manager`.
- All methods return `ErrPDFNotActivated` until Stage 2 lands.
- Register in `internal/app/app.go`, regenerate bindings.

**Definition of done**: bindings regenerate cleanly; frontend can call `Status()` and see `{Active: false}`; `ExportPDF` returns the typed error.

### Stage 2 — Activation flow

**Goal**: user can click "Activate PDF generation" on the Information page and have a working pipeline afterwards.

- Probe order in `activate.go`:
  1. `ROD_BROWSER_BIN` env var
  2. Common system paths (`/usr/bin/google-chrome`, `/usr/bin/chromium`, `/Applications/Google Chrome.app/...`, Windows registry / `Program Files`)
  3. go-rod's managed download cache (`~/.cache/rod/browser/...`)
- `Activate` surface methods:
  - `ProbeChrome(ctx) ProbeResult` — read-only, returns what we found.
  - `UseSystemChrome(ctx, path string) error` — explicit pick.
  - `DownloadManagedChromium(ctx, progress chan<- DownloadProgress) error` — triggers go-rod's download with progress streaming.
- Information-page Vue panel (in `frontend/src/`):
  - Status row mirroring Wiki/API panels.
  - Activation button → opens a dialog that calls `ProbeChrome` and shows: found path (with "Use this" button), or "Not found — download managed Chromium (~150 MB download, ~530 MB on disk)?".
  - Reconfigure / Deactivate links once active.
  - i18n keys under `internal/modules/i18n/locales/<locale>/pdf.json`.
- Frontend catches `ErrPDFNotActivated` from any later `ExportPDF` call and routes the user to the Information page with the activation panel highlighted.

**Definition of done**: activation works on a clean machine (no Chrome) and on a machine with Chrome installed; status persists across restarts; deactivation flips status back to inactive without deleting the managed Chromium cache.

### Stage 3 — Frontmatter parser + Input builder

**Goal**: given a markdown document and the merge inputs (manifest, form meta, global config), produce a valid `picoloom.Input`.

- `Frontmatter` struct mirroring `picoloom.Input` shape (typed YAML).
- Parser that splits `---\n...\n---\n<body>` cleanly. Tolerant of missing frontmatter (uses defaults).
- Merge function with explicit priority: `frontmatter > form meta > template manifest > global config`.
- `BuildInput(fm Frontmatter, body string) picoloom.Input` — pure function, no I/O, easy to test.
- Property-test the override priority: every layer can override every key.
- Unhappy-path tests: malformed frontmatter, type mismatches, unknown keys (warn + ignore, do not crash), missing closing `---`.

**Definition of done**: `BuildInput` round-trips every settable knob; merge priority verified for every key; malformed frontmatter logs a warning and uses defaults.

### Stage 4 — Render pipeline integration

**Goal**: `pdf.Service.ExportPDF(formGUID)` produces a PDF on disk by stitching `render` and `pdf` together.

- `pdf.Renderer` constructed per export call with `(formGUID, exportPath)`.
- Calls `render.Manager.RenderForm(formGUID)` to get the raymond-expanded markdown (with frontmatter still embedded — `render` shouldn't strip it for PDF target).
- Parses frontmatter, builds `picoloom.Input`, calls `picoloom.Converter.Convert`.
- Writes result via `system.SaveFile`. Atomic.
- Output path resolution: `<form>.pdf` next to the form by default, overridable via `ExportOpts.OutputPath`.
- Concurrent export: per-form serialization (mutex on the `formGUID`); independent forms can render in parallel.

**Definition of done**: every Examples/template form can be exported to PDF without errors; round-trips known frontmatter keys to visible PDF effects (cover title, watermark, TOC entries, page break before H1).

### Stage 5 — Export action UI wiring

**Goal**: user can trigger PDF export from the existing UI surfaces.

- Wherever CSV/markdown export already lives, add a parallel "Export as PDF" action.
- If `pdf.Status().Active == false`, the action is visible but clicking routes to the activation panel.
- Progress UI for long renders — picoloom's converter is fast enough that a simple toast may suffice; if not, use a slideout progress dialog.
- Error surface: backend errors round-trip via `utils/backendError.ts → backendErrMessage(err)` (project rule on Wails JSON error envelopes).

**Definition of done**: export action discoverable from at least one place; activation prompt routes correctly; success toast on completion with "Open" link.

### Stage 6 — Theme strategy

**Goal**: pick a theme exposure model and ship it.

- Decide whitelist vs. full passthrough (open question above).
- If whitelist: pick defaults (likely `default`, `technical`, `academic`), expose as a template-manifest field, allow `style: ./custom.css` for power users.
- If passthrough: dropdown in the template manifest with all 8 picoloom themes plus "Custom CSS path".
- Themes resolve via picoloom's `WithStyle()` option at converter construction time.

**Definition of done**: user can pick a theme per template; selection persists; preview-quality PDF differs visibly between themes.

### Stage 7 — Polish, batch, error UX

**Goal**: make it production-ready.

- i18n review for every user-facing string introduced.
- Error mapping: picoloom errors (`browser failed to start`, `page load timeout`, `style not found`) → typed Go errors → user-friendly i18n strings.
- Optional: `ExportAll(templateGUID)` using `picoloom.ConverterPool` with worker count from `runtime.NumCPU()`. Per-form errors collected and returned as a batch result.
- Logging: every render call emits a structured slog event with form GUID, render duration, theme, output path.
- `pdf doctor`-equivalent: a status page in the activation panel showing browser version, last successful render, last failure (if any).

**Definition of done**: known-good and known-bad documents both produce correct UX (success toast vs. actionable error); slog records render telemetry; batch export works for a multi-form template.

---

## Non-goals

- LaTeX support.
- PDF/A archival output (Chrome PDF engine doesn't do it).
- Multi-column layouts, mixed orientation, per-page headers/footers (Chrome PDF engine limits).
- Embedding non-system fonts (use Docker for cross-machine font consistency, not in scope for desktop app).
- Server-mode PDF generation (no HTTP surface; PDF stays Wails-only).
- Hot-swapping themes per page within a single document.

## References

- picoloom: <https://github.com/alnah/picoloom>
- picoloom Go reference: <https://pkg.go.dev/github.com/alnah/picoloom/v2>
- go-rod (browser layer): <https://github.com/go-rod/rod>
- Old external pipeline: `gomnirun + pandoc + eisvogel` — deprecated for Formidable.
- Project rule: one `render.Manager` per export target — see [architecture.md](architecture.md).
- Project rule: backend writes are atomic + serialized — frontmatter `feedback_atomic_writes`.
