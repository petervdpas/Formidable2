package app

import "github.com/petervdpas/formidable2/internal/modules/config"

// configWriter bridges *config.Manager → nav.configWriter without
// dragging nav's types into config. UpdateUserConfig returns a *Config
// that nav doesn't need; the adapter discards it.
type configWriterAdapter struct {
	cfg *config.Manager
}

func (a *configWriterAdapter) UpdateUserConfig(partial map[string]any) error {
	if a == nil || a.cfg == nil {
		return nil
	}
	_, err := a.cfg.UpdateUserConfig(partial)
	return err
}
