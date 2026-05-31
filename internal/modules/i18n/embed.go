// Package i18n owns user-facing translations. Bundles live as JSON under
// locales/<locale>/<namespace>.json and are embedded into the binary at build
// time. Both the Vue frontend and any Go-side consumers read from the same
// in-memory bundles, so there is one source of truth for every user-facing
// string. Loading a locale merges its namespace files into a single flat
// key->value map.
package i18n

import "embed"

//go:embed locales/*/*.json
var embedded embed.FS
