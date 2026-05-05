// Package i18n owns user-facing translations. Bundles live as JSON
// under locales/ and are embedded into the binary at build time so the
// running app has zero filesystem dependency on the source tree.
//
// Both the Wails-bound Vue frontend and any Go-side consumers (future
// error message translation, journal labels, etc.) read from the same
// in-memory bundles, so there is exactly one source of truth for every
// user-facing string.
package i18n

import "embed"

// Locale tree:
//   locales/<locale>/<namespace>.json
//
// Each <locale>/ directory holds one JSON file per namespace
// (shell, settings, menus, status, modals, fields, errors). Loading a
// locale at runtime merges all of its namespace files into a single
// flat key→value map served to the frontend.
//
//go:embed locales/*/*.json
var embedded embed.FS
