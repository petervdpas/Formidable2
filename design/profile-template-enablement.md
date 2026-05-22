# Profile-Level Template Enablement - Design

Each profile carries a list of templates the user has opted into; "use-side" surfaces (template pickers) filter to that list. Templates are *not* deleted, hidden globally, or categorised - this is per-profile curation, peer to existing `SelectedTemplate`.

See [architecture.md](architecture.md) for module conventions. No new module; this slots into the existing `config` and `template` modules.

---

## Decisions (settled)

| Topic | Decision |
|---|---|
| Identifier | Template **filename** (e.g. `"basic.yaml"`). `Filename` is stable; `Name` is user-editable. Templates have no top-level GUID. |
| Storage | New field `EnabledTemplates []string` on `config.Config`, JSON tag `enabled_templates`. Lives in `internal/modules/config/types.go`. |
| Default | `nil` / empty slice → **all templates enabled**. Existing profiles keep working untouched; feature is opt-in. Once the slice has at least one entry, it becomes authoritative. |
| Deletion behaviour | If a template referenced by `EnabledTemplates` is deleted, the entry is silently pruned the next time the list is read/written. No error, no toast. The list is advisory, not a foreign-key constraint. |
| `SelectedTemplate` interaction | Independent. If `SelectedTemplate` falls outside `EnabledTemplates`, the picker simply shows "no selection" - we do **not** auto-mutate `SelectedTemplate`. User notices and re-picks. |
| Filter seam | `template.LoadMany([]string)` already takes a filename slice. The filter is applied *before* calling it, via a pure helper on `config.Manager`. No new Wails surface required for the filter itself. |
| Editor visibility | `TemplatesWorkspace` (the editor) is **not** filtered - it's where you manage templates, you must see all of them. Only use-side pickers filter. |
| Settings surface | New tab `templates` in `frontend/src/workspaces/settings/index.ts`, file `SettingsTemplates.vue`. Toggle-per-row using `SwitchField`. Search box only (no reorder, no grouping). |
| i18n namespace | `settings.templates.*`. New file `internal/modules/i18n/locales/<lang>/settings.json` block, mirrored across all locales. |
| Boolean UI | `SwitchField` / `FormSwitchRow` per row. Never `<input type="checkbox">` (project convention). |

## Out of scope (v1)

- Filtering the templates editor / sidebar
- Auto-clearing `SelectedTemplate` when its template gets disabled
- Reordering, drag-and-drop, grouping, categories, clusters
- Bulk "enable all in folder" / "disable all" actions
- Migrating Electron-Formidable profile data
- Surfacing the enablement list through the `wiki` or `api` modules

These can come back as separate work if real usage demands them.

---

## Build order

Backend first per project convention; tests before code per project TDD rule.

### 1. Config field + default

- `internal/modules/config/types.go` - add `EnabledTemplates []string` with `json:"enabled_templates,omitempty"`.
- `internal/modules/config/defaults.go` - leave nil (matches "empty = all enabled" semantics; `omitempty` keeps existing user.json files clean).
- `internal/modules/config/domain_test.go` - unit tests:
  - default value is nil
  - JSON round-trip preserves a populated slice
  - JSON round-trip of a profile saved before this field exists deserialises with nil

### 2. Filter helper

Pure methods on `config.Manager`:

```go
// True iff the template should be visible in use-side surfaces.
// Empty/nil EnabledTemplates means "all enabled".
func (m *Manager) IsTemplateEnabled(filename string) bool

// Returns the input slice filtered to only enabled entries.
// Same empty-list semantics: empty config slice → input returned unchanged.
func (m *Manager) FilterEnabled(filenames []string) []string

// Removes filenames no longer present in `existing` from EnabledTemplates.
// Called from the templates module after a delete.
func (m *Manager) PruneEnabledTemplates(existing []string) (removed []string, err error)
```

Unit tests in `domain_test.go` for all three, including the prune-on-delete path.

### 3. Godog scenarios

`internal/modules/config/features/config.feature` - add scenarios:

- empty list → every template is reported enabled
- populated list → only listed templates are reported enabled
- prune after delete removes the stale entry and persists
- round-trip through save/load preserves the slice

### 4. Hook deletion into prune

When `template.Manager.Delete(filename)` succeeds, call `config.Manager.PruneEnabledTemplates(...)` with the current template filename set. Cross-module call goes through the existing service wiring, not a new bus.

Godog scenario in the template feature file: "deleting a template prunes it from the active profile's enabled list".

### 5. Wails exposure (frontend reads)

Frontend needs to:
- read `EnabledTemplates` (already available via existing `Config.Get()` - no new binding)
- write it (already available via existing `Config.Set()` style update - confirm path during implementation)

No new service methods unless the existing config service can't write a slice field - check first, add only if necessary.

### 6. Settings UI

`frontend/src/workspaces/settings/SettingsTemplates.vue`:

- Fetch all templates (`TemplateSvc.List()` or equivalent)
- Render one `FormSwitchRow` per template, bound to `enabled_templates` membership
- Search box filters the *visible rows* (does not affect enablement)
- Empty-state copy explains "no templates enabled = all enabled"

Register in `frontend/src/workspaces/settings/index.ts` between `general` and `history`.

Per-row toggle writes the whole `enabled_templates` slice back through the config save path - keeps it simple, no incremental add/remove API.

### 7. Wire `StorageWorkspace.vue`

Wherever the template list is consumed for the active-template dropdown, intersect with `enabled_templates` (or pass through unchanged when the slice is empty). The `useTemplates` composable is the natural seam.

No change to `TemplatesWorkspace` (editor).

### 8. i18n strings

Add to `settings.json` (all locales):

- `settings.templates.title`
- `settings.templates.search_placeholder`
- `settings.templates.empty_means_all`
- `settings.templates.no_templates_found`

---

## Risks / non-risks

- **Risk: stale `SelectedTemplate`** - handled by leaving the field alone and letting the picker show "no selection". Low-impact, recoverable in one click.
- **Risk: enablement drift across profiles** - by design. Each profile curates its own working set.
- **Non-risk: data loss on delete** - the enablement list is advisory; pruning the stale entry is the correct behaviour, not a regression.
- **Non-risk: backwards compatibility** - `omitempty` + nil-means-all means existing profiles need no migration.
