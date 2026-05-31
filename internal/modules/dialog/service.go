// Package dialog is a Wails-bound facade over the native open-file /
// save-file pickers. The OS picker blocks the calling Wails goroutine until
// the user commits or cancels; an empty returned path means cancellation, to
// be treated as a no-op rather than an error. The module is a stateless
// adapter with no Manager, verified by hand at the Vue layer.
package dialog

import "github.com/wailsapp/wails/v3/pkg/application"

// Service is the Wails-bound dialog surface.
type Service struct{}

// NewService constructs a Service; the Wails application instance is reached
// via application.Get() at call time.
func NewService() *Service { return &Service{} }

// ChooseFile opens a native open-file picker, optionally restricting visible
// files to the supplied filters. Returns the absolute selected path, or "" if
// the user cancelled.
func (s *Service) ChooseFile(filters []FileFilter) (string, error) {
	d := application.Get().Dialog.OpenFile()
	for _, f := range filters {
		d.AddFilter(f.DisplayName, f.Pattern)
	}
	return d.PromptForSingleSelection()
}

// ChooseDirectory opens a native folder picker. Returns "" on cancel.
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
