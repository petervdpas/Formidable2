# Sjabloon & Ontwerp

Een **sjabloon** is een YAML-bestand dat de vorm van één soort
record beschrijft - welke velden, hoe het formulier eruitziet, en
hoe het naar Markdown of PDF rendert. Sjablonen leven onder
`<profile>/templates/<naam>.yaml` en worden bij start ontdekt.

## YAML-vorm

```yaml
name: "Notitie"
filename: "{{title}}"
item_field: "title"
markdown_template: |
  # {{title}}

  {{body}}
enable_collection: false
fields:
  - key: title
    type: text
    label: "Titel"
    primary_key: true
  - key: body
    type: textarea
    label: "Inhoud"
```

Top-niveau velden:

- **name** - weergavenaam in de zijbalk.
- **filename** - Handlebars-expressie tegen het record voor de
  bestandsnaam op schijf.
- **item_field** - welk veld het record samenvat in de
  opslag-lijst.
- **markdown_template** - Handlebars-sjabloon dat rendert bij
  export, wiki of API.
- **enable_collection** - aan = meerdere records gekoppeld via een
  `guid`-veld; uit = één document per sjabloon.
- **facets** - multi-dimensionele meta-tags; zie het Facets-paneel
  in de Sjablonen-werkruimte.
- **pdf** - optionele PDF-exportconfiguratie (cover, stijl).
- **fields** - geordende lijst van velden. Zie de
  Velden-handleidingpagina voor de per-type-referentie.

## Auteur-werkwijze

De Sjablonen-werkruimte is de hoofdeditor:

- **Designer** - sleep en plaats velden, bekijk de type-matrix,
  bewerk eigenschappen per veld.
- **Markdown-sjabloon** - Handlebars-editor met live preview.
- **Facets** - palet + limieten per facet.
- **PDF** - kies cover-archief en stijl.

Opslaan is atomair en schrijft alleen bestanden waarvan de inhoud
echt veranderd is - andere bestanden in de sjabloon-map blijven met
rust.

## Loops

Een `loopstart` / `loopstop`-paar declareert een herhaalbare groep
velden. Nesting tot diepte 2 wordt ondersteund. Zie de
**Velden**-handleidingpagina voor het volledige authoring-patroon
met een uitgewerkt nested-voorbeeld.

## Ingeschakelde sjablonen

Het actieve profiel kan via **enabled_templates** bepalen welke
sjablonen in de Opslag-werkruimte verschijnen. Sjablonen die het
profiel niet activeert blijven op schijf staan en blijven
bereikbaar via het REST `api`-veldtype - deze curatie is een
UI-scope, geen beveiligingsgrens.

## Sjabloon-generator

De Sjablonen-werkruimte heeft een **Nieuw sjabloon**-dialoog die
een startsjabloon genereert op basis van vorm (report, minimal,
table, frontmatter) × image-modus (URL / inline) × wrap-loops
toggle. De gekozen toggles produceren zichtbare bron - geen
onzichtbare runtime-magie.

## Waar records leven

Records van een sjabloon leven onder
`<profile>/storage/<template>/`. Elk record is een `.md`-bestand
met YAML-frontmatter; collection-sjablonen koppelen elk record aan
een `.meta.json`-sidecar met tags, facets en audit-identiteit.
