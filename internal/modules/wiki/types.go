// Package wiki owns Formidable's runtime-controllable HTTP server: it serves the templates +
// storage tree as an HTML wiki. Gated by config EnableInternalServer + InternalServerPort.
package wiki

import "time"

// ServerStatus is the live state Manager.Status returns; consumers gate on Running, not StartedAt.
type ServerStatus struct {
	Running   bool      `json:"running"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
}
