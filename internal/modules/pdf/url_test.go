package pdf

import "testing"

func TestImageFileURL_SimpleNameOnLinux(t *testing.T) {
	got := ImageFileURL("/storage/tpl", "foo.png")
	want := "file:///storage/tpl/images/foo.png"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImageFileURL_NameWithSpacesEncoded(t *testing.T) {
	got := ImageFileURL("/storage/tpl", "AanmeldingInschrijving - Opleiding - QuerySet.png")
	want := "file:///storage/tpl/images/AanmeldingInschrijving%20-%20Opleiding%20-%20QuerySet.png"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImageFileURL_DirWithSpacesEncoded(t *testing.T) {
	// Spaces in the storage dir itself (e.g. macOS "/Users/John Doe/…")
	// must also encode, otherwise the URL breaks at the first dir-level
	// space just as readily as at a filename-level one.
	got := ImageFileURL("/Users/John Doe/storage/tpl", "img.png")
	want := "file:///Users/John%20Doe/storage/tpl/images/img.png"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImageFileURL_OtherSpecialChars(t *testing.T) {
	// CommonMark link destinations don't allow unescaped (), <>, or
	// control chars. PathEscape via url.URL covers the markdown-hostile
	// set we care about for goldmark.
	got := ImageFileURL("/storage/tpl", "fig (final) #2.png")
	if got == "" {
		t.Fatalf("got empty URL")
	}
	// Smoke: must not contain raw space or paren as bare bytes.
	for _, c := range []rune{' ', '(', ')'} {
		for _, r := range got {
			if r == c {
				t.Errorf("unencoded %q in %q", c, got)
			}
		}
	}
}
