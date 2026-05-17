package pdf

import "testing"

func TestSchemas_AutoDerivedFromFrontmatter(t *testing.T) {
	s := Schemas()

	parents := []string{"page", "cover", "toc", "footer", "signature", "watermark", "pageBreaks"}
	for _, p := range parents {
		if _, ok := s[p]; !ok {
			t.Errorf("Schemas() missing parent %q", p)
		}
	}

	if !s["cover"]["template"] || !s["cover"]["title"] || !s["cover"]["documentID"] {
		t.Errorf("cover children incomplete: %v", s["cover"])
	}
	if !s["toc"]["minDepth"] || !s["toc"]["maxDepth"] {
		t.Errorf("toc children incomplete: %v", s["toc"])
	}
	if !s["footer"]["showPageNumber"] || !s["footer"]["position"] {
		t.Errorf("footer children incomplete: %v", s["footer"])
	}

	if _, ok := s["style"]; ok {
		t.Errorf("style is a scalar, must not appear as a parent")
	}
	if _, ok := s["keywords"]; ok {
		t.Errorf("keywords is a slice, must not appear as a parent")
	}
}
