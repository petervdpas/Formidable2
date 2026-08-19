package formwidget

import "testing"

// A widget row is tinted like a field row, and the same rule applies: the
// palette is data the backend owns, so a second UI can render a widget from the
// descriptor alone rather than shipping a matching stylesheet.

func TestDescriptors_CoverEveryKind(t *testing.T) {
	got := Descriptors()
	if len(got) != len(AllKinds()) {
		t.Fatalf("Descriptors returned %d, AllKinds has %d", len(got), len(AllKinds()))
	}
	seen := map[Kind]bool{}
	for _, d := range got {
		seen[d.Kind] = true
	}
	for _, k := range AllKinds() {
		if !seen[k] {
			t.Errorf("kind %q has no descriptor", k)
		}
	}
}

func TestDescriptors_StableOrder(t *testing.T) {
	a, b := Descriptors(), Descriptors()
	for i := range a {
		if a[i].Kind != b[i].Kind {
			t.Fatalf("non-deterministic order at %d: %q vs %q", i, a[i].Kind, b[i].Kind)
		}
	}
}

func TestDescriptors_EveryKindShipsItsPalette(t *testing.T) {
	for _, d := range Descriptors() {
		p := d.Palette
		if p.Bg == "" || p.Border == "" || p.Badge == "" || p.Text == "" {
			t.Errorf("kind %q ships an incomplete palette: %+v", d.Kind, p)
		}
	}
}

func TestPalette_ValuesAreHex(t *testing.T) {
	for _, d := range Descriptors() {
		for name, v := range map[string]string{
			"bg": d.Palette.Bg, "border": d.Palette.Border,
			"badge": d.Palette.Badge, "text": d.Palette.Text,
		} {
			if len(v) != 7 || v[0] != '#' {
				t.Errorf("kind %q %s = %q, want a #rrggbb value", d.Kind, name, v)
			}
		}
	}
}

// A widget kind must never collide with a field type's colour slot: the two
// vocabularies share one CSS variable namespace.
func TestDescriptors_ColorTokenMatchesTheKind(t *testing.T) {
	for _, d := range Descriptors() {
		if d.Color != string(d.Kind) {
			t.Errorf("kind %q uses colour slot %q; widget slots are named after the kind", d.Kind, d.Color)
		}
	}
}

func TestDescriptors_ReturnsDefensiveCopy(t *testing.T) {
	got := Descriptors()
	if len(got) == 0 {
		t.Fatal("empty registry")
	}
	got[0].Palette.Bg = "#000000"
	if Descriptors()[0].Palette.Bg == "#000000" {
		t.Error("registry was mutated through the returned slice")
	}
}
