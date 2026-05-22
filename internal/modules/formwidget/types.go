// Package formwidget models form widgets - the non-input slots a
// plugin author can place inside a plugin's Run-dialog form. Widgets
// don't capture user input; they DISPLAY runtime state pushed from
// the Lua side (formidable.run.bar, formidable.run.status) so the
// user sees what the plugin is doing while it's working.
//
// Widgets are intentionally a separate concept from template Fields:
//
//   - Fields appear in the templates field-type picker, hold values,
//     and round-trip through frontmatter.
//   - Widgets appear ONLY in the plugin form editor, hold no values,
//     and exist solely as live-update display slots.
//
// Keeping them in a dedicated module prevents the field-types
// registry (internal/modules/template) from picking them up, and
// lets plugin form storage own a discrete widgets.json next to the
// existing form.json without inflating the template field surface.
package formwidget

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Kind identifies a widget's display role. Closed enum - adding a
// new widget kind is a deliberate package change with a matching
// frontend component, never an open-ended string.
type Kind string

const (
	// KindProgressBar renders a progress bar fed by RunBarEvent
	// (formidable.run.bar). Total == 0 → indeterminate animation.
	KindProgressBar Kind = "progressbar"

	// KindStatusMessage renders a single-line text label fed by
	// RunStatusEvent (formidable.run.status). Plugin authors
	// typically push the current item's name/path here so the
	// user sees "what's happening right now".
	KindStatusMessage Kind = "statusmessage"
)

// Widget is one entry inside a plugin's form.json - the same list
// that holds form Fields. The author places widgets wherever they
// want among the fields; position in the array IS the render order.
// ID is a per-form stable identifier the form editor uses for
// reordering / deletion (it does NOT route Lua updates - both
// widget kinds are driven by a single run-scoped event each).
// Label is optional chrome shown next to / above the widget.
//
// Heterogeneity in form.json: Widget and template.Field share the
// same array. Frontend dispatches by the presence of the `kind`
// field - Field uses `type`, Widget uses `kind` - so no extra
// discriminator is needed in the JSON.
type Widget struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Label string `json:"label,omitempty"`
}

// ErrWidgetInvalid wraps every validation failure. Callers use
// errors.Is to branch; the wrapped string carries the specific
// detail for logging / error display.
var ErrWidgetInvalid = errors.New("formwidget: invalid widget")

// validIDRe constrains widget IDs to a tight subset so they're safe
// as map keys, dom IDs, and JSON keys without escaping. Same shape
// used by the plugin module's id validator.
var validIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate enforces shape: non-empty kind in the closed set, ID
// matching validIDRe. Used by both load (refuses unknown widgets.json
// content) and save (refuses corrupt UI submissions) so the failure
// surfaces at the same boundary in both directions.
func (w Widget) Validate() error {
	id := strings.TrimSpace(w.ID)
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrWidgetInvalid)
	}
	if !validIDRe.MatchString(id) {
		return fmt.Errorf("%w: id %q must match %s", ErrWidgetInvalid, id, validIDRe.String())
	}
	switch w.Kind {
	case KindProgressBar, KindStatusMessage:
		// ok
	case "":
		return fmt.Errorf("%w: empty kind for id %q", ErrWidgetInvalid, id)
	default:
		return fmt.Errorf("%w: unknown kind %q for id %q", ErrWidgetInvalid, w.Kind, id)
	}
	return nil
}

// ValidateAll runs Validate on every widget and additionally checks
// that IDs are unique within the slice. Returns the first error
// encountered so the UI can highlight a specific offender instead
// of a vague "list is invalid".
func ValidateAll(ws []Widget) error {
	seen := make(map[string]struct{}, len(ws))
	for i, w := range ws {
		if err := w.Validate(); err != nil {
			return fmt.Errorf("widget[%d]: %w", i, err)
		}
		if _, dup := seen[w.ID]; dup {
			return fmt.Errorf("%w: duplicate id %q at widget[%d]", ErrWidgetInvalid, w.ID, i)
		}
		seen[w.ID] = struct{}{}
	}
	return nil
}
