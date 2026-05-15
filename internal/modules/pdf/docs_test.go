package pdf

import (
	"strings"
	"testing"
)

func TestDirectivesDoc_English(t *testing.T) {
	got, err := directivesDoc("en")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(got, "picoloom") {
		t.Errorf("english doc missing expected content")
	}
}

func TestDirectivesDoc_Dutch(t *testing.T) {
	got, err := directivesDoc("nl")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(got, "picoloom") {
		t.Errorf("dutch doc missing expected content")
	}
}

func TestDirectivesDoc_UnknownLocaleFallsBackToEnglish(t *testing.T) {
	en, err := directivesDoc("en")
	if err != nil {
		t.Fatalf("seed en: %v", err)
	}
	fallback, err := directivesDoc("zz")
	if err != nil {
		t.Fatalf("fallback locale err = %v", err)
	}
	if fallback != en {
		t.Errorf("unknown locale did not fall back to english")
	}
}

func TestDirectivesDoc_EmptyLocaleFallsBackToEnglish(t *testing.T) {
	en, _ := directivesDoc("en")
	got, err := directivesDoc("")
	if err != nil {
		t.Fatalf("empty locale err = %v", err)
	}
	if got != en {
		t.Errorf("empty locale did not fall back to english")
	}
}
