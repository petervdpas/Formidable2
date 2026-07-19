package template

import "encoding/json"

// The event field's value is a single time-bar placed on a project board: a
// task, a milestone, or a resource absence. Start/End are ISO dates
// (YYYY-MM-DD); a milestone is a zero-span point (End empty or equal to Start).
// Resource names who the bar belongs to. The board renderer lays each record's
// event onto its assigned project's shared axis; a later step caps Start/End to
// that project's date range.

// EventKindMilestone is the one reserved kind token: a bar whose kind is
// "milestone" renders as a zero-span point. Every other kind is author-defined
// on the event field's options; there is no built-in vocabulary.
const EventKindMilestone = "milestone"

// EventDoc is the stored value of an event field: a placement on the project
// board's two axes. Start/End are the X (time) span (ISO dates; an empty End, or
// End == Start, is a zero-span milestone). Resource is the Y axis: which of the
// project's author-defined resources (rows) this event sits in. A note about the
// bar is not part of the event: add a sibling field to the events loop for that.
type EventDoc struct {
	Start    string `json:"start"`
	End      string `json:"end,omitempty"`
	Kind     string `json:"kind"`
	Resource string `json:"resource,omitempty"`
}

// ParseEventDoc decodes a stored event value (a decoded map[string]any) into an
// EventDoc. A nil value is an empty doc. Round-trips via JSON so the shape is
// preserved exactly.
func ParseEventDoc(v any) (EventDoc, error) {
	var doc EventDoc
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
