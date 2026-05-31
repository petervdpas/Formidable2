package app

import (
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/config"
)

// defaultInternalServerPort mirrors wikiSvc's fallback when config has no
// explicit InternalServerPort, so the allowlist matches the listener's port.
const defaultInternalServerPort = 8383

// buildAPIOriginAllowlist returns the origins auth.RequireOrigin accepts on
// writes. Built at boot from InternalServerPort: a port change mid-session
// needs the same wiki-server restart the port-change flow already requires,
// so a static allowlist is acceptable. Both default + configured ports are
// included when they differ so fresh-install and customised profiles work.
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
