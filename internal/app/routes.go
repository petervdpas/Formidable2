package app

import "net/http"

// RegisterRoutes mounts every opt-in module's HTTP handlers on the given mux.
// Stays a no-op until F-801 lands the loopback HTTP server. The function exists
// now so main.go and the future internal/server package can call it without
// further refactoring once handlers come online.
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	_ = mux
}
