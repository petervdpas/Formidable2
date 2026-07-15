package template

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The project field defines a plan board. Its shared time axis (from/to dates
// and a time-block granularity) is author-time config carried in the field's
// options, like a slide's canvas format. The per-record value is the board's
// name. Events in the same template are laid on this axis and (a later step)
// capped to the from/to window.

// ProjectDoc is the stored per-record value of a project field: the board's
// name. The axis (dates + granularity) lives in the field options, not here.
type ProjectDoc struct {
	Name string `json:"name"`
}

// Time-block granularity for a project's axis: the width of one column on the
// board. A task bar spans whole blocks; the renderer ticks the axis by these.
const (
	TimeBlockDay   = "day"
	TimeBlockWeek  = "week"
	TimeBlock2Week = "2-week"
	TimeBlock3Week = "3-week"
	TimeBlockMonth = "month"
)

var builtinTimeBlocks = []string{
	TimeBlockDay, TimeBlockWeek, TimeBlock2Week, TimeBlock3Week, TimeBlockMonth,
}

// TimeBlocks returns a defensive copy of the time-block vocabulary (Wails-exposed
// so the options editor reads the set from the backend, never a hardcoded list).
func TimeBlocks() []string {
	out := make([]string, len(builtinTimeBlocks))
	copy(out, builtinTimeBlocks)
	return out
}

// IsTimeBlock reports whether s is a known time-block granularity.
func IsTimeBlock(s string) bool {
	return slices.Contains(builtinTimeBlocks, s)
}

// projectOption reads a project field option's label cell by its locked value
// key (the axis settings are stored one-per-row like slide's canvas options).
func projectOption(f Field, key string) string {
	for _, opt := range f.Options {
		if m, ok := opt.(map[string]any); ok {
			if v, _ := m["value"].(string); v == key {
				return strings.TrimSpace(fmt.Sprint(m["label"]))
			}
		}
	}
	return ""
}

// ProjectDateRange reads the board's authored axis window (ISO from/to), or ""
// for an unset endpoint. The board renderer clamps events to this window.
func ProjectDateRange(f Field) (from, to string) {
	return projectOption(f, "from"), projectOption(f, "to")
}

// ProjectTimeBlock reads the board's authored axis granularity, defaulting to
// weekly when unset or unrecognised (the whiteboard's wk-column cadence).
func ProjectTimeBlock(f Field) string {
	if tb := projectOption(f, "timeblock"); IsTimeBlock(tb) {
		return tb
	}
	return TimeBlockWeek
}

// ParseProjectDoc decodes a stored project value (a decoded map[string]any) into
// a ProjectDoc. A nil value is an empty doc. Round-trips via JSON so the shape is
// preserved exactly.
func ParseProjectDoc(v any) (ProjectDoc, error) {
	var doc ProjectDoc
	if v == nil {
		return doc, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}
