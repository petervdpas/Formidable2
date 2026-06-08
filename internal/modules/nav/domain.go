package nav

import (
	"fmt"
	"log/slog"

	"github.com/petervdpas/formidable2/internal/event"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateLoader loads a template by filename to confirm it exists.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// formStore loads a form by (template, datafile) to confirm it exists.
// LoadForm returns nil for missing or unreadable forms.
type formStore interface {
	LoadForm(templateFilename, datafile string) *storage.Form
}

// configWriter writes selected_template, selected_data_file, and
// context_ribbon atomically, and reads the current selection so a
// navigation can seed its origin onto the history stack.
type configWriter interface {
	UpdateUserConfig(partial map[string]any) error
	CurrentSelection() (template, datafile string)
}

// HistoryPusher pushes the canonical href onto the back/forward stack
// after a successful navigation. Nil silences pushes.
type HistoryPusher interface {
	Push(href string)
}

// Manager owns formidable:// link resolution.
type Manager struct {
	templates templateLoader
	storage   formStore
	config    configWriter
	emitter   event.Emitter
	history   HistoryPusher
	log       *slog.Logger
}

// NewManager constructs a nav Manager. log, emitter, and history may be nil.
func NewManager(t templateLoader, s formStore, c configWriter, e event.Emitter, h HistoryPusher, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{templates: t, storage: s, config: c, emitter: e, history: h, log: log}
}

// NavigateToFormidable parses the URL, validates that the (template,
// datafile) pair exists, updates config so the Storage workspace's
// watchers pick up the new selection, and emits nav:changed so the
// frontend can flip the active workspace.
func (m *Manager) NavigateToFormidable(href string) (*Result, error) {
	target := ParseFormidableHref(href)
	if target == nil {
		return &Result{Success: false, Error: fmt.Sprintf("invalid formidable url: %q", href)}, nil
	}

	// Capture where we're leaving from BEFORE the switch, so Back can return to
	// the record the user was on (sidebar selections never reach the stack
	// otherwise). Pushed only after the target validates; deduped on push.
	originHref := m.originHref(target)

	tpl, err := m.templates.LoadTemplate(target.Template)
	if err != nil {
		return &Result{
			Success: false,
			Target:  target,
			Error:   fmt.Sprintf("load template %q: %v", target.Template, err),
		}, nil
	}
	if tpl == nil {
		return &Result{
			Success: false,
			Target:  target,
			Error:   fmt.Sprintf("template not found: %q", target.Template),
		}, nil
	}

	if form := m.storage.LoadForm(target.Template, target.Datafile); form == nil {
		return &Result{
			Success: false,
			Target:  target,
			Error:   fmt.Sprintf("form not found: %q in %q", target.Datafile, target.Template),
		}, nil
	}

	if err := m.config.UpdateUserConfig(map[string]any{
		"selected_template":  target.Template,
		"selected_data_file": target.Datafile,
		"context_ribbon":     "storage",
	}); err != nil {
		return &Result{
			Success: false,
			Target:  target,
			Error:   fmt.Sprintf("update config: %v", err),
		}, nil
	}

	if m.emitter != nil {
		m.emitter.Emit(EventChanged, target)
	}
	if m.history != nil {
		if originHref != "" {
			m.history.Push(originHref)
		}
		m.history.Push(MakeHref(target))
	}

	return &Result{Success: true, Target: target}, nil
}

// originHref builds the formidable:// href for the current selection (the record
// being left), or "" when there's no selection, config can't be read, or it
// equals the target (no self-loop on the stack).
func (m *Manager) originHref(target *Target) string {
	if m.config == nil {
		return ""
	}
	tpl, df := m.config.CurrentSelection()
	if tpl == "" || df == "" {
		return ""
	}
	origin := MakeHref(&Target{Template: tpl, Datafile: df})
	if origin == MakeHref(target) {
		return ""
	}
	return origin
}

// ResolveFormidable parses and validates without mutating state. Used
// by the future internal HTTP server, which routes via URL rewriting
// rather than config-driven workspace switching.
func (m *Manager) ResolveFormidable(href string) (*Result, error) {
	target := ParseFormidableHref(href)
	if target == nil {
		return &Result{Success: false, Error: fmt.Sprintf("invalid formidable url: %q", href)}, nil
	}
	tpl, err := m.templates.LoadTemplate(target.Template)
	if err != nil {
		return &Result{Success: false, Target: target, Error: err.Error()}, nil
	}
	if tpl == nil {
		return &Result{Success: false, Target: target, Error: "template not found"}, nil
	}
	if form := m.storage.LoadForm(target.Template, target.Datafile); form == nil {
		return &Result{Success: false, Target: target, Error: "form not found"}, nil
	}
	return &Result{Success: true, Target: target}, nil
}
