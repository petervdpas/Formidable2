# Picoloom frontmatter — wat de renderer begrijpt

Onderstaande directives plaats je in het YAML-frontmatterblok bovenaan een markdown-template, tussen twee `---`-regels. Wat je weglaat valt terug op picoloom's ingebouwde standaard. Hogere merge-lagen (document-frontmatter) overschrijven lagere lagen (template-manifest, vervolgens globale config).

Voorbeeldskelet:

```yaml
---
style: technical

cover:
  title: Mijn Document
  author: Alice

toc:
  enabled: true
  maxDepth: 3
---
# Hier begint de body
```

## Stijl

| Key | Wat het doet |
| --- | --- |
| `style` | Themanaam (`default`, `technical`, `academic`, `corporate`, `legal`, `invoice`, `manuscript`, `creative`) **of** een pad naar een eigen `.css`-bestand. |

## Pagina

| Key | Wat het doet |
| --- | --- |
| `page.size` | `letter`, `a4` of `legal`. |
| `page.orientation` | `portrait` of `landscape`. |
| `page.margin` | Uniforme marge in inches (0.25 – 3.0). |

## Voorpagina

| Key | Wat het doet |
| --- | --- |
| `cover.enabled` | Zet op `false` om de voorpagina te onderdrukken, ook als andere cover-velden gezet zijn. Standaard aan als het blok aanwezig is. |
| `cover.title` | Titel op de voorpagina. `{{form.x}}`-placeholders worden door Raymond geëxpandeerd voordat picoloom de frontmatter weghaalt. |
| `cover.subtitle` | Optionele subtitel onder de titel. |
| `cover.author` | Auteur op de voorpagina. |
| `cover.organization` | Organisatie- of afdelingsnaam. |
| `cover.date` | Letterlijke datumstring, of een van `iso`, `european`, `us`, `long`, `auto:FORMAT`. |
| `cover.logo` | Pad naar een logo dat op de voorpagina wordt getoond (absoluut of template-relatief). |
| `cover.documentID` | Referentiecode (bv. `DOC-2026-001`). |

## Inhoudsopgave

| Key | Wat het doet |
| --- | --- |
| `toc.enabled` | Zet op `false` om de inhoudsopgave te onderdrukken. Standaard aan als het blok aanwezig is. |
| `toc.title` | Koptekst boven de inhoudsopgave. Leeg = geen titel. |
| `toc.minDepth` | Laagste kopniveau om op te nemen (1 – 6, standaard 2 — slaat H1 over). |
| `toc.maxDepth` | Hoogste kopniveau om op te nemen (1 – 6, standaard 3). |

## Voettekst

| Key | Wat het doet |
| --- | --- |
| `footer.enabled` | Zet op `false` om de voettekst te onderdrukken. |
| `footer.position` | `left`, `center` of `right` (standaard `right`). |
| `footer.showPageNumber` | `true` om het paginanummer in de voettekst te tonen. |
| `footer.text` | Vrije tekst in de voettekst (bv. `© Fontys`). |
| `footer.documentID` | Referentiecode in de voettekst. |

## Watermerk

| Key | Wat het doet |
| --- | --- |
| `watermark.enabled` | Zet op `true` om een diagonaal tekstwatermerk achter de inhoud te renderen. |
| `watermark.text` | Watermerktekst, bv. `CONCEPT`, `VERTROUWELIJK`. |
| `watermark.color` | Hex-kleur (`#RGB` of `#RRGGBB`; standaard `#888888`). |
| `watermark.opacity` | 0.0 – 1.0 (standaard 0.1). |
| `watermark.angle` | Rotatie in graden (−90 – 90, standaard −45). |

## Ondertekeningsblok

| Key | Wat het doet |
| --- | --- |
| `signature.enabled` | Zet op `true` om aan het einde van het document een ondertekeningsblok te renderen. |
| `signature.name` | Naam van de ondertekenaar. |
| `signature.email` | E-mail van de ondertekenaar. |
| `signature.imagePath` | Pad naar een afbeelding van de handtekening (PNG/JPG). |
| `signature.links` | Lijst met klikbare links: `[{ label, url }, …]`. |

## Pagina-einden

| Key | Wat het doet |
| --- | --- |
| `pageBreaks.enabled` | Zet op `false` om alle kop-gebaseerde pagina-einden uit te zetten. |
| `pageBreaks.beforeH1` | `true` forceert een pagina-einde voor elke H1. |
| `pageBreaks.beforeH2` | `true` forceert een pagina-einde voor elke H2. |
| `pageBreaks.beforeH3` | `true` forceert een pagina-einde voor elke H3. |
| `pageBreaks.orphans` | Minimum aantal regels onderaan een pagina (1 – 5, standaard 2). |
| `pageBreaks.widows` | Minimum aantal regels bovenaan een pagina (1 – 5, standaard 2). |
