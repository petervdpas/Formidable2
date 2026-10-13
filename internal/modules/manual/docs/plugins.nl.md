# Aanpassen via Plugins

Plugins breiden Formidable uit met door de gebruiker geschreven
Lua-scripts. Een plugin is een map onder `<AppRoot>/plugins/<id>/`
met:

| Bestand                      | Doel                                                          |
| ---------------------------- | ------------------------------------------------------------- |
| `plugin.json`                | Manifest: id, naam, versie, commando's, run-modus.            |
| `main.lua`                   | Lua-broncode. Elk commando is een globale functie.           |
| `form.json`                  | Optioneel invoerformulier wanneer `run_mode: "form"`.         |
| `i18n/<locale>.json`         | Optionele vertalingen per taal (zie hieronder).               |

## Waar plugins staan

De Plugins-werkruimte toont elke map onder `<AppRoot>/plugins/`.
Met Formidable meegeleverde plugins (zoals `test-plugin`) worden bij
de eerste start uit de binary geschreven; zie "Upgrades en
opnieuw seeden" hieronder.

## i18n authoring

Voeg een `i18n`-sleutel toe aan een formulierveld om vertaling te
activeren en lever per-taal-bestanden `<plugin>/i18n/<locale>.json`
mee. Sleutels krijgen tijdens runtime de prefix `plugin.<id>.`.

```yaml
- key: schema
  type: text
  label: "Schema"
  description: "DB-schema."
  i18n: form.schema
```

```json
{
  "form.schema.label": "Schema",
  "form.schema.description": "Database-schema."
}
```

Topniveau-sleutels `name`, `description` en `commands.<id>.label`
worden gebruikt voor naam, beschrijving en commando-labels van de
plugin.

De Plugins-werkruimte heeft een **i18n**-tabblad om taalbestanden te
bewerken zonder de app te verlaten.

## i18n vanuit Lua

```lua
local label = formidable.i18n.t("form.schema.label")
```

De actieve taal komt uit het gebruikersprofiel; ontbrekende sleutels
vallen terug op de letterlijke sleuteltekst.

## Upgrades en opnieuw seeden

De scaffold schrijft seed-bestanden alleen weg **als het doelbestand
ontbreekt**. Eigen wijzigingen worden nooit overschreven. Om één
bestand opnieuw te seeden (bijvoorbeeld na een upgrade met een
nieuw `form.json`-schema), verwijder het van schijf en herstart. De
meegeleverde versie wordt teruggeschreven.

Om een hele meegeleverde plugin opnieuw te seeden, verwijder de map
onder `<AppRoot>/plugins/<id>/` en herstart.

## Bewerken tijdens ontwikkeling

De Plugins-werkruimte bevat een editor voor elk plugin-bestand:

- **Manifest**: naam, beschrijving, run-modus, commando's.
- **Lua-broncode**: `main.lua`, Ctrl+Enter voor fullscreen.
- **Formulier-editor**: velden bij `run_mode: "form"`.
- **i18n**: sleutel/waarde-tabel per taal; wissel via de chips.

Opslaan persisteert alle gewijzigde bestanden in één atomaire actie;
de rest van de app pikt de wijzigingen op bij de volgende refresh.

## API-clients in een plugin

Een plugin kan een API-client meenemen en die vanuit het script gebruiken. Op
het tabblad **API-clients** van de plugin geef je een client een naam en een
OpenAPI- of Swagger-document, geplakt of uit een bestand geladen.

- **Weergavenaam** en **Basis-URL** zijn optioneel. Blijven ze leeg, dan krijgt
  de client de titel en de server uit het document.
- **Opslaan in plugin** bewaart het document in de plugin, zodat het meereist
  in de plugin-zip. Opslaan maakt geen client aan.
- **API-client aanmaken** maakt de client op deze machine. De plugin-zip
  importeren op een andere machine doet daar hetzelfde. Een bestaande client
  wordt nooit overschreven: verwijder hem onder Informatie > API-clients om hem
  opnieuw aan te maken.
- Inloggegevens reizen nooit mee met een plugin. Vul ze in onder Informatie >
  API-clients nadat de client is aangemaakt.

Het script bereikt alleen de clients die de plugin noemt. De aanroepen:

| Aanroep | Wat hij doet |
|---|---|
| `formidable.client.list()` | De clients van de plugin. |
| `formidable.client.operations(id)` | Elke operatie in het document, met methode, parameters en request-body. |
| `formidable.client.call(id, operation, params, body, content_type)` | Voert één operatie uit op basis van haar id, ongeacht de methode. |
| `formidable.client.items(id, resource, opts)` en `fetch(id, resource, item_id, opts)` | Lezen via een resource-binding. |
| `formidable.client.snapshot(item, keys)` | De waarde die een api-client-veld voor een item opslaat. |
| `formidable.client.exists(id)` en `create(id, opts)` | Laten het script de client aanmaken uit een document in de pluginmap. |

`call` verstuurt de methode die de operatie noemt: GET, POST, PUT, PATCH,
DELETE en de rest. Een tabel als `body` gaat als JSON, of als formulier wanneer
de operatie alleen dat accepteert. Een string gaat exact zoals hij is, dus een
bestand dat met `formidable.fs.read(path)` is gelezen komt byte voor byte aan;
geef het type mee als `content_type` (bijvoorbeeld `application/pdf`), tenzij
de operatie het zelf noemt. Multipart-uploads worden niet ondersteund. Een
mislukte aanroep geeft een fout die begint met de code, zoals `forbidden` of
`timeout`.

`formidable.client.attempt` neemt dezelfde argumenten en geeft geen fout bij
een mislukte aanroep. Het geeft `result, nil` of `nil, err` terug, waarbij
`err` de velden `code`, `message`, `status`, `delivery`, en `body` en `data`
met het antwoord van de dienst bevat. `delivery` is `not_sent`, `answered` of
`unknown`. Gebruik het voor een schrijvende aanroep: het script kan lezen
waarom de dienst weigerde, en weet na een time-out dat de uitkomst onbekend
is, zodat het de dienst kan vragen voordat het opnieuw verstuurt. Formidable
verstuurt een schrijvende aanroep zelf nooit twee keer: een redirect op zo'n
aanroep wordt niet gevolgd en komt terug als de fout `redirected`.

## PDF vanuit een plugin

`formidable.pdf.export(template, datafile, opts)` maakt de PDF van één
formulier, dezelfde export als in de werkruimte Opslag, en geeft `path`,
`bytes` en `duration_ms` terug. `opts` is optioneel en kent `output_path`,
`style`, `cover_template`, `disable_cover` en `disable_theme`.
`formidable.pdf.active()` zegt of PDF-export is ingesteld.

PDF-export vereist Chrome of Chromium en wordt geactiveerd onder Informatie >
PDF. Is dat niet gebeurd, dan geeft `export` een fout die begint met
`engine_inactive`. Zet **Vereist PDF-export** aan onder Gedrag om de run in dat
geval vooraf te laten weigeren.

## Sjablonen in een plugin

Een plugin kan eigen sjablonen bevatten, zodat het script ergens kan opslaan
wat het ophaalt. Op het tabblad **Meegeleverde sjablonen** van de plugin:

- **Sjabloon toevoegen** neemt een sjabloon over uit de werkruimte Sjablonen,
  of begint een nieuw op basis van YAML die je typt.
- **Opslaan in plugin** bewaart de kopie in de plugin, zodat die meereist in
  de plugin-zip. Opslaan maakt geen sjabloon aan.
- **Sjabloon aanmaken** maakt het sjabloon in de geopende contextmap. De
  plugin-zip importeren doet hetzelfde, en de plugin uitvoeren ook: zijn
  sjablonen worden aangemaakt voordat het script start. Een sjabloon dat al
  bestaat wordt nooit vervangen, dus je eigen wijzigingen blijven veilig.
- Onder **Sjablonen** in het manifest worden de sjablonen die een plugin bevat
  ter koppeling aangeboden nog voordat ze bestaan, zodat een plugin aan het
  sjabloon dat hij meebrengt kan worden gekoppeld. Voer hem één keer uit
  vanuit de werkruimte Plugins om dat sjabloon aan te maken.
- **Huidige versie overnemen** kopieert het sjabloon zoals het nu is terug
  naar de plugin.
- **Vervangen door versie uit plugin** verschijnt wanneer het sjabloon in de
  contextmap afwijkt van de kopie in de plugin. Na bevestiging overschrijft
  het het sjabloon met de versie uit de plugin. Dit is de enige manier waarop
  een sjabloon wordt vervangen: een import, een run of een script doet dat
  nooit. Records blijven behouden.

Een script kan hetzelfde met `formidable.template.install(file)`. Dat geeft
`true` terug wanneer het sjabloon is aangemaakt en `false` wanneer er al een
met die naam bestond. Alleen sjablonen die de plugin bevat kunnen worden
aangemaakt.

Wordt een plugin gestart vanuit de werkruimte Opslag, dan bevat de `ctx` van
het script `ctx.template` (de bestandsnaam van het geopende sjabloon) en
`ctx.datafile` (de bestandsnaam van het geopende record, als er een open is),
zodat het met "dit record" kan werken zonder te vragen welk.
