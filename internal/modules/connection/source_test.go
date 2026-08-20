package connection

import (
	"strings"
	"testing"
)

func sourceFixture(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(newMemFS(), nil)
	if _, err := m.ImportSpec("crm", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSpecSource_ReturnsTheUploadVerbatim(t *testing.T) {
	m := sourceFixture(t)
	src, err := m.SpecSource("crm.json")
	if err != nil {
		t.Fatal(err)
	}
	// Byte-identical to what was uploaded: a drift check compares the stored
	// copy against the remote, and a reformat would break every comparison.
	if src.Content != specV3JSON {
		t.Fatalf("content differs from the upload:\n%s", src.Content)
	}
	if src.File != "crm.json" {
		t.Errorf("File = %q, want crm.json", src.File)
	}
	if src.Bytes != len(specV3JSON) {
		t.Errorf("Bytes = %d, want %d", src.Bytes, len(specV3JSON))
	}
}

func TestSpecSource_ReportsTheLanguageToHighlight(t *testing.T) {
	m := sourceFixture(t)

	json, err := m.SpecSource("crm.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Language != "json" {
		t.Errorf("Language = %q, want json", json.Language)
	}

	if _, err := m.ImportSpec("tiny", []byte(specV3YAML)); err != nil {
		t.Fatal(err)
	}
	yaml, err := m.SpecSource("tiny.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if yaml.Language != "yaml" {
		t.Errorf("Language = %q, want yaml", yaml.Language)
	}
}

func TestSpecSource_RefusesAPathOutsideTheSpecsFolder(t *testing.T) {
	m := sourceFixture(t)
	for _, bad := range []string{"../config.yaml", "sub/crm.json", `..\crm.json`, ".hidden", ""} {
		if _, err := m.SpecSource(bad); err == nil {
			t.Errorf("SpecSource(%q) succeeded, want a refusal", bad)
		}
	}
}

func TestSpecSource_MissingFile(t *testing.T) {
	m := sourceFixture(t)
	if _, err := m.SpecSource("nope.json"); err == nil {
		t.Fatal("want an error for a spec that is not on disk")
	}
}

func TestSpecSource_TruncatesADocumentTooBigToDisplay(t *testing.T) {
	// A multi-megabyte document would choke the editor. Showing a prefix and
	// saying so beats freezing the panel.
	m := NewManager(newMemFS(), nil)
	padding := strings.Repeat("x", maxSourceBytes)
	huge := `{"openapi":"3.0.0","info":{"title":"` + padding + `","version":"1"},"paths":{}}`
	if _, err := m.ImportSpec("huge", []byte(huge)); err != nil {
		t.Fatal(err)
	}

	src, err := m.SpecSource("huge.json")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Truncated {
		t.Fatal("Truncated = false, want the oversized document marked")
	}
	if len(src.Content) > maxSourceBytes {
		t.Errorf("content is %d bytes, want it cut to the limit", len(src.Content))
	}
	// Bytes stays the real size, so the panel can say how much is not shown.
	if src.Bytes != len(huge) {
		t.Errorf("Bytes = %d, want the full size %d", src.Bytes, len(huge))
	}
}

func TestService_SpecSourceReadsTheDraftsFile(t *testing.T) {
	s, _ := newService(t)
	src, err := s.SpecSource(Connection{ID: "crm-prod", SpecFile: "crm-prod.json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src.Content, "listCustomers") {
		t.Fatalf("content = %q, want the uploaded document", src.Content)
	}
}

func TestService_SpecSourceWithoutASpecFails(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.SpecSource(Connection{ID: "crm-prod"}); err == nil {
		t.Fatal("want an error when the client names no document")
	}
}
