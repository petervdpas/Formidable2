package api

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFieldToProperty_DiagramHasNoDataProperty(t *testing.T) {
	if key, schema := fieldToProperty(template.Field{Key: "lineage", Type: "diagram"}); key != "" || schema != nil {
		t.Fatalf("diagram stores nothing; got %q %v", key, schema)
	}
}
