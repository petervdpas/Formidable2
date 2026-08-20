package connection

import (
	"regexp"
	"testing"
)

var hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func TestMethods_CoverEveryMethodTheCatalogCanHold(t *testing.T) {
	got := Methods()
	if len(got) != len(methodOrder) {
		t.Fatalf("described %d methods, want %d", len(got), len(methodOrder))
	}
	for i, m := range methodOrder {
		if got[i].Method != m {
			t.Fatalf("methods = %v, want catalog order", got)
		}
	}
}

func TestMethods_EveryBadgeIsPaintable(t *testing.T) {
	// A method with no palette renders as an unstyled word, which defeats the
	// point of the badge. The backend ships values, not just a token.
	for _, d := range Methods() {
		for name, value := range map[string]string{
			"bg": d.Palette.Bg, "border": d.Palette.Border, "text": d.Palette.Text,
		} {
			if !hexColor.MatchString(value) {
				t.Errorf("%s %s = %q, want a hex colour", d.Method, name, value)
			}
		}
	}
}
