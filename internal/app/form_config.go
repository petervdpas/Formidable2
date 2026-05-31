package app

import (
	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/form"
)

// configAdapter bridges *config.Manager to form.configReader, keeping the
// dependency arrow strictly app to modules.
type configAdapter struct {
	cfg *config.Manager
}

// FormDefaults snapshots the config values form.Manager needs. Best-effort:
// errors yield zero defaults so a missing config never blocks the pipeline.
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
	}
}
