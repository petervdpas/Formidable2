package app

import (
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/config"
)

// defaultInternalServerPort mirrors the fallback used by wikiSvc when
// the user's config has no explicit InternalServerPort. Kept in sync
// here so the API origin allowlist is built against the same port the
// wiki listener will actually bind on.
const defaultInternalServerPort = 8383

// buildAPIOriginAllowlist returns the scheme+host[+port] strings the
// auth.RequireOrigin middleware accepts on writes. Built at app boot
// from the configured InternalServerPort: a port change mid-session
// requires the same wiki-server restart that today's port-change flow
// already needs, so a static allowlist is acceptable here.
//
// Both ports (default + configured) are included when they differ so a
// fresh-install profile and a customised profile both work without a
// rebuild.
func buildAPIOriginAllowlist(cfgM *config.Manager) []string {
	port := defaultInternalServerPort
	if cfgM != nil {
		if c, err := cfgM.LoadUserConfig(); err == nil && c != nil && c.InternalServerPort > 0 {
			port = c.InternalServerPort
		}
	}
	out := []string{fmt.Sprintf("http://127.0.0.1:%d", port)}
	if port != defaultInternalServerPort {
		out = append(out, fmt.Sprintf("http://127.0.0.1:%d", defaultInternalServerPort))
	}
	return out
}
