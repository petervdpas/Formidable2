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

type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

type formStore interface {
	EnsureFormDir(templateFilename string) error
	ListForms(templateFilename string) ([]string, error)
	ExtendedListForms(templateFilename string) ([]storage.FormSummary, error)
	LoadForm(templateFilename, datafile string) *storage.Form
	SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) storage.SaveResult
	DeleteForm(templateFilename, datafile string) error
	SortFieldValue(templateFilename, datafile, fieldKey, column, direction string) (any, error)
	DedupFieldValue(templateFilename, datafile, fieldKey, column string) (any, error)
}

type configReader interface {
	FormDefaults() ConfigDefaults
}

// ReferenceEdgeSyncer reconciles a host record's api-field references into the
// relation edge graph. SyncReferenceEdges is the authoritative save reconcile (add
// new, remove orphaned); AddReferenceEdges is the add-only load heal (create
// missing edges, never remove). Optional: nil disables edge syncing, so the form
// Manager stays decoupled from relations.
type ReferenceEdgeSyncer interface {
	SyncReferenceEdges(hostTemplate, hostGuid string, fields []template.Field, data map[string]any) error
	AddReferenceEdges(hostTemplate, hostGuid string, fields []template.Field, data map[string]any) error
}

// RelationReader returns the existing relation edges from a host template to a
// target collection, as {from-guid, to-guid} pairs. Injected so the form Manager
// can back-fill an api field from already-synced edges without importing the
// relation module. Optional: nil makes SyncRelationsToField a no-op.
type RelationReader interface {
	RelationEdges(hostTemplate, targetCollection string) ([]EdgePair, error)
}

// ConfigDefaults bundles config values that affect form rendering.
// Author identity is NOT here: storage.Manager pulls it directly from its
// own AuthorProvider so every save path stamps the active profile.
type ConfigDefaults struct {
	LoopStateCollapsed bool
	// RelationSyncEnabled gates the destructive "Synchronize from relations" pass
	// (SyncRelationsToField/SyncRelationsForTemplate), which overwrites api-field
	// values from the relation graph. Off by default: the action is a no-op guard
	// unless the user explicitly enables it in config.
	RelationSyncEnabled bool
}

// Manager owns form-view orchestration.
type Manager struct {
	templates     templateLoader
	storage       formStore
	config        configReader
	refEdges      ReferenceEdgeSyncer
	resolveRecord recordResolver
	relations     RelationReader
	log           *slog.Logger
}

// NewManager constructs a form Manager; log may be nil.
func NewManager(t templateLoader, s formStore, c configReader, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{templates: t, storage: s, config: c, log: log}
}

// SetReferenceEdgeSyncer wires the optional api-field edge reconciler. Called
// once at composition; safe to leave unset (edge syncing is then a no-op).
func (m *Manager) SetReferenceEdgeSyncer(s ReferenceEdgeSyncer) { m.refEdges = s }

// SetRelationReader wires the optional relation-edge reader used to back-fill an
// api field from existing edges (SyncRelationsToField). Safe to leave unset.
func (m *Manager) SetRelationReader(r RelationReader) { m.relations = r }

// BuildView prepares the FormView for one (template, datafile) pair.
// A missing or empty datafile yields an unsaved view with type-defaults
// injected so the UI has something to bind to.
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

	// Add-only edge heal: create relation edges missing for the references this
	// record carries (e.g. a target that only just got a stable guid). Never drains
	// here, so a partially-resolved view can't strip live edges; the save path owns
	// removal. Best-effort: the view is returned regardless.
	if m.refEdges != nil && loaded.Meta.ID != "" {
		if err := m.refEdges.AddReferenceEdges(templateName, loaded.Meta.ID, tpl.Fields, loaded.Data); err != nil {
			m.log.Warn("form: reference edge add-heal failed",
				"template", templateName, "id", loaded.Meta.ID, "err", err)
		}
	}

	return &FormView{
		Template:   tpl,
		Values:     loaded.Data,
		Meta:       loaded.Meta,
		LoopGroups: groups,
		Datafile:   datafile,
		Saved:      true,
	}, nil
}

// SaveValues persists values + meta for one (template, datafile) pair and
// returns the round-tripped FormView so caller state matches disk.
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

	// Storage owns id-generation, Created/Updated stamping (via its
	// AuthorProvider), tags collection, and shape-coercion. Meta passes
	// through only for what storage can't otherwise know (template name).
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

	view, err := m.BuildView(templateName, payload.Datafile)
	if err != nil {
		return nil, err
	}

	// Reconcile api-field references into the relation edge graph. Best-effort:
	// the record is already persisted, and reconcile heals any miss on next save.
	if m.refEdges != nil && view.Meta.ID != "" && view.Template != nil {
		if err := m.refEdges.SyncReferenceEdges(templateName, view.Meta.ID, view.Template.Fields, view.Values); err != nil {
			m.log.Warn("form: reference edge sync failed",
				"template", templateName, "id", view.Meta.ID, "err", err)
		}
	}

	return view, nil
}

// CopyForm duplicates an existing record into newDatafile. Every field value,
// tag and facet is carried over verbatim; only the identity is reset. The guid
// data field and meta.id are cleared so the normal save path mints a fresh GUID
// (storage.CanonicalGuid) and stamps fresh Created/Updated (newDatafile means
// prev == nil in storage.SaveForm). Routing through SaveValues also reconciles
// the copy's api references into the relation edge graph under its new id.
func (m *Manager) CopyForm(templateName, sourceDatafile, newDatafile string) (*FormView, error) {
	if sourceDatafile == "" {
		return nil, errors.New("form: empty source datafile")
	}
	if newDatafile == "" {
		return nil, errors.New("form: empty datafile")
	}
	if sourceDatafile == newDatafile {
		return nil, errors.New("form: copy target must differ from source")
	}
	tpl, err := m.templates.LoadTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("form: load template %q: %w", templateName, err)
	}
	src := m.storage.LoadForm(templateName, sourceDatafile)
	if src == nil {
		return nil, fmt.Errorf("form: source %q not found", sourceDatafile)
	}
	if m.storage.LoadForm(templateName, newDatafile) != nil {
		return nil, fmt.Errorf("form: %q already exists", newDatafile)
	}

	// Clone the data and strip the guid field, the deterministic half of the
	// reset: with the data field empty CanonicalGuid mints a fresh id rather than
	// preserving the source's. (The guidCollides guard is best-effort backup, not
	// the rule.)
	values := maps.Clone(src.Data)
	for _, f := range tpl.Fields {
		if f.Type == "guid" {
			delete(values, f.Key)
			break
		}
	}

	// Keep tags + facets; drop id and audit so storage stamps the copy fresh.
	meta := src.Meta
	meta.ID = ""
	meta.Created = storage.AuditEntry{}
	meta.Updated = storage.AuditEntry{}

	return m.SaveValues(templateName, SavePayload{
		Datafile: newDatafile,
		Values:   values,
		Meta:     meta,
	})
}

// SortFieldValue fetches a list/table field from the saved record (pointer:
// template + datafile + fieldKey), sorts it, and returns the sorted value. It
// does not persist: the frontend applies the value to its draft and the normal
// save path writes it. For tables, column is the column key (empty = first
// column); direction is "asc" (default) or "desc".
func (m *Manager) SortFieldValue(templateName, datafile, fieldKey, column, direction string) (any, error) {
	return m.storage.SortFieldValue(templateName, datafile, fieldKey, column, direction)
}

// DedupFieldValue fetches a list/table field from the saved record, removes
// duplicates, and returns the result without persisting. For tables, column is
// the key whose value identifies a duplicate row (empty = first column).
func (m *Manager) DedupFieldValue(templateName, datafile, fieldKey, column string) (any, error) {
	return m.storage.DedupFieldValue(templateName, datafile, fieldKey, column)
}

// DeleteForm removes the form's meta.json; missing is a no-op.
func (m *Manager) DeleteForm(templateName, datafile string) error {
	if datafile == "" {
		return errors.New("form: empty datafile")
	}
	return m.storage.DeleteForm(templateName, datafile)
}

// ListForms returns the title-resolved, expression-bearing summary rows.
func (m *Manager) ListForms(templateName string) ([]storage.FormSummary, error) {
	return m.storage.ExtendedListForms(templateName)
}

// EnsureFormDir creates the per-template folder before listing.
func (m *Manager) EnsureFormDir(templateName string) error {
	return m.storage.EnsureFormDir(templateName)
}

// ─────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────

// configDefaults is nil-safe so early-boot tests can pass a nil config.
func (m *Manager) configDefaults() ConfigDefaults {
	if m.config == nil {
		return ConfigDefaults{}
	}
	return m.config.FormDefaults()
}

// defaultValues fills a fresh values map from each field's default or
// type-default. Loop fields get an empty array.
func defaultValues(fields []template.Field) map[string]any {
	out := map[string]any{}
	skip := map[string]bool{}
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f.Type == "loopstart" {
			out[f.Key] = []any{}
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
	case "number", "sequence":
		return 0
	case "range":
		return 50
	case "multioption", "list", "table":
		return []any{}
	default:
		return ""
	}
}

// metaToMap builds the `_meta` block storage.Sanitize reads. Created /
// Updated are emitted only when set, since SaveForm overrides them from
// prev + provider anyway.
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
