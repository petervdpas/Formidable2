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
		application.NewService(a.Csv),
		application.NewService(a.Template),
		application.NewService(a.Storage),
		application.NewService(a.Form),
		application.NewService(a.I18n),
		application.NewService(a.Dialog),
		application.NewService(a.Render),
		application.NewService(a.Nav),
		application.NewService(a.Wiki),
		application.NewService(a.Dataprovider),
		application.NewService(a.Plugin),
	}
}
