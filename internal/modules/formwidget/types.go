// Package formwidget models form widgets: non-input display slots in a plugin's
// Run-dialog form that show runtime state pushed from Lua (run.bar, run.status).
// Kept a separate concept from template Fields so the field-type registry never
// picks them up.
package formwidget

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Kind identifies a widget's display role. Closed enum: adding one is a
// deliberate package change with a matching frontend component.
type Kind string

const (
	// KindProgressBar renders a progress bar fed by RunBarEvent; Total 0 is
	// indeterminate.
	KindProgressBar Kind = "progressbar"

	// KindStatusMessage renders a single-line label fed by RunStatusEvent.
	KindStatusMessage Kind = "statusmessage"

	// KindChart renders an interactive chart: it owns the stat-object and
	// chart-shape pickers and drives the Lua call itself, unlike the
	// event-fed kinds.
	KindChart Kind = "chart"
)

// Widget is one entry in a plugin's form.json (sharing the array with template
// Fields; the frontend dispatches on "kind" vs Field's "type"). Array position
// is render order. ID is the editor's stable handle, not a Lua route.
type Widget struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Label string `json:"label,omitempty"`
}

// ErrWidgetInvalid wraps every validation failure (errors.Is to branch).
var ErrWidgetInvalid = errors.New("formwidget: invalid widget")

// validIDRe constrains widget IDs to a safe subset (matches the plugin id validator).
var validIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate enforces a non-empty ID matching validIDRe and a kind in the closed set.
func (w Widget) Validate() error {
	id := strings.TrimSpace(w.ID)
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrWidgetInvalid)
	}
	if !validIDRe.MatchString(id) {
		return fmt.Errorf("%w: id %q must match %s", ErrWidgetInvalid, id, validIDRe.String())
	}
	switch w.Kind {
	case KindProgressBar, KindStatusMessage, KindChart:
	case "":
		return fmt.Errorf("%w: empty kind for id %q", ErrWidgetInvalid, id)
	default:
		return fmt.Errorf("%w: unknown kind %q for id %q", ErrWidgetInvalid, w.Kind, id)
	}
	return nil
}

// ValidateAll validates each widget and checks IDs are unique, returning the
// first error.
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
