// Package query is a constrained, read-only SELECT surface over a
// template's form data. It prepares an in-memory string matrix from the
// forms (referenced tables cartesian-exploded with provenance) and runs
// the Spec over it: project, filter, distinct, sort, group, aggregate.
// Scope is deliberately single-template: no cross-template joins, no
// subqueries, no user SQL.
package query

import (
	"fmt"
	"strings"
)

// Manager prepares a matrix from the template's forms (via Loader) then
// executes the Spec over it.
type Manager struct {
	loader Loader
}

func NewManager(loader Loader) *Manager { return &Manager{loader: loader} }

const defaultCountHeader = "count"

// FilterOps is the comparison operators the engine accepts, in display
// order. eq/ne compare as text; lt/le/gt/ge coerce to number. Backend-
// owned so the operator picker can't drift from the engine.
var FilterOps = []string{"eq", "ne", "lt", "le", "gt", "ge"}

// Run prepares the matrix and executes the spec.
func (m *Manager) Run(spec Spec) (Result, error) {
	if strings.TrimSpace(spec.Template) == "" {
		return Result{}, fmt.Errorf("query: template required")
	}
	mx, err := Prepare(spec, m.loader)
	if err != nil {
		return Result{}, err
	}
	return mx.Execute(spec)
}

func headers(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

func countHeader(spec Spec) string {
	if spec.CountHeader != "" {
		return spec.CountHeader
	}
	return defaultCountHeader
}
