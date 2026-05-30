package nav

import (
	"fmt"
	"log/slog"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateLoader loads a template by filename to confirm it exists.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// formStore loads a form by (template, datafile) to confirm it exists.
// LoadForm returns nil for missing/unreadable.
type formStore interface {
	LoadForm(templateFilename, datafile string) *storage.Form
}

// configWriter writes selected_template, selected_data_file, and
// context_ribbon atomically.
type configWriter interface {
	UpdateUserConfig(partial map[string]any) error
}

// EventEmitter publishes nav change events. Nil silences emit.
type EventEmitter interface {
	Emit(name string, data any)
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
	emitter   EventEmitter
	history   HistoryPusher
	log       *slog.Logger
}

// NewManager constructs a nav Manager. log, emitter, and history may
// be nil.
func NewManager(t templateLoader, s formStore, c configWriter, e EventEmitter, h HistoryPusher, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{templates: t, storage: s, config: c, emitter: e, history: h, log: log}
}

// NavigateToFormidable parses the URL, validates that the (template,
// datafile) pair exists, updates config so the Storage workspace's
// reactive watchers pick up the new selection, and emits a
// `nav:changed` event so the frontend can flip the active workspace.
//
// Returns a Result rather than a bare error so the frontend gets one
// shape - Success=false carries Error for direct toast display, Target
// is filled even on validation failure for diagnostics.
func (m *Manager) NavigateToFormidable(href string) (*Result, error) {
	target := ParseFormidableHref(href)
	if target == nil {
		return &Result{Success: false, Error: fmt.Sprintf("invalid formidable url: %q", href)}, nil
	}

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
		m.history.Push(MakeHref(target))
	}

	return &Result{Success: true, Target: target}, nil
}

// ResolveFormidable parses + validates without mutating state.
// Used by the future internal HTTP server, which routes via URL
// rewriting rather than config-driven workspace switching.
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
