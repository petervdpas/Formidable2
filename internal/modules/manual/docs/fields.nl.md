# Velden

Een **veld** is één element in de `fields:`-lijst van een template.
Elk veld heeft een stabiele `key`, een `type` uit de matrix
hieronder, en een set attributen waar het type op intekent.

## Algemene attributen

| Attribuut        | Doel                                                              |
| ---------------- | ----------------------------------------------------------------- |
| `key`            | Stabiele identifier; gebruikt in Handlebars, `meta.json`, expressies. |
| `type`           | Eén van de type-id's hieronder.                                   |
| `label`          | Weergavelabel in het formulier.                                   |
| `description`    | Helptekst onder het veld.                                         |
| `default`        | Voorgevulde waarde op nieuwe records.                             |
| `primary_key`    | Markeert het veld dat het record identificeert.                   |
| `summary_field`  | Markeert een veld dat in de opslag-zijbalk-sublabel verschijnt.   |
| `two_column`     | Toont het veld in een twee-koloms rij wanneer gekoppeld aan een ander. |
| `readonly`       | Schakelt bewerken uit in het runtime-formulier.                   |
| `collapsible`    | Container-types — vouwt children weg achter een kop.              |
| `expression_item`| Beschikbaar als variabele in de expression-builder.               |
| `format`         | Type-specifieke format-hint (bijv. datum, nummer).                |
| `options`        | Type-specifieke lijst (dropdown-keuzes, file-patronen, enz.).     |

Niet elk attribuut geldt voor elk type — de Designer verbergt rijen
die niet van toepassing zijn, en de backend stript verboden waarden
bij opslaan.

## Type-matrix

| Type           | Doel                                                               |
| -------------- | ------------------------------------------------------------------ |
| `text`         | Eén regel tekst.                                                   |
| `textarea`     | Meerregelige tekst met optionele Markdown-editor.                  |
| `number`       | Numerieke invoer.                                                  |
| `range`        | Schuifregelaar; ondersteunt `min`/`max`/`step` in `options`.       |
| `date`         | Datumkiezer met gelokaliseerde opmaak.                             |
| `boolean`      | Toggle met aangepaste True/False-labels in `options`.              |
| `dropdown`     | Enkelvoudige keuze uit `options`.                                  |
| `multioption`  | Meerkeuze-vinkjes uit `options`.                                   |
| `radio`        | Wederzijds uitsluitende radioknoppen uit `options`.                |
| `file-path`    | Bestandskiezer; `options`-patronen filteren de dialoog.            |
| `folder-path`  | Mapkiezer.                                                         |
| `list`         | Vrije herhaalbare stringlijst (chips).                             |
| `table`        | Bewerkbaar raster; kolomtypes inclusief `reference` voor record-links. |
| `image`        | Afbeelding-upload; biedt `{{imageURL}}` / `{{imageBase64}}`.       |
| `link`         | Vrije URL met optioneel label.                                     |
| `tags`         | Tag-chips gekoppeld aan `meta.tags`; maximaal één per template.    |
| `api`          | Cross-template lookup via de REST `/api/<tpl>/{id}`-route.         |
| `guid`         | Automatisch gegenereerde record-id; vereist door `enable_collection`. |
| `looper`       | Marker — declareert een herhaalbare groep hieronder.               |
| `loopstart`    | Marker — opent een loop-body.                                      |
| `loopstop`     | Marker — sluit een loop-body.                                      |

## Loops

Een loop is een herhaalbare groep velden. Open hem met een
`loopstart`-entry waarvan de `key` de loopnaam is; sluit af met een
bijbehorende `loopstop`. Elk veld tussen die twee hoort bij de
loop-body en rendert per iteratie.

```yaml
fields:
  - key: chapters
    type: loopstart
    label: "Hoofdstukken"
    summary_field: chapter_title
  - key: chapter_title
    type: text
    label: "Hoofdstuk-titel"
  - key: chapter_body
    type: textarea
    label: "Hoofdstuk-tekst"
  - key: chapters
    type: loopstop
    label: "Hoofdstukken"
```

`summary_field` op `loopstart` kiest één van de binnenste velden om
elke iteratie samen te vatten in de form-runtime — de waarde van
dat veld wordt het label van de ingeklapte rij.

### Geneste loops

Een `loopstart` binnen een andere `loopstart` opent een geneste
loop. De maximale diepte is **2**: een child-loop binnen een
parent-loop is OK; een grandchild-loop wordt afgekeurd met
`excessive-loop-nesting`.

```yaml
fields:
  - key: chapters
    type: loopstart
    label: "Hoofdstukken"
    summary_field: chapter_title
  - key: chapter_title
    type: text
  - key: sections                    # binnenste loop opent hier
    type: loopstart
    label: "Secties"
    summary_field: section_title
  - key: section_title
    type: text
  - key: section_body
    type: textarea
  - key: sections                    # binnenste loop sluit
    type: loopstop
  - key: chapters                    # buitenste loop sluit
    type: loopstop
```

De Markdown-template rendert geneste loops met geneste
`{{#loop "naam"}} ... {{/loop}}`-blokken; in de body is een
automatisch gegenereerde `<loopnaam>_index` beschikbaar. Helpers
`{{loopItemBefore}}` / `{{loopItemAfter}}` worden uitgebreid tot de
omringende separator.

## Plugin-veld-i18n

Een plugin-formulierveld kan een `i18n:`-basis-sleutel declareren om
vertaling te activeren:

```yaml
- key: schema
  type: text
  label: "Schema"
  description: "DB-schema."
  i18n: form.schema
```

De renderer resolvet `<plugin-namespace>.form.schema.label` /
`.description` / `.placeholder` tegen het locale-bestand van de
plugin. Zie de Plugins-handleidingpagina voor de volledige
conventie.

## Validatie

Validatie bij opslaan dwingt af:

- Unieke `key` per template.
- Exact nul of één `primary_key`-veld.
- `tags`-veld aantal ≤ 1.
- `enable_collection` vereist één `guid`-veld.
- Loop-paring (`loopstart` ↔ `loopstop`) per `looper`.
- `api`-veldvorm (doel-template + lookup-veld).

Fouten verschijnen in de Designer met het problematische veld
gemarkeerd; er wordt niets naar schijf geschreven zolang een
validatieprobleem niet is opgelost.
