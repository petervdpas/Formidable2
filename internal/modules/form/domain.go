package form

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// templateLoader — what form needs from the template module. Satisfied
// by *template.Manager.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// formStore — what form needs from the storage module. Satisfied by
// *storage.Manager.
type formStore interface {
	EnsureFormDir(templateFilename string) error
	ListForms(templateFilename string) ([]string, error)
	ExtendedListForms(templateFilename string) ([]storage.FormSummary, error)
	LoadForm(templateFilename, datafile string) *storage.Form
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) storage.SaveResult
	DeleteForm(templateFilename, datafile string) error
}

// configReader — minimal config surface form needs. Satisfied by a
// thin adapter on *config.Manager (composition root supplies it).
//
// Kept narrow so config doesn't have to depend on form's value types,
// nor form on config's full Config struct.
type configReader interface {
	FormDefaults() ConfigDefaults
}

// ConfigDefaults bundles config values that affect form rendering.
// Snapshot read once per call — these change rarely. Author identity
// is NOT here: storage.Manager pulls it directly from its own
// AuthorProvider so every save path (form, api, csv import, integrity)
// gets the active profile stamped.
type ConfigDefaults struct {
	LoopStateCollapsed bool
}

// Manager owns the form-view orchestration.
type Manager struct {
	templates templateLoader
	storage   formStore
	config    configReader
	log       *slog.Logger
}

// NewManager constructs a form Manager. log may be nil.
func NewManager(t templateLoader, s formStore, c configReader, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{templates: t, storage: s, config: c, log: log}
}

// BuildView prepares the FormView for one (template, datafile) pair.
// Empty datafile, or a datafile that doesn't exist, yields an unsaved
// view with type-defaults injected so the UI has something to bind to.
func (m *Manager) BuildView(templateName, datafile string) (*FormView, error) {
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("form: load template %q: %w", templateName, err)
	}

	defaults := m.configDefaults()
	groups := ComputeLoopGroups(tpl.Fields, defaults.LoopStateCollapsed)

	if datafile == "" {
		return &FormView{
			Template:   tpl,
			Values:     defaultValues(tpl.Fields),
			Meta:       storage.FormMeta{Template: templateName},
			LoopGroups: groups,
			Datafile:   "",
			Saved:      false,
		}, nil
	}

	loaded := m.storage.LoadForm(templateName, datafile)
	if loaded == nil {
		// Missing or unreadable — fall back to an unsaved view, keep
		// the requested datafile so the UI can show "this is the
		// name you chose" without erroring.
		m.log.Warn("form: datafile not found; returning unsaved view",
			"template", templateName, "datafile", datafile)
		return &FormView{
			Template:   tpl,
			Values:     defaultValues(tpl.Fields),
			Meta:       storage.FormMeta{Template: templateName},
			LoopGroups: groups,
			Datafile:   datafile,
			Saved:      false,
		}, nil
	}

	// storage.LoadForm already injected defaults via Sanitize.
	return &FormView{
		Template:   tpl,
		Values:     loaded.Data,
		Meta:       loaded.Meta,
		LoopGroups: groups,
		Datafile:   datafile,
		Saved:      true,
	}, nil
}

// SaveValues persists the values + meta for one (template, datafile)
// pair, runs save-side normalizations, and returns the round-tripped
// FormView so caller state matches disk.
func (m *Manager) SaveValues(templateName string, payload SavePayload) (*FormView, error) {
	if payload.Datafile == "" {
		return nil, errors.New("form: empty datafile")
	}
	if _, err := m.templates.LoadTemplate(templateName); err != nil {
		return nil, fmt.Errorf("form: load template %q: %w", templateName, err)
	}

	values := payload.Values
	if values == nil {
		values = map[string]any{}
	}

	// Compose the bare-payload shape storage.Sanitize expects:
	//   {...values, _meta:{...}}
	// Storage owns id-generation, Created/Updated stamping (via its
	// AuthorProvider — wired to the active profile by the composition
	// root), tags collection, and shape-coercion. We pass meta through
	// only for fields storage doesn't otherwise know: template name +
	// flag state. Identity stamping is no longer the form module's job.
	meta := payload.Meta
	if meta.Template == "" {
		meta.Template = templateName
	}

	envelope := make(map[string]any, len(values)+1)
	maps.Copy(envelope, values)
	envelope["_meta"] = metaToMap(meta)

	if err := m.storage.EnsureFormDir(templateName); err != nil {
		return nil, fmt.Errorf("form: ensure dir: %w", err)
	}
	res := m.storage.SaveForm(context.Background(), templateName, payload.Datafile, envelope)
	if !res.Success {
		return nil, fmt.Errorf("form: save: %s", res.Error)
	}

	return m.BuildView(templateName, payload.Datafile)
}

// DeleteForm removes the form's meta.json. Missing is a no-op.
func (m *Manager) DeleteForm(templateName, datafile string) error {
	if datafile == "" {
		return errors.New("form: empty datafile")
	}
	return m.storage.DeleteForm(templateName, datafile)
}

// ListForms — passthrough to storage.ExtendedListForms so Vue gets the
// title-resolved + expression-bearing rows the sidebar needs.
func (m *Manager) ListForms(templateName string) ([]storage.FormSummary, error) {
	return m.storage.ExtendedListForms(templateName)
}

// EnsureFormDir — passthrough to storage; lets Vue create the per-
// template folder before listing on a fresh template.
func (m *Manager) EnsureFormDir(templateName string) error {
	return m.storage.EnsureFormDir(templateName)
}

// ─────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────

// configDefaults reads from the configReader once. nil-safe so the
// composition root can pass nil during early-boot tests.
func (m *Manager) configDefaults() ConfigDefaults {
	if m.config == nil {
		return ConfigDefaults{}
	}
	return m.config.FormDefaults()
}

// defaultValues fills a fresh values map from each field's default
// (or its type-default when no default is declared). Loop fields get
// an empty array so Vue can iterate over zero entries safely.
func defaultValues(fields []template.Field) map[string]any {
	out := map[string]any{}
	skip := map[string]bool{}
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			out[f.Key] = []any{}
			// Skip every field up to matching loopstop; nested loops
			// are handled recursively by the inner walk when Vue
			// renders entries.
			depth := 1
			for j := i + 1; j < len(fields); j++ {
				switch fields[j].Type {
				case "loopstart":
					depth++
				case "loopstop":
					depth--
					if depth == 0 {
						i = j
						goto next
					}
				}
				skip[fields[j].Key] = true
			}
		next:
			continue
		}
		if f.Type == "loopstop" {
			continue
		}
		if skip[f.Key] {
			continue
		}
		if f.Default != nil {
			out[f.Key] = f.Default
		} else {
			out[f.Key] = typeDefault(f.Type)
		}
	}
	return out
}

func typeDefault(t string) any {
	switch t {
	case "boolean":
		return false
	case "number":
		return 0
	case "range":
		return 50
	case "multioption", "list", "table":
		return []any{}
	default:
		return ""
	}
}

// metaToMap — JSON-shaped meta block, matching what storage.Sanitize
// reads out of the bare envelope's `_meta` key. Only emits Created /
// Updated when their At field is set — storage.SaveForm overrides
// these from prev + provider anyway, so passing zero values would just
// be noise.
func metaToMap(m storage.FormMeta) map[string]any {
	out := map[string]any{
		"id":       m.ID,
		"template": m.Template,
	}
	if len(m.Facets) > 0 {
		out["facets"] = facetsToMap(m.Facets)
	}
	if m.Created.At != "" {
		out["created"] = auditEntryToMap(m.Created)
	}
	if m.Updated.At != "" {
		out["updated"] = auditEntryToMap(m.Updated)
	}
	if len(m.Tags) > 0 {
		out["tags"] = m.Tags
	}
	return out
}

func facetsToMap(in map[string]storage.FacetState) map[string]any {
	out := make(map[string]any, len(in))
	for key, state := range in {
		entry := map[string]any{"set": state.Set}
		if state.Selected != "" {
			entry["selected"] = state.Selected
		}
		out[key] = entry
	}
	return out
}

func auditEntryToMap(a storage.AuditEntry) map[string]any {
	return map[string]any{
		"at":    a.At,
		"name":  a.Name,
		"email": a.Email,
	}
}
