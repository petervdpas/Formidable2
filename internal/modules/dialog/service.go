// Package dialog is a thin Wails-bound facade over the native
// open-file / save-file pickers. The frontend triggers a dialog by
// calling Dialog.Service.ChooseFile / ChooseSaveFile; the OS picker
// blocks the calling Wails goroutine until the user commits or
// cancels. An empty returned path indicates user cancellation -
// callers should treat that as a no-op, not an error.
//
// There is no Manager type here. The module is just an adapter to the
// Wails dialog API; it has no state of its own and is therefore not
// unit-testable without a running Wails app. Behaviour is verified by
// hand at the Vue layer.
package dialog

import "github.com/wailsapp/wails/v3/pkg/application"

// Service is the Wails-bound dialog surface.
type Service struct{}

// NewService constructs a Service. No dependencies - the running Wails
// application instance is reached via application.Get() at call time.
func NewService() *Service { return &Service{} }

// ChooseFile opens a native open-file picker. When filters are
// supplied, the picker restricts visible files to those matching
// (each filter becomes one entry in the picker's filter dropdown).
// Returns the absolute selected path, or "" if the user cancelled.
func (s *Service) ChooseFile(filters []FileFilter) (string, error) {
	d := application.Get().Dialog.OpenFile()
	for _, f := range filters {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	return d.PromptForSingleSelection()
}

// ChooseDirectory opens a native folder picker. Used by export-to-
// folder flows that don't need a filename. Returns "" on cancel.
func (s *Service) ChooseDirectory() (string, error) {
	d := application.Get().Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true)
	return d.PromptForSingleSelection()
}

// ChooseSaveFile opens a native save-file picker pre-populated with
// defaultName. filters narrow allowed extensions. Returns "" on cancel.
func (s *Service) ChooseSaveFile(defaultName string, filters []FileFilter) (string, error) {
	d := application.Get().Dialog.SaveFile()
	if defaultName != "" {
		d.SetFilename(defaultName)
	}
	for _, f := range filters {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	return d.PromptForSingleSelection()
}
