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
