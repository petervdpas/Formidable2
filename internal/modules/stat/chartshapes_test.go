package stat

import "testing"

func TestChartShapes_StableCatalog(t *testing.T) {
	got := ChartShapes()
	want := []string{"bar", "stacked", "line", "scalars"}
	if len(got) != len(want) {
		t.Fatalf("got %d shapes, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("shape[%d] = %q, want %q", i, got[i].Name, name)
		}
		if got[i].LabelKey == "" {
			t.Fatalf("shape[%d] (%s) missing label key", i, name)
		}
	}
}

func TestChartShapes_ReturnsCopy(t *testing.T) {
	// Mutating the returned slice must not poison the package catalog.
	a := ChartShapes()
	a[0].Name = "tampered"
	b := ChartShapes()
	if b[0].Name != "bar" {
		t.Fatalf("catalog mutated through returned slice: %q", b[0].Name)
	}
}
