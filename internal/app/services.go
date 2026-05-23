package app

import "github.com/wailsapp/wails/v3/pkg/application"

// WailsServices returns the list of services to register on the
// application options. Order is not significant.
func (a *App) WailsServices() []application.Service {
	return []application.Service{
		application.NewService(a.About),
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
		application.NewService(a.Git),
		application.NewService(a.Gigot),
		application.NewService(a.Credential),
		application.NewService(a.Monitor),
		application.NewService(a.Expression),
		application.NewService(a.History),
		application.NewService(a.Integrity),
		application.NewService(a.Logging),
		application.NewService(a.PDF),
		application.NewService(a.Manual),
		application.NewService(a.CodeFormatter),
		application.NewService(a.UpdateCheck),
	}
}
