package expression

import (
	"errors"
	"fmt"
)

// TemplateProvider is the template surface the sidebar evaluator needs.
type TemplateProvider interface {
	LookupExpression(name string) (expr string, fields []ExpressionField, err error)
}

// ExpressionField carries an expression-flagged field's Key and a value->label Options map.
type ExpressionField struct {
	Key     string
	Options map[string]string
}

// StorageProvider is the storage list-with-context surface.
type StorageProvider interface {
	ListForExpression(templateName string) ([]Record, error)
	// LookupForExpression returns one record by datafile; (Record{}, nil) means the file is missing.
	LookupForExpression(templateName, datafile string) (Record, error)
}

// Record pairs a result back to a row: Filename, Title (fallback), and the harvested Context.
type Record struct {
	Filename string
	Title    string
	Context  map[string]any
}

// Manager owns the engine and the two providers.
type Manager struct {
	eng *engine
	tpl TemplateProvider
	sto StorageProvider
}

// NewManager wires the engine and providers; either provider may be nil (the methods needing it error, not panic).
func NewManager(tpl TemplateProvider, sto StorageProvider) *Manager {
	return &Manager{eng: newEngine(), tpl: tpl, sto: sto}
}

// ErrNoExpression signals a template with no sidebar_expression; the frontend hides the sub-label entirely.
var ErrNoExpression = errors.New("template has no sidebar_expression")

// Evaluate runs one expression against an arbitrary context (ctx may be nil).
func (m *Manager) Evaluate(src string, ctx map[string]any) (Result, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	return m.eng.Evaluate(src, ctx)
}

// EvaluateValue runs one expression against an arbitrary context and returns
// the raw typed value (not a styled Result). This is the typed entry point that
// formula fields use: the datacore loader evaluates a formula per record and
// coerces the result to the formula's declared type.
func (m *Manager) EvaluateValue(src string, ctx map[string]any) (any, error) {
	if ctx == nil {
		ctx = map[string]any{}
	}
	return m.eng.EvaluateRaw(src, ctx)
}

// FormulaSpec is one computed field for EvaluateFormulas: a key, a declared
// result type (caller's concern for coercion), and its expression. Defined here
// so callers (the datacore loader, the index harvest) need not share a struct.
type FormulaSpec struct {
	Key        string
	Type       string
	Expression string
}

// EvaluateFormulas computes specs in declared order against ctx and returns the
// raw value per key. Each result is also written back into ctx so a later
// formula can reference an earlier one via F["..."]. A spec that fails to
// evaluate is skipped (absent from the result), so one bad formula doesn't
// abort the rest. The single source of formula evaluation: both the datacore
// loader (statistics) and the index harvest (expression engine) call this, so
// the value a chart sees and the value the sidebar sees are computed identically.
func (m *Manager) EvaluateFormulas(specs []FormulaSpec, ctx map[string]any) map[string]any {
	if ctx == nil {
		ctx = map[string]any{}
	}
	out := make(map[string]any, len(specs))
	for _, s := range specs {
		raw, err := m.eng.EvaluateRaw(s.Expression, ctx)
		if err != nil {
			continue
		}
		out[s.Key] = raw
		ctx[s.Key] = raw
	}
	return out
}

// EvaluateList compiles the sidebar_expression once and runs it against every record; per-row failures
// are isolated (the row's Error is set, Text falls back to the title) so one bad row doesn't blank the rest.
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

	// Warm the cache so a syntax error surfaces once, not N times (Evaluate hits the cache by source key).
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
		// Fall back to the title when the expression evaluates to "" so a row always has something to show.
		if item.Text == "" {
			item.Text = r.Title
		}
		out = append(out, item)
	}
	return out, nil
}

// EvaluateListOne is the per-record analogue of EvaluateList, for a row refreshing itself after a save.
// Missing file returns (zero Result, nil) so the caller can tell "row gone" from "render failed".
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

// EvaluateListMany renders sub-labels for an explicit ordered list of records; missing files emit a zero
// Result at that slot, same per-record isolation as EvaluateList.
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

// optionLabelMap builds the per-record O env: each enum field's current value resolved to its option label.
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
			// Unknown value: fall back to the raw value so a stale option set doesn't blank a chip.
			out[f.Key] = val
		}
	}
	return out
}

// narrowContext copies ctx limited to expressionFields (defence-in-depth so a stale storage layer can't
// leak an un-opted-in field); empty expressionFields trusts the storage harvest and passes ctx through.
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
