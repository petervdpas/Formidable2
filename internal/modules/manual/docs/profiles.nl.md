# Gebruikersprofielen

Een **profiel** is een geïsoleerde werkruimte: eigen sjablonen,
opslag, plugins, PDF-voorpagina's, git/gigot-remotes en `config.json`.
Wisselen van profiel herlaadt de app naar de context van het gekozen
profiel - er gaat niets tussen profielen heen en weer.

## Waar profielen staan

Profielen leven in de per-user applicatie-datamap. Elk profiel is een
map met:

| Pad                        | Doel                                                         |
| -------------------------- | ------------------------------------------------------------ |
| `config.json`              | De volledige instellingen-record voor dit profiel.           |
| `templates/`               | YAML-sjabloondefinities, ontdekt bij start.                  |
| `storage/<template>/`      | Per-sjabloon recordbestanden (`.md`, `.meta.json`).          |
| `plugins/`                 | Plugin-mappen, zie de Aanpassen via Plugins-pagina.          |
| `pdf/covers/`              | Door gebruiker geschreven PDF-voorpagina's.                  |

De actieve profielnaam staat in de titelbalk; het pad is zichtbaar
via het Informatie → Over-paneel.

## Wisselen van profiel

Het gebruikersmenu rechtsonder toont elk gedetecteerd profiel. Een
keuze triggert een context-reload: composables gooien hun caches
leeg, opslag en sjablonen lezen opnieuw uit de nieuwe map, en de
werkruimte komt schoon terug onder de nieuwe identiteit.

## Profielen aanmaken en verwijderen

De Profielen-werkruimte (zijbalk-icoon) is waar je nieuwe profielen
aanmaakt en bestaande aanpast. Elk profiel krijgt het volledige
instellingenformulier; velden die je niet aanraakt vallen terug op de
applicatie-defaults.

Eén profiel resetten doe je door de relevante velden leeg te maken.
Een profiel volledig wegvegen: sluit de app, verwijder de map - de
volgende start maakt 'm leeg opnieuw aan.

## Waarom isolatie ertoe doet

Per-profiel sjablonen en opslag betekenen dat je een "werk"-profiel
en een "privé"-profiel kunt hebben die nul state delen. Per-profiel
plugins betekenen dat een plugin die voor één profiel is
geïnstalleerd niet zichtbaar is voor de andere. Per-profiel remotes
betekenen dat een profiel één git-repo of één gigot-abonnement kan
targeten zonder credentials over contexten heen te lekken.
