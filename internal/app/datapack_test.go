package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/storage"
)

func TestFlattenText(t *testing.T) {
	form := &storage.Form{
		Data: map[string]any{
			"name":   "Marketing",
			"nested": map[string]any{"code": "MKT"},
			"list":   []any{"alpha", "beta"},
			"amount": 42, // non-string leaves are not indexed
		},
		Meta: storage.FormMeta{Tags: []string{"finance"}},
	}
	got := flattenText("Cost Centre", form)
	for _, want := range []string{"Cost Centre", "Marketing", "MKT", "alpha", "beta", "finance"} {
		if !strings.Contains(got, want) {
			t.Errorf("flattenText missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "42") {
		t.Errorf("non-string leaf leaked into text: %q", got)
	}
}

func TestUniqueSorted(t *testing.T) {
	got := uniqueSorted([]string{"b", "a", "b", "c", "a"})
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("uniqueSorted = %v", got)
	}
	if uniqueSorted(nil) != nil {
		t.Fatal("uniqueSorted(nil) should stay nil")
	}
}
