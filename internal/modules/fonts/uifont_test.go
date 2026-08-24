package fonts

import (
	"strings"
	"testing"
)

func TestUIFonts_AlwaysOffersTheBuiltInStacks(t *testing.T) {
	list, err := NewManager(newMemFS()).UIFonts()
	if err != nil {
		t.Fatalf("UIFonts: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d built-ins, want 3: %+v", len(list), list)
	}
	if list[0].ID != UIFontSystem {
		t.Errorf("first entry = %q, want the system default so an untouched profile matches it", list[0].ID)
	}
	for _, f := range list {
		if f.Stack == "" {
			t.Errorf("%q has no CSS stack; the frontend has nothing to write", f.ID)
		}
		if f.Uploaded {
			t.Errorf("%q is a built-in, not an uploaded font", f.ID)
		}
	}
}

func TestUIFonts_AppendsUploadedFamiliesWithASystemFallback(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Brand One.woff2"] = "AB"
	fs.files["fonts/Inter.ttf"] = "CD"

	list, err := NewManager(fs).UIFonts()
	if err != nil {
		t.Fatalf("UIFonts: %v", err)
	}
	byID := map[string]UIFont{}
	for _, f := range list {
		byID[f.ID] = f
	}
	for _, want := range []string{"Brand One", "Inter"} {
		f, ok := byID[want]
		if !ok {
			t.Fatalf("uploaded family %q is not selectable: %+v", want, list)
		}
		if !f.Uploaded {
			t.Errorf("%q must be marked uploaded", want)
		}
		if !strings.HasPrefix(f.Stack, `"`+want+`",`) {
			t.Errorf("%q stack = %q, want the family quoted and first", want, f.Stack)
		}
		if !strings.Contains(f.Stack, "sans-serif") {
			t.Errorf("%q stack = %q, want the system stack behind it so a failed load stays readable", want, f.Stack)
		}
	}
}

// An uploaded font that shadows a built-in id must not produce two entries the
// picker cannot tell apart.
func TestUIFonts_DoesNotDuplicateABuiltInID(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/serif.woff2"] = "AB"

	list, err := NewManager(fs).UIFonts()
	if err != nil {
		t.Fatalf("UIFonts: %v", err)
	}
	seen := map[string]int{}
	for _, f := range list {
		seen[strings.ToLower(f.ID)]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("id %q appears %d times: %+v", id, n, list)
		}
	}
}

func TestResolveUIFont(t *testing.T) {
	fs := newMemFS()
	fs.files["fonts/Brand One.woff2"] = "AB"
	m := NewManager(fs)

	if got := m.ResolveUIFont("Brand One"); !got.Uploaded || !strings.HasPrefix(got.Stack, `"Brand One"`) {
		t.Errorf("uploaded font resolved to %+v", got)
	}
	if got := m.ResolveUIFont("serif"); got.ID != "serif" {
		t.Errorf("built-in resolved to %+v", got)
	}
	if got := m.ResolveUIFont("SERIF"); got.ID != "serif" {
		t.Errorf("id match must not be case-sensitive: %+v", got)
	}

	// A font that no longer exists (deleted file, profile from another machine)
	// must leave the app readable rather than chase a missing family.
	for _, gone := range []string{"Deleted Font", "  ", "Wingdings"} {
		got := m.ResolveUIFont(gone)
		if got.ID != UIFontSystem {
			t.Errorf("ResolveUIFont(%q) = %+v, want the system fallback", gone, got)
		}
	}
	if got := m.ResolveUIFont(UIFontSystem); got.ID != UIFontSystem || got.Stack == "" {
		t.Errorf("the default must resolve to a real stack: %+v", got)
	}
}
