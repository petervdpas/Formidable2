# Template & Design

A **template** is a YAML file that declares the shape of one kind of
record: what fields it carries, how the form looks, and how it
renders to Markdown or PDF. Templates live under
`<profile>/templates/<name>.yaml` and are discovered on startup.

## YAML shape

```yaml
name: "Note"
filename: "{{title}}"
item_field: "title"
markdown_template: |
  # {{title}}

  {{body}}
enable_collection: false
fields:
  - key: title
    type: text
    label: "Title"
    primary_key: true
  - key: body
    type: textarea
    label: "Body"
```

Top-level fields:

- **name**: display name shown in the sidebar.
- **filename**: Handlebars expression resolved against the record
  to produce its on-disk filename.
- **item_field**: which field's value summarises the record in the
  storage list.
- **markdown_template**: Handlebars template rendered when the
  record is exported or piped through the wiki / API.
- **enable_collection**: when true the template carries multiple
  records keyed by a `guid` field; off means a single-document
  template.
- **facets**: multi-dimensional meta-tags; see the Templates
  workspace's Facets panel.
- **pdf**: optional PDF export config (cover, style).
- **fields**: the ordered list of fields. See the Fields manual
  page for the per-type reference.

## Authoring

The Templates workspace is the main editor:

- **Designer**: drag and drop fields, inspect the type matrix, edit
  per-field properties.
- **Markdown template**: Handlebars editor with live preview.
- **Facets**: palette + limits per facet.
- **PDF**: picks the cover archive and style.

Save is atomic and only writes files whose contents actually changed
- other files in the template folder are left untouched.

## Loops

A `loopstart` / `loopstop` pair declares a repeatable group of
fields. Nesting is supported up to depth 2. See the **Fields**
manual page for the full authoring pattern with a worked nested
example.

## Enabled templates

The active profile may curate which templates appear in the Storage
workspace via the **enabled_templates** list. Templates the profile
doesn't enable stay on disk and still serve through the REST `api`
field type. The curation is a UI scope, not a security boundary.

## Template generator

The Templates workspace ships a **New template** dialog that
materialises a starter template by shape (report, minimal, table,
frontmatter) × image mode (URL / inline) × wrap-loops toggle. The
selected toggles produce visible source. There is no invisible
runtime magic.

## Where records live

Records produced by a template live under
`<profile>/storage/<template>/`. Each record is a `.md` file with
YAML frontmatter; collection templates pair every record with a
`.meta.json` sidecar holding tags, facets, and audit identity.
