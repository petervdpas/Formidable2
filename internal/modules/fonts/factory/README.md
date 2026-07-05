# Factory fonts

Font files placed in this directory are embedded into the Formidable binary and
shipped as **factory fonts**. On boot they are scaffolded to `<AppRoot>/fonts/`,
shown in the Information -> Fonts panel with a SEED badge, and restored by the
"Restore default fonts" action if the user deletes them.

Only add **open-licensed** fonts here (SIL Open Font License or similar), since
they ship inside the binary. Accepted extensions: `.woff2`, `.woff`, `.ttf`,
`.otf`. The family name shown in the Font picker is the filename without its
extension (e.g. `Inter.woff2` -> "Inter"), so name the file after the family.

This folder ships empty by design: Formidable is brand-neutral and does not
bundle a specific typeface. Drop the fonts you want as defaults here.
