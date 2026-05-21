# Instellingen

Formidable groepeert zijn configuratie in **profielen**. Elk profiel
is een onafhankelijke werkruimte met eigen templates, opslag, plugins
en voorkeuren. Wissel van profiel via het gebruikersmenu rechtsonder
— de app herlaadt naar de context van het gekozen profiel.

## Waar instellingen staan

Profielen leven in de per-user applicatie-datamap. Elk profiel is een
map met:

| Pad                        | Doel                                                         |
| -------------------------- | ------------------------------------------------------------ |
| `config.json`              | De volledige instellingen-record voor dit profiel.           |
| `templates/`               | YAML-templatedefinities, ontdekt bij start.                  |
| `storage/<template>/`      | Per-template recordbestanden (`.md`, `.meta.json`).          |
| `plugins/`                 | Plugin-mappen, zie de Plugins handleiding-pagina.            |
| `pdf/covers/`              | Door gebruiker geschreven PDF-voorpagina's.                  |

De actieve profielnaam staat in de titelbalk; het pad is zichtbaar
in het Informatie → Over-paneel.

## Veelgebruikte instellingen

Een paar velden waar je het meest aan zit:

- **Thema** — licht / donker / systeem.
- **Taal** — bepaalt de UI-locale én de resolutie van plugin-i18n.
- **Plugins inschakelen** — globale kill-switch. Uit verbergt de
  Plugins-werkruimte en slaat plugin-detectie over.
- **Logging aan** — schrijft een roterende log naar schijf; het
  Informatie → Logging-paneel toont 'm live.
- **Ingeschakelde templates** — bepaalt welke templates in de
  Opslag-werkruimte verschijnen; lege lijst betekent "alle".
- **Plakknoppen tonen** — toont een plak-uit-klembord-icoon naast
  tekst- en textarea-velden.
- **Auteurnaam** + **Auteur e-mail** — standaardidentiteit op nieuwe
  records en op git-commits gemaakt via de Sync-werkruimte.
- **Context-modus / ribbon / map** — kiest de actieve
  werkruimte-context bij start.

De volledige lijst staat als formulier in de Profielen-werkruimte.

## Interne server

Een kleine HTTP-server kan per profiel worden ingeschakeld om de
wiki + REST API op een lokale poort aan te bieden. Plugins die
`formidable.api.fetch` nodig hebben vereisen dit. Zie het
Informatie → Interne server-paneel voor status en knoppen.

## Git en Gigot

Elk profiel heeft eigen remote-backend-instellingen:

- **Git** — wijst naar een remote-repository via HTTPS of SSH;
  credentials leven in de keychain.
- **Gigot** — Formidable's lichtgewicht ledger-gebaseerde sync,
  geadresseerd via een base-URL + per-profiel-abonnementstoken.

Deze staan los van elkaar — een profiel kiest één of geen.

## Opslaan en resetten

Instellingen slaan atomair op (temp-bestand + fsync + rename), zodat
een crash tijdens opslaan `config.json` nooit kan corrupteren. Om één
veld te resetten leeg je 't in de Profielen-werkruimte; voor een
volledige reset verwijder je de profielmap en laat je de app 'm bij
de volgende start opnieuw aanmaken.
