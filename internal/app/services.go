package app

import "github.com/wailsapp/wails/v3/pkg/application"

// WailsServices returns the list of services to register on the
// application options. Order is not significant.
func (a *App) WailsServices() []application.Service {
	return []application.Service{
		application.NewService(a.System),
		application.NewService(a.Config),
		application.NewService(a.Sfr),
		application.NewService(a.Journal),
	}
}
