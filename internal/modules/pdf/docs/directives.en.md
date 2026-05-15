# Picoloom frontmatter — what the renderer understands

Every directive below goes inside the YAML frontmatter block at the top of a markdown template, between two `---` lines. Anything you omit uses picoloom's built-in default. Higher merge layers (document frontmatter) override lower ones (template manifest, then global config).

Example skeleton:

```yaml
---
style: technical

cover:
  title: My Document
  author: Alice

toc:
  enabled: true
  maxDepth: 3
---
# Body starts here
```

## Style

| Key | What it does |
| --- | --- |
| `style` | Theme name (`default`, `technical`, `academic`, `corporate`, `legal`, `invoice`, `manuscript`, `creative`) **or** a path to a custom `.css` file. |

## Page

| Key | What it does |
| --- | --- |
| `page.size` | `letter`, `a4`, or `legal`. |
| `page.orientation` | `portrait` or `landscape`. |
| `page.margin` | Uniform margin in inches (0.25 – 3.0). |

## Cover page

| Key | What it does |
| --- | --- |
| `cover.enabled` | Set to `false` to suppress the cover page even when other cover fields are present. Defaults to on when the block is present. |
| `cover.title` | Cover title. `{{form.x}}` placeholders are expanded by Raymond before picoloom strips the frontmatter. |
| `cover.subtitle` | Optional subtitle under the title. |
| `cover.author` | Author name shown on the cover. |
| `cover.organization` | Organization / department name. |
| `cover.date` | Literal date string, or one of `iso`, `european`, `us`, `long`, `auto:FORMAT`. |
| `cover.logo` | Path to a logo image rendered on the cover (absolute or template-relative). |
| `cover.documentID` | Reference code (e.g. `DOC-2026-001`). |

## Table of contents

| Key | What it does |
| --- | --- |
| `toc.enabled` | Set to `false` to suppress the table of contents. Defaults to on when the block is present. |
| `toc.title` | Heading text for the TOC page. Empty = no title. |
| `toc.minDepth` | Lowest heading level to include (1 – 6, default 2 — skips H1). |
| `toc.maxDepth` | Highest heading level to include (1 – 6, default 3). |

## Footer

| Key | What it does |
| --- | --- |
| `footer.enabled` | Set to `false` to suppress the footer. |
| `footer.position` | `left`, `center`, or `right` (default `right`). |
| `footer.showPageNumber` | `true` to print the page number in the footer. |
| `footer.text` | Free-form footer text (e.g. `© Fontys`). |
| `footer.documentID` | Reference code shown in the footer. |

## Watermark

| Key | What it does |
| --- | --- |
| `watermark.enabled` | Set to `true` to render a diagonal text watermark behind the content. |
| `watermark.text` | Watermark text, e.g. `DRAFT`, `CONFIDENTIAL`. |
| `watermark.color` | Hex color (`#RGB` or `#RRGGBB`; default `#888888`). |
| `watermark.opacity` | 0.0 – 1.0 (default 0.1). |
| `watermark.angle` | Rotation in degrees (−90 – 90, default −45). |

## Signature block

| Key | What it does |
| --- | --- |
| `signature.enabled` | Set to `true` to render a signature block at the end of the document. |
| `signature.name` | Signer's name. |
| `signature.email` | Signer's email. |
| `signature.imagePath` | Path to a signature image (PNG/JPG). |
| `signature.links` | List of clickable links: `[{ label, url }, …]`. |

## Page breaks

| Key | What it does |
| --- | --- |
| `pageBreaks.enabled` | Set to `false` to disable all heading-based page breaks. |
| `pageBreaks.beforeH1` | `true` to force a page break before every H1. |
| `pageBreaks.beforeH2` | `true` to force a page break before every H2. |
| `pageBreaks.beforeH3` | `true` to force a page break before every H3. |
| `pageBreaks.orphans` | Min lines at the bottom of a page (1 – 5, default 2). |
| `pageBreaks.widows` | Min lines at the top of a page (1 – 5, default 2). |
