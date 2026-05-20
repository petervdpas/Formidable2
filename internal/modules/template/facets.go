package template

import (
	"fmt"
	"regexp"
)

const (
	MaxFacets          = 5
	MaxOptionsPerFacet = 16
)

// FacetColorList is the ordered closed set of color tokens a
// FacetOption may reference. Mirrors the 16 .expr-bg-* utilities in
// expression.css — order matters because the frontend renders this
// list as the swatch grid. The Service exposes it to the frontend
// via FacetLimits.
var FacetColorList = []string{
	"red", "orange", "amber", "yellow",
	"green", "teal", "blue", "purple",
	"pink", "gray", "cyan", "lime",
	"indigo", "rose", "brown", "slate",
}

// FacetIconList is the ordered closed set of FontAwesome icon keys a
// Facet may declare. Same display-order contract as FacetColorList.
var FacetIconList = []string{
	"fa-flag", "fa-check", "fa-star", "fa-heart",
	"fa-bookmark", "fa-bell", "fa-shirt", "fa-circle-info",
	"fa-triangle-exclamation", "fa-circle-question", "fa-user", "fa-clock",
	"fa-tag", "fa-bug", "fa-gear", "fa-fire",
}

// FacetColors and FacetIcons are lookup sets derived from the ordered
// lists above; kept for O(1) membership checks during validation.
var (
	FacetColors = listToSet(FacetColorList)
	FacetIcons  = listToSet(FacetIconList)
)

const (
	FacetKeyPattern   = `^[a-z][a-z0-9_-]*$`
	FacetLabelPattern = `^[A-Z][A-Z0-9 _-]*$`
)

var (
	facetKeyRe   = regexp.MustCompile(FacetKeyPattern)
	facetLabelRe = regexp.MustCompile(FacetLabelPattern)
)

func listToSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, k := range items {
		out[k] = struct{}{}
	}
	return out
}

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

// FacetMeta is the wire-shape returned to the frontend so it can
// render the editor without hardcoding ANY backend constraint. The
// frontend reads this once at boot via Service.FacetMeta and treats
// the backend as the single source of truth for:
//   - max counts (MaxFacets, MaxOptionsPerFacet)
//   - palettes (Colors, Icons — ordered for display)
//   - validation patterns (KeyPattern, LabelPattern — compiled in JS)
//
// Adding a new backend-owned facet rule means extending this struct;
// the frontend stays a thin renderer.
type FacetMeta struct {
	MaxFacets          int                      `json:"max_facets"`
	MaxOptionsPerFacet int                      `json:"max_options_per_facet"`
	Colors             []string                 `json:"colors"`
	Icons              []string                 `json:"icons"`
	IconSVGs           map[string]FacetIconSpec `json:"icon_svgs"`
	KeyPattern         string                   `json:"key_pattern"`
	LabelPattern       string                   `json:"label_pattern"`
}

// GetFacetMeta returns a snapshot of the current facet constraints.
// Exposed via the Wails Service.
func GetFacetMeta() FacetMeta {
	colors := make([]string, len(FacetColorList))
	copy(colors, FacetColorList)
	icons := make([]string, len(FacetIconList))
	copy(icons, FacetIconList)
	svgs := make(map[string]FacetIconSpec, len(FacetIconSVGs))
	for k, v := range FacetIconSVGs {
		svgs[k] = v
	}
	return FacetMeta{
		MaxFacets:          MaxFacets,
		MaxOptionsPerFacet: MaxOptionsPerFacet,
		Colors:             colors,
		Icons:              icons,
		IconSVGs:           svgs,
		KeyPattern:         FacetKeyPattern,
		LabelPattern:       FacetLabelPattern,
	}
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
