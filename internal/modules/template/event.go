package template

import "encoding/json"

// The event field's value is a single time-bar placed on a project board: a
// task, a milestone, or a resource absence. Start/End are ISO dates
// (YYYY-MM-DD); a milestone is a zero-span point (End empty or equal to Start).
// Resource names who the bar belongs to. The board renderer lays each record's
// event onto its assigned project's shared axis; a later step caps Start/End to
// that project's date range.

// Event kind discriminator. A task is a work bar, a milestone a point marker,
// an absence a resource's unavailability (e.g. a vacation).
const (
	EventKindTask      = "task"
	EventKindMilestone = "milestone"
	EventKindAbsence   = "absence"
)

// EventKindDescriptor names one event kind for the editor's kind picker.
// Name is the stored token; LabelKey is its i18n label.
type EventKindDescriptor struct {
	Name     string `json:"name"`
	LabelKey string `json:"label_key"`
}

// builtinEventKinds is the event kind palette; display order is significant.
var builtinEventKinds = []EventKindDescriptor{
	{Name: EventKindTask, LabelKey: "workspace.templates.event.kind.task"},
	{Name: EventKindMilestone, LabelKey: "workspace.templates.event.kind.milestone"},
	{Name: EventKindAbsence, LabelKey: "workspace.templates.event.kind.absence"},
}

// EventDoc is the stored value of an event field. Start/End are ISO dates; an
// empty End (or End == Start) is a zero-span point (a milestone).
type EventDoc struct {
	Start    string `json:"start"`
	End      string `json:"end,omitempty"`
	Kind     string `json:"kind"`
	Resource string `json:"resource,omitempty"`
}

// EventKinds returns a defensive copy of the kind vocabulary (Wails-exposed so
// the editor reads the set from the backend, never a hardcoded JS list).
func EventKinds() []EventKindDescriptor {
	out := make([]EventKindDescriptor, len(builtinEventKinds))
	copy(out, builtinEventKinds)
	return out
}

// IsEventKind reports whether kind is an allowed event kind.
func IsEventKind(kind string) bool {
	for _, k := range builtinEventKinds {
		if k.Name == kind {
			return true
		}
	}
	return false
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
