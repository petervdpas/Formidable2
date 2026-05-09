package expression

import (
	"errors"
	"fmt"
)

// TemplateProvider abstracts the template module so this package can
// stay layer-clean (no template import → no risk of an import cycle
// when template eventually wants to consume an Expression hook).
// Returned `Template` is opaque to the engine; we read only the two
// fields the sidebar needs.
type TemplateProvider interface {
	// LookupSidebar returns (sidebarExpression, fields, error). Fields
	// is the full field list so callers can resolve `expression_item`
	// flags without leaking the template type into the engine.
	LookupSidebar(name string) (expr string, expressionFields []string, err error)
}

// StorageProvider abstracts the storage module's list-with-context
// surface. Each Record carries the harvested ExpressionItems map for
// one record; that map IS the per-row evaluation context, so the
// engine never reads template Field metadata at evaluate time.
type StorageProvider interface {
	ListForExpression(templateName string) ([]Record, error)
}

// Record is the slim shape ExpressionProvider needs — Filename to
// pair the result back to a row, Title as the safe fallback when
// evaluation fails or the expression returns empty, Context as the
// harvested ExpressionItems.
type Record struct {
	Filename string
	Title    string
	Context  map[string]any
}

// Manager owns the Engine + the two providers. Constructed in app
// wiring; Configure can be called at runtime if providers ever need
// to swap (today they don't — managers live for the app's lifetime).
type Manager struct {
	eng *engine
	tpl TemplateProvider
	sto StorageProvider
}

// NewManager builds a Manager with default helpers and the two
// providers wired. Either provider may be nil at construction time —
// the public methods that need them will return a clear error rather
// than panic, so test setups that exercise only Evaluate (no
// template/storage) can pass nils.
func NewManager(tpl TemplateProvider, sto StorageProvider) *Manager {
	return &Manager{eng: newEngine(), tpl: tpl, sto: sto}
}

// ErrNoExpression is returned when EvaluateSidebar is called against
// a template that has no sidebar_expression configured. The frontend
// should hide the sub-label entirely in this case rather than show a
// fallback — there's nothing to render.
var ErrNoExpression = errors.New("template has no sidebar_expression")

// Evaluate runs one expression against an arbitrary context. The
// public single-shot path — used by Wails callers and (later) plugin
// authors. ctx may be nil.
func (m *Manager) Evaluate(src string, ctx map[string]any) (SidebarItem, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	return m.eng.Evaluate(src, ctx)
}

// EvaluateSidebar compiles the template's sidebar_expression once and
// runs it against every record's harvested ExpressionItems. Per-row
// failures are isolated: the SidebarItem for that row carries the
// error in its Error field with Text falling back to the record
// title, so a sidebar of 100 records with one bad date doesn't blank
// out 99 valid rows.
func (m *Manager) EvaluateSidebar(templateName string) ([]SidebarItem, error) {
	if m.tpl == nil || m.sto == nil {
		return nil, fmt.Errorf("expression: providers not wired")
	}

	src, expressionFields, err := m.tpl.LookupSidebar(templateName)
	if err != nil {
		return nil, fmt.Errorf("expression: load template %q: %w", templateName, err)
	}
	if src == "" {
		return nil, ErrNoExpression
	}

	// Compile once — Evaluate would re-compile per record otherwise.
	// We discard the program here because Evaluate hits the cache by
	// source key, but warming the cache up-front means a syntax error
	// surfaces once instead of N times.
	if _, err := m.eng.Compile(src); err != nil {
		return nil, fmt.Errorf("expression: compile %q: %w", templateName, err)
	}

	records, err := m.sto.ListForExpression(templateName)
	if err != nil {
		return nil, fmt.Errorf("expression: list records: %w", err)
	}

	out := make([]SidebarItem, 0, len(records))
	for _, r := range records {
		ctx := narrowContext(r.Context, expressionFields)
		item, err := m.eng.Evaluate(src, ctx)
		if err != nil {
			out = append(out, SidebarItem{
				Filename: r.Filename,
				Text:     r.Title,
				Error:    err.Error(),
				Classes:  []string{"expr-error"},
			})
			continue
		}
		item.Filename = r.Filename
		// Preserve the title fallback when the expression returns
		// empty text — sidebar rows must always have something to
		// show, even if the expression evaluated to "".
		if item.Text == "" {
			item.Text = r.Title
		}
		out = append(out, item)
	}
	return out, nil
}

// narrowContext returns a copy of ctx limited to expressionFields.
// A defence-in-depth: even though ListForExpression already narrows
// to ExpressionItem-flagged fields at storage harvest time, this
// double-checks here so a buggy or stale storage layer can't leak a
// field the user never opted in to expose.
//
// Empty expressionFields means "trust the storage harvest" (the
// template provider returned no opt-in field list); the context
// passes through unchanged.
func narrowContext(ctx map[string]any, expressionFields []string) map[string]any {
	if len(expressionFields) == 0 {
		return ctx
	}
	allow := make(map[string]struct{}, len(expressionFields))
	for _, k := range expressionFields {
		allow[k] = struct{}{}
	}
	out := make(map[string]any, len(ctx))
	for k, v := range ctx {
		if _, ok := allow[k]; ok {
			out[k] = v
		}
	}
	return out
}
