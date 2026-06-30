package template

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultSequenceStep is the sparse spacing a sequence field uses when its step
// option is missing or unparseable: gaps of 10 let a record slot between two
// positions (e.g. 15 between 10 and 20) without renumbering the rest.
const defaultSequenceStep = 10

// SequenceStep reads a sequence field's step magnitude from the option whose
// value=="step", defaulting to 10 and never below 1. Shared by auto-assign and
// the sidebar reorder so both space positions the same way.
func SequenceStep(f Field) int {
	for _, opt := range f.Options {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		if v, _ := m["value"].(string); v != "step" {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(m["label"]))
		if n, err := strconv.Atoi(s); err == nil && n >= 1 {
			return n
		}
	}
	return defaultSequenceStep
}
