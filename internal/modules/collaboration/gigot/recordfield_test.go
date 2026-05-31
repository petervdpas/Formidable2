package gigot

import (
	"encoding/json"
	"testing"
)

func TestRecordField_GetFromDataAndMeta(t *testing.T) {
	rec := []byte(`{"meta":{"flagged":false},"data":{"name":"Oak","count":3}}`)

	v, ok, err := getRecordField(rec, "data", "name")
	if err != nil || !ok {
		t.Fatalf("get data/name: ok=%v err=%v", ok, err)
	}
	if string(v) != `"Oak"` {
		t.Errorf("data/name = %s, want \"Oak\"", v)
	}

	v, ok, _ = getRecordField(rec, "data", "count")
	if !ok || string(v) != "3" {
		t.Errorf("data/count = %s ok=%v", v, ok)
	}

	v, ok, _ = getRecordField(rec, "meta", "flagged")
	if !ok || string(v) != "false" {
		t.Errorf("meta/flagged = %s ok=%v", v, ok)
	}

	if _, ok, _ := getRecordField(rec, "data", "missing"); ok {
		t.Error("missing field should report ok=false")
	}
}

func TestRecordField_SetRoundTrips(t *testing.T) {
	rec := []byte(`{"meta":{"flagged":false},"data":{"name":"Oak"}}`)

	out, err := setRecordField(rec, "data", "name", json.RawMessage(`"Yours"`))
	if err != nil {
		t.Fatal(err)
	}
	v, ok, _ := getRecordField(out, "data", "name")
	if !ok || string(v) != `"Yours"` {
		t.Fatalf("after set, data/name = %s", v)
	}
	// Other fields untouched.
	if v, _, _ := getRecordField(out, "meta", "flagged"); string(v) != "false" {
		t.Errorf("meta/flagged changed to %s", v)
	}
}

// Setting a previously-absent field adds it.
func TestRecordField_SetAddsMissingField(t *testing.T) {
	rec := []byte(`{"meta":{},"data":{"name":"Oak"}}`)
	out, err := setRecordField(rec, "data", "country", json.RawMessage(`"nl"`))
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := getRecordField(out, "data", "country"); !ok || string(v) != `"nl"` {
		t.Fatalf("country not added: %s ok=%v", v, ok)
	}
}

// copyFields moves the named field values from source into target (the
// primitive behind neutralize-to-theirs and apply-mine).
func TestCopyFields_MovesNamedFieldsOnly(t *testing.T) {
	target := []byte(`{"meta":{},"data":{"name":"Yours","country":"nl"}}`)
	source := []byte(`{"meta":{},"data":{"name":"Theirs","country":"uk"}}`)

	out, err := copyFields(target, source, []FieldResolution{
		{Scope: "data", Key: "name"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// name taken from source, country left as target's.
	if v, _, _ := getRecordField(out, "data", "name"); string(v) != `"Theirs"` {
		t.Errorf("name = %s, want Theirs", v)
	}
	if v, _, _ := getRecordField(out, "data", "country"); string(v) != `"nl"` {
		t.Errorf("country = %s, want nl (untouched)", v)
	}
}

// A nested object value round-trips intact (the field is atomic).
func TestRecordField_NestedValueAtomic(t *testing.T) {
	rec := []byte(`{"meta":{},"data":{"addr":{"city":"NYC"}}}`)
	theirs := json.RawMessage(`{"city":"LA","zip":"90001"}`)
	out, err := setRecordField(rec, "data", "addr", theirs)
	if err != nil {
		t.Fatal(err)
	}
	v, _, _ := getRecordField(out, "data", "addr")
	var got map[string]any
	if err := json.Unmarshal(v, &got); err != nil {
		t.Fatal(err)
	}
	if got["city"] != "LA" || got["zip"] != "90001" {
		t.Errorf("nested value not replaced atomically: %v", got)
	}
}
