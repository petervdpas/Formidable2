package app

import (
	"reflect"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func demoStatTemplate() *template.Template {
	return &template.Template{
		Facets: []template.Facet{
			{Key: "tshirt", Options: []template.FacetOption{
				{Label: "LARGE"}, {Label: "MEDIUM"}, {Label: "SMALL"},
			}},
		},
		Fields: []template.Field{
			{Key: "status", Type: "dropdown", Options: []any{
				map[string]any{"value": "open", "label": "Open"},
				map[string]any{"value": "closed", "label": "Closed"},
			}},
			{Key: "done", Type: "boolean"},
			{Key: "amount", Type: "number"},
			{Key: "due", Type: "date"},
		},
	}
}

func TestDimensionOptionLabels(t *testing.T) {
	tpl := demoStatTemplate()
	cases := []struct {
		name   string
		src    stat.SourceRef
		want   []string
		wantOK bool
	}{
		{"facet full ordered labels", stat.SourceRef{Kind: stat.SourceFacet, Key: "tshirt"}, []string{"LARGE", "MEDIUM", "SMALL"}, true},
		{"dropdown option values", stat.SourceRef{Kind: stat.SourceField, Key: "status"}, []string{"open", "closed"}, true},
		{"boolean true/false", stat.SourceRef{Kind: stat.SourceField, Key: "done"}, []string{"true", "false"}, true},
		{"number has no fixed set", stat.SourceRef{Kind: stat.SourceField, Key: "amount"}, nil, false},
		{"date has no fixed set", stat.SourceRef{Kind: stat.SourceField, Key: "due"}, nil, false},
		{"table column deferred", stat.SourceRef{Kind: stat.SourceField, Key: "status", Column: "x"}, nil, false},
		{"unknown facet", stat.SourceRef{Kind: stat.SourceFacet, Key: "ghost"}, nil, false},
		{"unknown field", stat.SourceRef{Kind: stat.SourceField, Key: "ghost"}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := dimensionOptionLabels(tpl, tc.src)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("labels = %v, want %v", got, tc.want)
			}
		})
	}
}
