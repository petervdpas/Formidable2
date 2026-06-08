package app

import "github.com/petervdpas/formidable2/internal/modules/config"

// configWriterAdapter bridges *config.Manager to nav.configWriter. nav
// doesn't need the *Config that UpdateUserConfig returns; it's discarded.
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

// CurrentSelection returns the active profile's selected (template, datafile)
// so nav can seed the origin onto the history stack. Empty on any read error.
func (a *configWriterAdapter) CurrentSelection() (string, string) {
	if a == nil || a.cfg == nil {
		return "", ""
	}
	cfg, err := a.cfg.LoadUserConfig()
	if err != nil || cfg == nil {
		return "", ""
	}
	return cfg.SelectedTemplate, cfg.SelectedDataFile
}
