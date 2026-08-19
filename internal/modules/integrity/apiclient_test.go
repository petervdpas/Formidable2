package integrity

import "testing"

// The doctor must read an api-client snapshot as the intended shape, not as
// debris. It is an object (single) or an array of objects (to-many).

func TestCheckValueType_ApiClientSnapshotIsClean(t *testing.T) {
	snap := map[string]any{
		"id":     "cust-42",
		"label":  "Acme BV",
		"fields": map[string]any{"city": "Utrecht"},
	}
	if got := checkValueType("api-client", snap, "customer"); len(got) != 0 {
		t.Errorf("snapshot flagged: %+v", got)
	}
}

func TestCheckValueType_ApiClientMultiIsClean(t *testing.T) {
	multi := []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
	if got := checkValueType("api-client", multi, "customers"); len(got) != 0 {
		t.Errorf("multi snapshot flagged: %+v", got)
	}
}

func TestCheckValueType_ApiClientUnpickedIsClean(t *testing.T) {
	if got := checkValueType("api-client", nil, "customer"); len(got) != 0 {
		t.Errorf("unpicked flagged: %+v", got)
	}
}

func TestCheckValueType_ApiClientNumberIsFlagged(t *testing.T) {
	if got := checkValueType("api-client", 42, "customer"); len(got) == 0 {
		t.Error("a number is not a snapshot; expected a type-mismatch issue")
	}
}

func TestShapeOfFieldType_ApiClientIsObject(t *testing.T) {
	if got := shapeOfFieldType("api-client"); got != "object" {
		t.Errorf("shapeOfFieldType(api-client) = %q, want object", got)
	}
}

func TestDefaultForFieldType_ApiClientIsNil(t *testing.T) {
	if got := defaultForFieldType("api-client"); got != nil {
		t.Errorf("defaultForFieldType(api-client) = %#v, want nil", got)
	}
}
