# Settings

Formidable groups its configuration into **profiles**. Each profile is
an independent workspace with its own templates, storage, plugins, and
preferences. Switch profiles from the bottom-right user menu — the app
reloads to the picked profile's context.

## Where settings live

Profiles live in the per-user application data directory. Each profile
is a folder containing:

| Path                       | Purpose                                                      |
| -------------------------- | ------------------------------------------------------------ |
| `config.json`              | The full settings record for this profile.                   |
| `templates/`               | YAML template definitions discovered on startup.             |
| `storage/<template>/`      | Per-template record files (`.md`, `.meta.json`).             |
| `plugins/`                 | Plugin folders, see the Plugins manual page.                 |
| `pdf/covers/`              | User-authored PDF cover archives.                            |

The active profile name shows in the title bar; the path is
discoverable via the Information → About panel.

## Common settings

A handful of fields you'll touch most often:

- **Theme** — light / dark / system.
- **Language** — picks the locale for the UI and for plugin i18n
  resolution.
- **Enable plugins** — global kill-switch. Off hides the Plugins
  workspace and skips plugin discovery.
- **Logging enabled** — writes a rolling log to disk; the Information
  → Logging panel tails it live.
- **Enabled templates** — curates which templates appear in the
  Storage workspace; an empty list means "all of them."
- **Show paste buttons** — surfaces a paste-from-clipboard icon next
  to text/textarea fields.
- **Author name** + **Author email** — used as the default identity
  on new records and on git commits made through the Sync workspace.
- **Context mode / ribbon / folder** — selects the active workspace
  context on startup.

The full list is rendered as a form in the Profiles workspace.

## Internal server

A small HTTP server can be enabled per profile to host the wiki + REST
API on a local port. Plugins that need `formidable.api.fetch` require
this. See the Information → Internal Server panel for status and
toggles.

## Git and Gigot

Each profile carries its own remote-backend settings:

- **Git** — points at a remote repo over HTTPS or SSH; credentials
  live in the keychain.
- **Gigot** — Formidable's lightweight ledger-based sync, addressed
  by a base URL + per-profile subscription token.

These are independent — a profile picks one or neither.

## Saving and resets

Settings save atomically (temp file + fsync + rename) so a crash
mid-save can never corrupt `config.json`. To reset a single field,
clear it in the Profiles workspace; to reset everything for a fresh
profile, delete the profile folder and let the app recreate it on
next launch.
