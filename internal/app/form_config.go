package app

import (
	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/form"
)

// configAdapter bridges *config.Manager → form.configReader without
// dragging form's types into the config module. Keeps the dependency
// arrow strictly app → modules.
type configAdapter struct {
	cfg *config.Manager
}

// FormDefaults snapshots the config values form.Manager needs.
// Errors are swallowed and reported as zero defaults — this is a
// best-effort read called on every form save/build, and a missing
// or unreadable config should never block the form pipeline.
func (a *configAdapter) FormDefaults() form.ConfigDefaults {
	if a == nil || a.cfg == nil {
		return form.ConfigDefaults{}
	}
	c, err := a.cfg.LoadUserConfig()
	if err != nil || c == nil {
		return form.ConfigDefaults{}
	}
	return form.ConfigDefaults{
		LoopStateCollapsed: c.LoopStateCollapsed,
		AuthorName:         c.AuthorName,
		AuthorEmail:        c.AuthorEmail,
	}
}
