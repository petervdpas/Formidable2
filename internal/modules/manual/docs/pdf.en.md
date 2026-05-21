# Portable Document Format (PDF) Export

Formidable exports records to PDF via **picoloom v2** - a Chromium-
based renderer with no LaTeX dependency. The export pipeline is
opt-in: nothing PDF-related runs until the user clicks **Activate**
on the Information → PDF Export panel.

## Setup

The Information → PDF Export panel drives the engine lifecycle.

1. Click **Probe** to scan for usable Chrome / Chromium binaries.
   Formidable looks at, in order:
   - `FORMIDABLE_CHROME` environment variable (explicit override)
   - Platform-conventional system paths (`/usr/bin/chromium`,
     `/Applications/…`, `Program Files/…`)
   - The latest binary in the managed-cache directory
2. Pick a candidate and click **Activate**. The picked binary is
   recorded in `pdf-state.json`; subsequent boots adopt it without
   re-probing.
3. **No Chrome found?** The managed-download flow fetches a
   standalone Chromium build into the per-user cache directory.
   It only runs on explicit request - Formidable never downloads
   binaries silently.

**Export directory** - the panel also remembers where exports land.
Empty means the system Documents folder; any non-empty value must
be an existing absolute path.

## Exporting a PDF

From the **Storage** workspace, with a record selected, open
**Export → PDF** (or use the ribbon shortcut). The Export dialog
pre-fills sensible defaults from the template and the record's
frontmatter:

- **Theme** - picoloom's bundled style (`technical`, `academic`,
  `corporate`, `legal`, …). Surfaces as the `theme:` frontmatter key.
- **Cover** - picks a cover template from the on-disk library.
  The list comes from `<AppRoot>/pdf/covers/*.html`; select
  *None* to skip the cover page.
- **Orientation** - portrait or landscape.
- **Footer position** - none, page numbers, bottom-left, etc.
- **Keywords** - comma-separated terms baked into the PDF's
  `/Keywords` metadata field for desktop indexers.

Clicking **Export** writes a `<filename>.pdf` to the chosen output
directory and shows a toast with the file path. Pre-existing files
are overwritten atomically (temp file + rename).

## Covers

Cover-page templates live under `<AppRoot>/pdf/covers/<name>.html`
and are managed from the Information → PDF Covers page.

### Cover HTML files

Each cover is a standalone HTML document with a magic header line
declaring `name:` and `description:`. The Library panel lists every
discovered cover; pick one to load it into the editor. Save
validates the document against the cover schema; structural errors
block the save and surface in the editor.

**Seed covers** (Classic, Banner, Corporate) ship with the binary
and are flagged with a **SEED** pill. They're editable; the
destructive action is phrased as **Reset** rather than Delete
because the next app start re-writes the seed from the bundled
copy if the on-disk file is missing.

### Cover images

The **Images** tab next to **Covers** maintains the binary assets
(logos, banners) that cover HTML files reference. Images live at
`<AppRoot>/pdf/covers/images/`. The seed library ships
`formidable.svg`; user uploads land alongside it.

A cover references an image by basename:

```html
<img src="formidable.svg">
```

The picoloom renderer resolves bare basenames against the images
directory at convert time, so the same cover works locally and
when shared via the archive flow below.

### Sharing a cover

The Library row has an **Export** button that bundles the cover
HTML plus every image it references (img src + CSS url()) into a
single `<name>.zip`. **Import** unpacks one of those archives;
existing covers prompt for overwrite confirmation before the
import commits.

## Frontmatter

Every export merges the template's frontmatter with the record's
own, then runs the result through the picoloom directive
processor. The Information → Help → Frontmatter Directives page
is the full reference. Common keys:

- `theme:` / `cover:` / `orientation:` - picked by the Export
  dialog; can also be hardcoded in the template.
- `keywords:` - top-level string baked into the PDF's `/Keywords`
  metadata field.
- `cover.logo:` - image path the cover template renders. Bare
  basenames resolve against `<AppRoot>/pdf/covers/images/`.

## Troubleshooting

The Information → PDF Export panel includes a **PDF Doctor**
sub-panel that surfaces structured diagnostics from the last
export. Each card is one component of the pipeline (probe,
activation, render, convert, post-process) marked success or
failure with the exact error code from the export taxonomy.
