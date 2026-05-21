# User Profiles

A **profile** is an isolated workspace: its own templates, storage,
plugins, PDF covers, git/gigot remotes, and `config.json`. Switching
profiles reloads the app into the picked profile's context - nothing
crosses between them.

## Where profiles live

Profiles live under the per-user application data directory. Each
profile is a folder containing:

| Path                       | Purpose                                                      |
| -------------------------- | ------------------------------------------------------------ |
| `config.json`              | The full settings record for this profile.                   |
| `templates/`               | YAML template definitions discovered on startup.             |
| `storage/<template>/`      | Per-template record files (`.md`, `.meta.json`).             |
| `plugins/`                 | Plugin folders, see the Customize via Plugins page.          |
| `pdf/covers/`              | User-authored PDF cover archives.                            |

The active profile name shows in the title bar; the on-disk path is
discoverable via the Information → About panel.

## Switching profiles

The bottom-right user menu lists every detected profile. Picking one
triggers a context reload: composables drop their caches, storage and
templates re-read from the new folder, and the workspace returns to a
clean state under the new identity.

## Creating and deleting profiles

The Profiles workspace (sidebar icon) is where you author new profiles
and tweak existing ones. Each profile gets the full settings form;
fields you don't touch fall back to the application defaults.

To reset a single profile, clear the relevant fields in the form. To
nuke a profile entirely, close the app and delete the folder - the
next launch recreates it empty.

## Why isolation matters

Per-profile templates and storage mean you can keep a "work" profile
and a "personal" profile that share zero state. Per-profile plugins
mean a plugin installed for one profile isn't visible to the others.
Per-profile remotes mean a profile can target one git repo or one
gigot subscription without leaking credentials across contexts.
