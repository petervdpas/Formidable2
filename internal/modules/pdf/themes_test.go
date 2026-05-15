package pdf

import "testing"

func TestService_ListThemes_ReturnsCanonicalSet(t *testing.T) {
	svc := NewService(&Manager{})
	themes := svc.ListThemes()
	if len(themes) == 0 {
		t.Fatalf("ListThemes returned empty slice")
	}
	wantContains := []string{"technical", "academic", "corporate", "legal", "invoice", "manuscript", "creative"}
	have := map[string]bool{}
	for _, th := range themes {
		have[th.Name] = true
	}
	for _, w := range wantContains {
		if !have[w] {
			t.Errorf("ListThemes missing %q; got %+v", w, themes)
		}
	}
}

func TestService_ListThemes_ReturnsCopy(t *testing.T) {
	// Caller mutation must NOT leak into the service-internal slice.
	svc := NewService(&Manager{})
	first := svc.ListThemes()
	if len(first) == 0 {
		t.Fatalf("empty")
	}
	first[0].Name = "MUTATED"

	second := svc.ListThemes()
	if second[0].Name == "MUTATED" {
		t.Errorf("ListThemes returned internal slice; caller mutation leaked")
	}
}

func TestService_ListThemes_StableOrder(t *testing.T) {
	svc := NewService(&Manager{})
	a := svc.ListThemes()
	b := svc.ListThemes()
	if len(a) != len(b) {
		t.Fatalf("len mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			t.Errorf("order differs at %d: %q vs %q", i, a[i].Name, b[i].Name)
		}
	}
}
