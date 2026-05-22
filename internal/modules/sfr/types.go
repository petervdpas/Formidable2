// Package sfr is the Single-File Repository - a thin layer that
// stores blobs at <directory>/<basename><extension> and normalizes the
// caller-provided base filename. Wails-only by design: callers
// supply the directory, so this never gets exposed over the local
// HTTP server.
package sfr

// Options overrides the manager defaults for a single call.
// JSON is a pointer so callers can leave it unset; nil means "use
// the manager default" rather than "false".
type Options struct {
	Extension string `json:"extension,omitempty"`
	JSON      *bool  `json:"json,omitempty"`
}

// SaveResult mirrors the JS shape so frontend handlers can keep using
// `result.success`, `result.path`, `result.error`.
type SaveResult struct {
	Success bool   `json:"success"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}
