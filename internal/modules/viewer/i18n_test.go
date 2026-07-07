package viewer

import (
	"sort"
	"testing"
)

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestMessagesHaveEnNlParity(t *testing.T) {
	en := Messages("en")
	nl := Messages("nl")
	if len(en) == 0 {
		t.Fatal("en messages empty")
	}
	ek, nk := keysOf(en), keysOf(nl)
	if len(ek) != len(nk) {
		t.Fatalf("key count mismatch: en=%d nl=%d", len(ek), len(nk))
	}
	for i := range ek {
		if ek[i] != nk[i] {
			t.Fatalf("key mismatch at %d: en=%q nl=%q", i, ek[i], nk[i])
		}
	}
	for k, v := range nl {
		if v == "" {
			t.Errorf("nl translation empty for %q", k)
		}
	}
}

func TestMessagesUnknownFallsBackToEnglish(t *testing.T) {
	got := Messages("zz")
	if got["settings.title"] != "Settings" {
		t.Fatalf("unknown lang did not fall back to English: %q", got["settings.title"])
	}
}

func TestResolveLanguage(t *testing.T) {
	if ResolveLanguage("nl") != "nl" {
		t.Error("explicit nl not honored")
	}
	if ResolveLanguage("en") != "en" {
		t.Error("explicit en not honored")
	}
	got := ResolveLanguage("system")
	if got != "en" && got != "nl" {
		t.Fatalf("system resolved to unsupported %q", got)
	}
}

func TestKnownKeysPresent(t *testing.T) {
	en := Messages("en")
	for _, k := range []string{"home.drop_hint", "settings.serve_http", "settings.port", "toast.open_failed"} {
		if _, ok := en[k]; !ok {
			t.Errorf("missing expected key %q", k)
		}
	}
}
