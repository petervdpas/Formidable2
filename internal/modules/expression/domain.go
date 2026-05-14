package expression

import (
	"errors"
	"fmt"
)

// TemplateProvider abstracts the template module so this package can
// stay layer-clean (no template import → no risk of an import cycle
// when template eventually wants to consume an Expression hook).
// Returned `Template` is opaque to the engine; we read only what the
// sidebar needs.
type TemplateProvider interface {
	// LookupExpression returns the template's sidebar expression and the
	// list of expression-flagged fields with their option metadata.
	// The fields slice backs both narrowContext (defence-in-depth on
	// what the expression can see) and the per-record O map that
	// resolves O["key"] to the option label of the field's current
	// value at evaluation time.
	LookupExpression(name string) (expr string, fields []ExpressionField, err error)
}

// ExpressionField is the slim shape the sidebar evaluator needs from
// each expression-flagged field: the variable Key (used for
// narrowContext + O[key] resolution) and Options as a value→label
// map. Fields without options carry an empty Options map.
type ExpressionField struct {
	Key     string
	Options map[string]string
}

// StorageProvider abstracts the storage module's list-with-context
// surface. Each Record carries the harvested ExpressionItems map for
// one record; that map IS the per-row evaluation context, so the
// engine never reads template Field metadata at evaluate time.
type StorageProvider interface {
	ListForExpression(templateName string) ([]Record, error)
	// LookupForExpression returns one record by datafile so the
	// per-item evaluation path (EvaluateListOne) doesn't have to
	// walk every record. Returns (Record{}, nil) when the file is
	// missing — callers should treat that as "no row to render".
	LookupForExpression(templateName, datafile string) (Record, error)
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

// ErrNoExpression is returned when EvaluateList is called against
// a template that has no sidebar_expression configured. The frontend
// should hide the sub-label entirely in this case rather than show a
// fallback — there's nothing to render.
var ErrNoExpression = errors.New("template has no sidebar_expression")

// Evaluate runs one expression against an arbitrary context. The
// public single-shot path — used by Wails callers and (later) plugin
// authors. ctx may be nil.
func (m *Manager) Evaluate(src string, ctx map[string]any) (Result, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	return m.eng.Evaluate(src, ctx)
}

// EvaluateList compiles the template's sidebar_expression once and
// runs it against every record's harvested ExpressionItems. Per-row
// failures are isolated: the Result for that row carries the
// error in its Error field with Text falling back to the record
// title, so a sidebar of 100 records with one bad date doesn't blank
// out 99 valid rows.
func (m *Manager) EvaluateList(templateName string) ([]Result, error) {
	if m.tpl == nil || m.sto == nil {
		return nil, fmt.Errorf("expression: providers not wired")
	}

	src, fields, err := m.tpl.LookupExpression(templateName)
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

	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}

	out := make([]Result, 0, len(records))
	for _, r := range records {
		ctx := narrowContext(r.Context, keys)
		ctx["O"] = optionLabelMap(fields, ctx)
		item, err := m.eng.Evaluate(src, ctx)
		if err != nil {
			out = append(out, Result{
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

// EvaluateListOne is the per-record analogue of EvaluateList.
// Loads only the one record's harvested ExpressionItems and runs the
// template's sidebar expression against it. Used by self-serving list
// items refreshing themselves after a save, so a single row change
// does not require a full-list re-evaluation.
//
// Returns ErrNoExpression when the template has no sidebar_expression,
// matching the bulk method's contract. Missing file → (zero
// Result, nil) so the caller can distinguish "this row no longer
// exists" from "rendering failed".
func (m *Manager) EvaluateListOne(templateName, datafile string) (Result, error) {
	if m.tpl == nil || m.sto == nil {
		return Result{}, fmt.Errorf("expression: providers not wired")
	}

	src, fields, err := m.tpl.LookupExpression(templateName)
	if err != nil {
		return Result{}, fmt.Errorf("expression: load template %q: %w", templateName, err)
	}
	if src == "" {
		return Result{}, ErrNoExpression
	}
	if _, err := m.eng.Compile(src); err != nil {
		return Result{}, fmt.Errorf("expression: compile %q: %w", templateName, err)
	}

	r, err := m.sto.LookupForExpression(templateName, datafile)
	if err != nil {
		return Result{}, fmt.Errorf("expression: lookup record %q/%q: %w", templateName, datafile, err)
	}
	if r.Filename == "" {
		return Result{}, nil
	}

	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	ctx := narrowContext(r.Context, keys)
	ctx["O"] = optionLabelMap(fields, ctx)
	item, err := m.eng.Evaluate(src, ctx)
	if err != nil {
		return Result{
			Filename: r.Filename,
			Text:     r.Title,
			Error:    err.Error(),
			Classes:  []string{"expr-error"},
		}, nil
	}
	item.Filename = r.Filename
	if item.Text == "" {
		item.Text = r.Title
	}
	return item, nil
}

// EvaluateListMany renders sub-labels for an explicit list of
// records, returning items in the same order as the input filenames.
// Used by the Storage workspace on initial mount and Refresh to
// collapse N parallel EvaluateListOne IPC calls into one. Missing
// files emit a zero Result at that slot; per-record evaluation
// errors carry an Error field with the title as Text — same isolation
// posture as EvaluateList.
func (m *Manager) EvaluateListMany(templateName string, datafiles []string) ([]Result, error) {
	if m.tpl == nil || m.sto == nil {
		return nil, fmt.Errorf("expression: providers not wired")
	}

	src, fields, err := m.tpl.LookupExpression(templateName)
	if err != nil {
		return nil, fmt.Errorf("expression: load template %q: %w", templateName, err)
	}
	if src == "" {
		return nil, ErrNoExpression
	}
	if _, err := m.eng.Compile(src); err != nil {
		return nil, fmt.Errorf("expression: compile %q: %w", templateName, err)
	}

	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}

	out := make([]Result, 0, len(datafiles))
	for _, df := range datafiles {
		r, lerr := m.sto.LookupForExpression(templateName, df)
		if lerr != nil {
			out = append(out, Result{
				Filename: df,
				Error:    lerr.Error(),
				Classes:  []string{"expr-error"},
			})
			continue
		}
		if r.Filename == "" {
			out = append(out, Result{})
			continue
		}
		ctx := narrowContext(r.Context, keys)
		ctx["O"] = optionLabelMap(fields, ctx)
		item, eerr := m.eng.Evaluate(src, ctx)
		if eerr != nil {
			out = append(out, Result{
				Filename: r.Filename,
				Text:     r.Title,
				Error:    eerr.Error(),
				Classes:  []string{"expr-error"},
			})
			continue
		}
		item.Filename = r.Filename
		if item.Text == "" {
			item.Text = r.Title
		}
		out = append(out, item)
	}
	return out, nil
}

// optionLabelMap builds the per-record `O` env entry: a map keyed
// by expression-field key whose value is the option label that
// resolves the record's current value for that field. Fields with
// no options or with a value not present in the option list emit
// an empty string — runtime O[key] then stringifies as "" rather
// than blowing up. Builder.Compile only emits O[key] for fields
// the UI gates as enum-typed, so missing entries here mean stale
// config rather than expected absence.
func optionLabelMap(fields []ExpressionField, ctx map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if len(f.Options) == 0 {
			continue
		}
		raw, ok := ctx[f.Key]
		if !ok || raw == nil {
			continue
		}
		val := fmt.Sprintf("%v", raw)
		if label, ok := f.Options[val]; ok {
			out[f.Key] = label
		} else {
			// Unknown value: fall back to the raw value so a stale
			// option set doesn't blank a chip. Same fallback the
			// previous baked ternary used.
			out[f.Key] = val
		}
	}
	return out
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
