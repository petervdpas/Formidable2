package connection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSpecDocument_KeepsTheUploadsContent(t *testing.T) {
	m := sourceFixture(t)
	doc, err := m.SpecDocument("crm.json")
	if err != nil {
		t.Fatal(err)
	}
	// Prepared for a renderer, so escaping may differ; the document itself
	// must not. SpecSource is the verbatim copy a drift check compares.
	var got, want any
	if err := json.Unmarshal([]byte(doc.JSON), &got); err != nil {
		t.Fatalf("document is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(specV3JSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("document differs from the upload:\n%s", doc.JSON)
	}
	if doc.Format != FormatOpenAPI3 {
		t.Errorf("Format = %q, want %q", doc.Format, FormatOpenAPI3)
	}
}

func TestSpecDocument_CannotBreakOutOfAScriptTag(t *testing.T) {
	// The renderer embeds this document in a <script> block. A summary
	// containing "</script>" would otherwise close the tag early and run
	// whatever follows it with the page's privileges.
	m := NewManager(newMemFS(), nil)
	hostile := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},` +
		`"paths":{"/a":{"get":{"operationId":"a","summary":"</script><script>alert(1)</script>"}}}}`
	if _, err := m.ImportSpec("hostile", []byte(hostile)); err != nil {
		t.Fatal(err)
	}

	doc, err := m.SpecDocument("hostile.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(doc.JSON), "<script") ||
		strings.Contains(strings.ToLower(doc.JSON), "</script") {
		t.Fatalf("document can close its own script tag:\n%s", doc.JSON)
	}
	// Escaped, not dropped: the summary still reads the same once parsed.
	var probe map[string]any
	if err := json.Unmarshal([]byte(doc.JSON), &probe); err != nil {
		t.Fatalf("escaped document is not valid JSON: %v", err)
	}
	paths := probe["paths"].(map[string]any)["/a"].(map[string]any)["get"].(map[string]any)
	if paths["summary"] != "</script><script>alert(1)</script>" {
		t.Errorf("summary = %v, want the original text preserved", paths["summary"])
	}
}

func TestSpecDocument_YAMLIsConvertedForTheRenderer(t *testing.T) {
	// Swagger UI takes an object, so a YAML document has to arrive as JSON.
	m := sourceFixture(t)
	if _, err := m.ImportSpec("tiny", []byte(specV3YAML)); err != nil {
		t.Fatal(err)
	}
	doc, err := m.SpecDocument("tiny.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var probe map[string]any
	if err := json.Unmarshal([]byte(doc.JSON), &probe); err != nil {
		t.Fatalf("converted document is not valid JSON: %v", err)
	}
	if probe["openapi"] != "3.0.0" {
		t.Errorf("converted document = %v, want the original content", probe)
	}
}

func TestSpecDocument_Swagger2IsNotConvertedToV3(t *testing.T) {
	// The renderer handles 2.0 natively. Converting would show the author a
	// document they never uploaded, with operation ids that may not match.
	m := NewManager(newMemFS(), nil)
	if _, err := m.ImportSpec("legacy", []byte(specV2JSON)); err != nil {
		t.Fatal(err)
	}
	doc, err := m.SpecDocument("legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.JSON, `"swagger"`) {
		t.Fatalf("want the 2.0 document as uploaded:\n%s", doc.JSON)
	}
	if doc.Format != FormatSwagger2 {
		t.Errorf("Format = %q, want %q", doc.Format, FormatSwagger2)
	}
}

func TestSpecDocument_RefusesAPathOutsideTheSpecsFolder(t *testing.T) {
	m := sourceFixture(t)
	for _, bad := range []string{"../secrets.yaml", "sub/crm.json", ".hidden"} {
		if _, err := m.SpecDocument(bad); err == nil {
			t.Errorf("SpecDocument(%q) succeeded, want a refusal", bad)
		}
	}
}

func TestSpecDocument_RefusesADocumentThatIsNotASpec(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	if err := m.fs.SaveFile(specPath("junk.json"), "just some notes"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SpecDocument("junk.json"); err == nil {
		t.Fatal("want an error for a document that is not a spec")
	}
}

func TestSwaggerAssets_CarryTheVendoredBundle(t *testing.T) {
	// Reusing the copy Formidable already vendors, rather than a second one on
	// the frontend that could drift from it.
	a, err := SwaggerAssets()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"css":    a.CSS,
		"bundle": a.Bundle,
		"preset": a.Preset,
	} {
		if len(body) < 1000 {
			t.Errorf("%s is %d bytes, want the real asset", name, len(body))
		}
	}
	if !strings.Contains(a.Bundle, "SwaggerUIBundle") {
		t.Error("bundle does not define SwaggerUIBundle")
	}
}

func TestService_SpecDocumentReadsTheDraftsFile(t *testing.T) {
	s, _ := newService(t)
	doc, err := s.SpecDocument(Connection{ID: "crm-prod", SpecFile: "crm-prod.json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.JSON, "listCustomers") {
		t.Fatalf("document = %q, want the uploaded spec", doc.JSON)
	}
}
