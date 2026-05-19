package template

import (
	"fmt"
	"regexp"
)

const (
	MaxFacets          = 5
	MaxOptionsPerFacet = 16
)

// FacetColors is the closed set of color tokens a FacetOption may
// reference. Mirrors the 16 .expr-bg-* utilities in expression.css —
// designers pick from this palette so dark/light themes stay coherent.
var FacetColors = map[string]struct{}{
	"red": {}, "orange": {}, "amber": {}, "yellow": {},
	"green": {}, "teal": {}, "blue": {}, "purple": {},
	"pink": {}, "gray": {},
	"cyan": {}, "lime": {}, "indigo": {}, "rose": {},
	"brown": {}, "slate": {},
}

// FacetIcons is the curated 16-icon FontAwesome palette a Facet may
// declare. Kept in sync with frontend/src/utils/facetColors.ts
// FACET_ICONS — the editor renders these as the icon swatch grid.
var FacetIcons = map[string]struct{}{
	"fa-flag": {}, "fa-check": {}, "fa-star": {}, "fa-heart": {},
	"fa-bookmark": {}, "fa-bell": {}, "fa-shirt": {}, "fa-circle-info": {},
	"fa-triangle-exclamation": {}, "fa-circle-question": {}, "fa-eye": {}, "fa-clock": {},
	"fa-tag": {}, "fa-bug": {}, "fa-gear": {}, "fa-fire": {},
}

var (
	facetKeyRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	facetLabelRe = regexp.MustCompile(`^[A-Z][A-Z0-9 _-]*$`)
)

// IsKnownFacetColor reports whether c is a valid color token.
func IsKnownFacetColor(c string) bool {
	_, ok := FacetColors[c]
	return ok
}

// IsKnownFacetIcon reports whether icon is a valid FontAwesome key
// from the curated palette.
func IsKnownFacetIcon(icon string) bool {
	_, ok := FacetIcons[icon]
	return ok
}

func facetsErrors(facets []Facet) []ValidationError {
	if len(facets) == 0 {
		return nil
	}
	var errs []ValidationError
	if len(facets) > MaxFacets {
		errs = append(errs, ValidationError{
			Type:    "too-many-facets",
			Message: fmt.Sprintf("At most %d facets are allowed (found %d)", MaxFacets, len(facets)),
		})
	}
	seenKeys := map[string]struct{}{}
	for i, f := range facets {
		if !facetKeyRe.MatchString(f.Key) {
			errs = append(errs, ValidationError{
				Type:    "invalid-facet-key",
				Index:   i,
				Key:     f.Key,
				Message: fmt.Sprintf("Facet key %q must match ^[a-z][a-z0-9_-]*$", f.Key),
			})
		} else if _, dup := seenKeys[f.Key]; dup {
			errs = append(errs, ValidationError{
				Type:    "duplicate-facet-key",
				Index:   i,
				Key:     f.Key,
				Message: fmt.Sprintf("Duplicate facet key %q", f.Key),
			})
		} else {
			seenKeys[f.Key] = struct{}{}
		}
		if f.Icon == "" {
			errs = append(errs, ValidationError{
				Type:    "missing-facet-icon",
				Index:   i,
				Key:     f.Key,
				Message: fmt.Sprintf("Facet %q is missing an icon", f.Key),
			})
		} else if !IsKnownFacetIcon(f.Icon) {
			errs = append(errs, ValidationError{
				Type:    "unknown-facet-icon",
				Index:   i,
				Key:     f.Key,
				Detail:  map[string]any{"icon": f.Icon},
				Message: fmt.Sprintf("Facet %q icon %q is not in the curated palette", f.Key, f.Icon),
			})
		}
		if len(f.Options) == 0 {
			errs = append(errs, ValidationError{
				Type:    "empty-facet-options",
				Index:   i,
				Key:     f.Key,
				Message: fmt.Sprintf("Facet %q must declare at least one option", f.Key),
			})
		}
		if len(f.Options) > MaxOptionsPerFacet {
			errs = append(errs, ValidationError{
				Type:    "too-many-facet-options",
				Index:   i,
				Key:     f.Key,
				Message: fmt.Sprintf("Facet %q has %d options; at most %d allowed", f.Key, len(f.Options), MaxOptionsPerFacet),
			})
		}
		errs = append(errs, facetOptionsErrors(i, f)...)
	}
	return errs
}

func facetOptionsErrors(facetIdx int, f Facet) []ValidationError {
	var errs []ValidationError
	seenLabels := map[string]struct{}{}
	for j, o := range f.Options {
		if !facetLabelRe.MatchString(o.Label) {
			errs = append(errs, ValidationError{
				Type:    "invalid-facet-label",
				Index:   facetIdx,
				Key:     f.Key,
				Detail:  map[string]any{"option_index": j, "label": o.Label},
				Message: fmt.Sprintf("Facet %q option %d label %q must match ^[A-Z][A-Z0-9 _-]*$", f.Key, j, o.Label),
			})
		} else if _, dup := seenLabels[o.Label]; dup {
			errs = append(errs, ValidationError{
				Type:    "duplicate-facet-label",
				Index:   facetIdx,
				Key:     f.Key,
				Detail:  map[string]any{"option_index": j, "label": o.Label},
				Message: fmt.Sprintf("Facet %q has duplicate option label %q", f.Key, o.Label),
			})
		} else {
			seenLabels[o.Label] = struct{}{}
		}
		if !IsKnownFacetColor(o.Color) {
			errs = append(errs, ValidationError{
				Type:    "unknown-facet-color",
				Index:   facetIdx,
				Key:     f.Key,
				Detail:  map[string]any{"option_index": j, "label": o.Label, "color": o.Color},
				Message: fmt.Sprintf("Facet %q option %q has unknown color %q", f.Key, o.Label, o.Color),
			})
		}
	}
	return errs
}
