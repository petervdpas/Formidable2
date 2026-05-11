package template

import (
	"fmt"
	"regexp"
)

const MaxFlagDefinitions = 16

// FlagColors is the closed set of color tokens a FlagDefinition may
// reference. Mirrors the 16 .expr-bg-* utilities in expression.css —
// designers pick from this palette so dark/light themes stay coherent.
var FlagColors = map[string]struct{}{
	"red": {}, "orange": {}, "amber": {}, "yellow": {},
	"green": {}, "teal": {}, "blue": {}, "purple": {},
	"pink": {}, "gray": {},
	"cyan": {}, "lime": {}, "indigo": {}, "rose": {},
	"brown": {}, "slate": {},
}

var flagLabelRe = regexp.MustCompile(`^[A-Z][A-Z0-9 _-]*$`)

func flagDefinitionsErrors(defs []FlagDefinition) []ValidationError {
	if len(defs) == 0 {
		return nil
	}
	var errs []ValidationError
	if len(defs) > MaxFlagDefinitions {
		errs = append(errs, ValidationError{
			Type:    "too-many-flag-definitions",
			Message: fmt.Sprintf("At most %d flag_definitions are allowed (found %d)", MaxFlagDefinitions, len(defs)),
		})
	}
	seen := map[string]struct{}{}
	for i, d := range defs {
		if !flagLabelRe.MatchString(d.Label) {
			errs = append(errs, ValidationError{
				Type:    "invalid-flag-label",
				Index:   i,
				Key:     d.Label,
				Message: fmt.Sprintf("Flag label %q must match ^[A-Z][A-Z0-9 _-]*$", d.Label),
			})
		} else if _, dup := seen[d.Label]; dup {
			errs = append(errs, ValidationError{
				Type:    "duplicate-flag-label",
				Index:   i,
				Key:     d.Label,
				Message: fmt.Sprintf("Duplicate flag label %q", d.Label),
			})
		} else {
			seen[d.Label] = struct{}{}
		}
		if _, ok := FlagColors[d.Color]; !ok {
			errs = append(errs, ValidationError{
				Type:    "unknown-flag-color",
				Index:   i,
				Key:     d.Label,
				Detail:  map[string]any{"color": d.Color},
				Message: fmt.Sprintf("Unknown flag color %q for label %q", d.Color, d.Label),
			})
		}
	}
	return errs
}
