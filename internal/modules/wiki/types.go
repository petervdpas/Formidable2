// Package wiki owns Formidable's runtime-controllable HTTP server.
//
// The original Formidable's `controls/internalServer.js` exposed the
// templates + storage tree as a small wiki + JSON API for plugins,
// addons, and bookmarkable browsing. This module is its Go port:
//
//   - Manager owns the listener lifecycle (Start / Stop / Status).
//   - Future slices wire route groups (HTML pages, /storage/*, API
//     collections) onto a single mux that the manager hot-swaps.
//   - The composition root injects *dataprovider.Manager + *render.Manager
//     so route handlers stay decoupled from disk.
//
// Lives behind a feature flag in user config (EnableInternalServer +
// InternalServerPort). The about workspace turns it on and off at
// runtime; "later monitoring" hooks (request log, last-N requests)
// are deferred but the lifecycle gives them a place to land.
package wiki

import "time"

// ServerStatus is the live state the about workspace renders.
// Returned by Manager.Status; safe to call when the server is idle.
// StartedAt is the zero time when Running is false — JSON consumers
// should gate on Running, not on the timestamp value.
type ServerStatus struct {
	Running   bool      `json:"running"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
}
