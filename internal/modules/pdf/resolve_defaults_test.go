package pdf

import (
	"errors"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestResolveExportDefaults_EmptyFrontmatterIsPicoloomDefault(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	// No frontmatter at all - exactly the BC.01 case the user pointed at.
	rdr.md["tpl.yaml|bc01.meta.json"] = "# body without any frontmatter\n"

	got, err := m.ResolveExportDefaults("tpl.yaml", "bc01.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "" {
		t.Errorf("Theme = %q, want empty (picoloom default)", got.Theme)
	}
	if got.CoverTemplate != "" {
		t.Errorf("CoverTemplate = %q, want empty", got.CoverTemplate)
	}
}

func TestResolveExportDefaults_DocFrontmatterTheme(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: technical\n---\n# body\n"

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "technical" {
		t.Errorf("Theme = %q, want technical", got.Theme)
	}
}

func TestResolveExportDefaults_DocFrontmatterCover(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\ncover:\n  template: classic\n  title: Hi\n---\n# body\n"

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.CoverTemplate != "classic" {
		t.Errorf("CoverTemplate = %q, want classic", got.CoverTemplate)
	}
}

func TestResolveExportDefaults_ManifestSuppliesTheme(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body\n"
	m.templates = &fakeTemplateLoader{
		tpls: map[string]*template.Template{
			"tpl.yaml": {PDF: &template.PDFConfig{Style: "academic"}},
		},
	}

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "academic" {
		t.Errorf("Theme = %q, want academic (from manifest)", got.Theme)
	}
}

func TestResolveExportDefaults_DocOverridesManifest(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: legal\n---\n# body\n"
	m.templates = &fakeTemplateLoader{
		tpls: map[string]*template.Template{
			"tpl.yaml": {PDF: &template.PDFConfig{Style: "academic"}},
		},
		err: nil,
	}

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "legal" {
		t.Errorf("Theme = %q, want legal (doc beats manifest)", got.Theme)
	}
}

func TestResolveExportDefaults_MalformedFrontmatterStillResolvesManifest(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	// Missing closing --- - ParseFrontmatter returns malformed err but
	// resolve must continue to manifest layer.
	rdr.md["tpl.yaml|broken.meta.json"] = "---\nstyle: technical\n# body without closing fence\n"
	m.templates = &fakeTemplateLoader{
		tpls: map[string]*template.Template{
			"tpl.yaml": {PDF: &template.PDFConfig{Style: "creative"}},
		},
	}

	got, err := m.ResolveExportDefaults("tpl.yaml", "broken.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if got.Theme != "creative" {
		t.Errorf("Theme = %q, want creative (manifest takes over when doc is malformed)", got.Theme)
	}
}

func TestResolveExportDefaults_RendererErrorBubbles(t *testing.T) {
	m, _, rdr, _, _ := newActiveManager(t)
	rdr.err = errors.New("template not found")

	_, err := m.ResolveExportDefaults("missing.yaml", "x.meta.json")
	if err == nil {
		t.Errorf("err = nil, want renderer failure to surface")
	}
}

func TestResolveExportDefaults_InactiveStillResolves(t *testing.T) {
	// Dialog might want to preview before the engine is active. Resolve
	// is read-only metadata - no need to gate on activation.
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: invoice\n---\n# body\n"
	if err := m.Deactivate(); err != nil {
		t.Fatalf("seed Deactivate: %v", err)
	}

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("inactive ResolveExportDefaults: %v", err)
	}
	if got.Theme != "invoice" {
		t.Errorf("Theme = %q, want invoice", got.Theme)
	}
}

func TestService_ResolveExportDefaults_DelegatesToManager(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: manuscript\n---\n# body\n"

	svc := NewService(m)
	got, err := svc.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("svc.ResolveExportDefaults: %v", err)
	}
	if got.Theme != "manuscript" {
		t.Errorf("Theme = %q, want manuscript", got.Theme)
	}
}

