# Gebruikersinstellingen

Elk profiel draagt z'n eigen instellingen-record, opgeslagen als
`config.json` in de profielmap. De Profielen-werkruimte rendert die
record als formulier; de Gebruikersprofielen-pagina behandelt de
per-profiel-mapstructuur en hoe je tussen profielen wisselt.

## Veelgebruikte velden

Een paar instellingen waar je het meest aan zit:

- **Thema** - licht / donker / systeem.
- **Taal** - bepaalt de UI-locale én de resolutie van plugin-i18n.
- **Plugins inschakelen** - globale kill-switch. Uit verbergt de
  Plugins-werkruimte en slaat plugin-detectie over.
- **Logging aan** - schrijft een roterende log naar schijf; het
  Informatie → Logging-paneel toont 'm live.
- **Ingeschakelde templates** - bepaalt welke templates in de
  Opslag-werkruimte verschijnen; lege lijst betekent "alle".
- **Plakknoppen tonen** - toont een plak-uit-klembord-icoon naast
  tekst- en textarea-velden.
- **Auteurnaam** + **Auteur e-mail** - standaardidentiteit op nieuwe
  records en op git-commits gemaakt via de Sync-werkruimte.
- **Context-modus / ribbon / map** - kiest de actieve
  werkruimte-context bij start.

## Interne server

Een kleine HTTP-server kan per profiel worden ingeschakeld om de
wiki + REST API op een lokale poort aan te bieden. Plugins die
`formidable.api.fetch` nodig hebben vereisen dit. Zie het
Informatie → Interne server-paneel voor status en knoppen.

## Git en Gigot

Elk profiel heeft eigen remote-backend-instellingen:

- **Git** - wijst naar een remote-repository via HTTPS of SSH;
  credentials leven in de keychain.
- **Gigot** - Formidable's lichtgewicht ledger-gebaseerde sync,
  geadresseerd via een base-URL + per-profiel-abonnementstoken.

Deze staan los van elkaar - een profiel kiest één of geen.

## Opslaan en resetten

Instellingen slaan atomair op (temp-bestand + fsync + rename), zodat
een crash tijdens opslaan `config.json` nooit kan corrupteren. Om één
veld te resetten leeg je 't in de Profielen-werkruimte; voor een
volledige reset zie Gebruikersprofielen voor hoe je de profielmap
opnieuw laat aanmaken.
