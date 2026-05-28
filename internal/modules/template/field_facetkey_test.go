package template

import (
	"strings"
	"testing"
)

// Field.FacetKey is the binding from a virtual facet field to one of
// the template's declared facets. Round-trip both ways: a facet field
// with facet_key + format must survive marshal → unmarshal unchanged,
// and an empty FacetKey must NOT serialize (omitempty contract).

func TestField_FacetKey_RoundTripsThroughYAML(t *testing.T) {
	src := &Template{
		Name:     "Aanpak",
		Filename: "aanpak.yaml",
		Facets: []Facet{{
			Key:  "status",
			Icon: "fa-flag",
			Options: []FacetOption{
				{Label: "OPEN", Color: "blue"},
				{Label: "CLOSED", Color: "gray"},
			},
		}},
		Fields: []Field{
			{Key: "title", Type: "text"},
			{Key: "status_inline", Type: "facet", FacetKey: "status", Format: "radio"},
		},
	}
	raw, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "facet_key: status") {
		t.Errorf("expected facet_key in YAML output; got:\n%s", string(raw))
	}
	var got Template
	if err := unmarshalYAML(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Fields) != 2 {
		t.Fatalf("field count = %d, want 2", len(got.Fields))
	}
	f := got.Fields[1]
	if f.Type != "facet" {
		t.Errorf("type = %q, want facet", f.Type)
	}
	if f.FacetKey != "status" {
		t.Errorf("FacetKey = %q, want status", f.FacetKey)
	}
	if f.Format != "radio" {
		t.Errorf("Format = %q, want radio", f.Format)
	}
}

func TestField_FacetKey_EmptyOmitted(t *testing.T) {
	src := &Template{
		Name:     "T",
		Filename: "t.yaml",
		Fields: []Field{
			{Key: "title", Type: "text"},
		},
	}
	raw, err := marshalYAML(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "facet_key:") {
		t.Errorf("empty FacetKey must not serialize; got:\n%s", string(raw))
	}
}
