package storage

import (
	"strings"
	"testing"
)

func TestSlugifyDatafileStem(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"My Great Slide", "My-Great-Slide"},
		{"  spaced  out  ", "spaced-out"},
		{"note-2026-05-05", "note-2026-05-05"},
		{"v1.2", "v1.2"},                 // interior dot kept
		{"weird@#$name!", "weirdname"},   // disallowed dropped
		{"a///b\\c", "abc"},              // separators dropped (never a path)
		{"..hidden..", "hidden"},         // leading/trailing dots trimmed
		{"a..b", "a.b"},                  // interior dot run collapsed (no "..")
		{"multi---dash", "multi-dash"},   // dash runs collapse
		{"café résumé", "caf-rsum"},      // non-ASCII dropped
		{"UPPER_case-1", "UPPER_case-1"}, // case + underscore preserved
		{"   ", ""},                      // nothing valid
		{"@@@", ""},                      // nothing valid
	}
	for _, c := range cases {
		if got := SlugifyDatafileStem(c.in); got != c.want {
			t.Errorf("SlugifyDatafileStem(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugifyDatafileStem_NeverYieldsTraversal(t *testing.T) {
	for _, in := range []string{"../escape", "a/../b", ".../x"} {
		got := SlugifyDatafileStem(in)
		if got == "" {
			continue
		}
		for _, bad := range []string{"/", "\\", ".."} {
			if strings.Contains(got, bad) {
				t.Errorf("SlugifyDatafileStem(%q) = %q leaked %q", in, got, bad)
			}
		}
	}
}
