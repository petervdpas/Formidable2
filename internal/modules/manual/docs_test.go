package manual

import (
	"strings"
	"testing"
)

func TestManualDoc_English(t *testing.T) {
	got, err := manualDoc("plugins", "en")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(got, "Plugins") {
		t.Errorf("english plugins doc missing expected content")
	}
}

func TestManualDoc_Dutch(t *testing.T) {
	got, err := manualDoc("plugins", "nl")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(strings.TrimSpace(got)) == 0 {
		t.Errorf("dutch plugins doc is empty")
	}
}

func TestManualDoc_UnknownLocaleFallsBackToEnglish(t *testing.T) {
	en, err := manualDoc("plugins", "en")
	if err != nil {
		t.Fatalf("seed en: %v", err)
	}
	fallback, err := manualDoc("plugins", "zz")
	if err != nil {
		t.Fatalf("fallback locale err = %v", err)
	}
	if fallback != en {
		t.Errorf("unknown locale did not fall back to english")
	}
}

func TestManualDoc_EmptyLocaleFallsBackToEnglish(t *testing.T) {
	en, _ := manualDoc("plugins", "en")
	got, err := manualDoc("plugins", "")
	if err != nil {
		t.Fatalf("empty locale err = %v", err)
	}
	if got != en {
		t.Errorf("empty locale did not fall back to english")
	}
}

func TestManualDoc_UnknownTopicReturnsError(t *testing.T) {
	if _, err := manualDoc("does-not-exist", "en"); err == nil {
		t.Errorf("expected error for unknown topic")
	}
}

func TestManualDoc_RejectsTraversalTopic(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "..", "plugins/../plugins", ""} {
		if _, err := manualDoc(bad, "en"); err == nil {
			t.Errorf("expected error for topic %q", bad)
		}
	}
}

func TestManualDoc_RejectsTraversalLocale(t *testing.T) {
	if _, err := manualDoc("plugins", "../en"); err == nil {
		t.Errorf("expected error for locale containing traversal")
	}
}
