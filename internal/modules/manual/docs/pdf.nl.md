# Portable Document Format (PDF) Exporteren

Formidable exporteert records naar PDF via **picoloom v2**, een
Chromium-gebaseerde renderer zonder LaTeX-afhankelijkheid. De
export-pijplijn is opt-in: er gebeurt niets PDF-gerelateerds tot
de gebruiker op **Activeren** klikt in het Informatie → PDF
Export-paneel.

## Setup

Het Informatie → PDF Export-paneel stuurt de levenscyclus van de
engine.

1. Klik **Zoeken** om bruikbare Chrome- / Chromium-binaries op te
   sporen. Formidable kijkt achtereenvolgens naar:
   - de omgevingsvariabele `FORMIDABLE_CHROME` (expliciete override)
   - platform-conventionele systeempaden (`/usr/bin/chromium`,
     `/Applications/…`, `Program Files/…`)
   - de meest recente binary in de managed-cache-map
2. Kies een kandidaat en klik **Activeren**. De gekozen binary
   wordt vastgelegd in `pdf-state.json`; volgende starts gebruiken
   hem zonder opnieuw te zoeken.
3. **Geen Chrome gevonden?** De managed-download-flow haalt een
   standalone Chromium-build naar de per-user-cache-map. Dat
   gebeurt alleen op expliciet verzoek. Formidable downloadt
   nooit stilzwijgend.

**Export-map**: het paneel onthoudt ook waar exports landen. Leeg
betekent de systeem-Documents-map; een niet-leeg pad moet een
bestaand absoluut pad zijn.

## Een PDF exporteren

Open in de **Opslag**-werkruimte, met een record geselecteerd,
**Export → PDF** (of gebruik de ribbon-shortcut). De
Export-dialoog vult zinnige defaults vooraf in op basis van het
template en de frontmatter van het record:

- **Thema**: picoloom's meegeleverde stijlen (`technical`,
  `academic`, `corporate`, `legal`, …). Verschijnt als de
  `theme:`-frontmatter-sleutel.
- **Voorpagina**: kiest een cover-template uit de bibliotheek op
  schijf. De lijst komt uit `<AppRoot>/pdf/covers/*.html`; kies
  *Geen* om de voorpagina over te slaan.
- **Oriëntatie**: staand of liggend.
- **Voettekst-positie**: geen, paginanummers, linksonder, enz.
- **Trefwoorden**: kommagescheiden termen die in het
  `/Keywords`-metadataveld van de PDF terechtkomen voor
  desktop-indexers.

Klikken op **Exporteren** schrijft een `<bestandsnaam>.pdf` naar
de gekozen uitvoermap en toont een toast met het pad.
Reeds bestaande bestanden worden atomair overschreven (tijdelijk
bestand + rename).

## Voorpagina's

Voorpagina-templates leven onder
`<AppRoot>/pdf/covers/<naam>.html` en worden beheerd vanuit de
Informatie → PDF Covers-pagina.

### Cover-HTML-bestanden

Elke cover is een zelfstandig HTML-document met een magische
header-regel die `name:` en `description:` declareert. Het
Library-paneel toont elke ontdekte cover; klik om hem in de editor
te laden. Opslaan valideert het document tegen het cover-schema;
structurele fouten blokkeren opslaan en verschijnen in de editor.

**Seed-covers** (Classic, Banner, Corporate) worden meegeleverd
met de binary en zijn gemarkeerd met een **SEED**-pil. Ze zijn
bewerkbaar; de destructieve actie heet **Resetten** in plaats van
Verwijderen omdat de volgende app-start de seed opnieuw schrijft
als het schijfbestand ontbreekt.

### Cover-afbeeldingen

Het **Afbeeldingen**-tabblad naast **Voorpagina's** beheert de
binaire assets (logo's, banners) waarnaar cover-HTML-bestanden
verwijzen. Afbeeldingen leven onder `<AppRoot>/pdf/covers/images/`.
De seed-bibliotheek levert `formidable.svg`; uploads van de
gebruiker komen daarnaast.

Een cover verwijst naar een afbeelding met de basisnaam:

```html
<img src="formidable.svg">
```

De picoloom-renderer resolvet kale basisnamen tegen de
afbeeldingenmap op convert-tijd, dus dezelfde cover werkt lokaal
én wanneer hij gedeeld wordt via de archief-flow hieronder.

### Een cover delen

De Library-rij heeft een **Export**-knop die de cover-HTML plus
elke afbeelding waarnaar hij verwijst (img src + CSS url())
bundelt tot één `<naam>.zip`. **Importeren** pakt zo'n archief
uit; bestaande covers vragen om bevestiging voor overschrijven
voordat het importeren commit.

## Frontmatter

Elke export voegt de frontmatter van het template samen met die
van het record, en laat het resultaat door de
picoloom-directive-processor lopen. De Informatie → Help →
Frontmatter Directives-pagina is de volledige referentie. Veel
gebruikte sleutels:

- `theme:` / `cover:` / `orientation:`. Gekozen via de
  Export-dialoog; kunnen ook hardcoded in het template staan.
- `keywords:`. String op topniveau die in het
  `/Keywords`-metadataveld van de PDF terechtkomt.
- `cover.logo:`. Afbeeldingspad dat het cover-template rendert.
  Kale basisnamen resolven tegen `<AppRoot>/pdf/covers/images/`.

## Probleemoplossing

Het Informatie → PDF Export-paneel bevat een **PDF
Doctor**-sub-paneel dat gestructureerde diagnostiek toont van de
laatste export. Elke kaart is één component van de pijplijn
(probe, activatie, render, convert, post-process) gemarkeerd als
succes of fout, met de exacte foutcode uit de export-taxonomie.
