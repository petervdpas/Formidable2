# User Settings

Every profile carries its own settings record, stored as `config.json`
in the profile folder. The Profiles workspace renders that record as
a form; the User Profiles page covers the per-profile folder layout
and how to switch between them.

## Common fields

A handful of settings you'll touch most often:

- **Theme**: light / dark / system.
- **Language**: picks the locale for the UI and for plugin i18n
  resolution.
- **Enable plugins**: global kill-switch. Off hides the Plugins
  workspace and skips plugin discovery.
- **Logging enabled**: writes a rolling log to disk; the Information
  → Logging panel tails it live.
- **Enabled templates**: curates which templates appear in the
  Storage workspace; an empty list means "all of them."
- **Show paste buttons**: surfaces a paste-from-clipboard icon next
  to text/textarea fields.
- **Author name** + **Author email**: used as the default identity
  on new records and on git commits made through the Sync workspace.
- **Context mode / ribbon / folder**: selects the active workspace
  context on startup.

## Internal server

A small HTTP server can be enabled per profile to host the wiki + REST
API on a local port. Plugins that need `formidable.api.fetch` require
this. See the Information → Internal Server panel for status and
toggles.

## Git and Gigot

Each profile carries its own remote-backend settings:

- **Git**: points at a remote repo over HTTPS or SSH; credentials
  live in the keychain.
- **Gigot**: Formidable's lightweight ledger-based sync, addressed
  by a base URL + per-profile subscription token.

These are independent. A profile picks one or neither.

## Saving and resets

Settings save atomically (temp file + fsync + rename) so a crash
mid-save can never corrupt `config.json`. To reset a single field,
clear it in the Profiles workspace; to reset everything for a fresh
profile, see User Profiles for how to recreate the profile folder.
