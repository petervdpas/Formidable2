package pdf

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestExport_DisableCover_OverridesFrontmatter(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\ncover:\n  template: classic\n  title: Hi\n---\n# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{DisableCover: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS != nil {
		t.Errorf("converter got coverTS = %+v, want nil (cover disabled)", cf.coverTS)
	}
	if cf.last == nil {
		t.Fatalf("converter was not called")
	}
}

func TestExport_DisableCover_BeatsCoverTemplateOpt(t *testing.T) {
	// If the dialog ever sends both flags (it shouldn't, but defense
	// in depth), DisableCover must win.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{
		CoverTemplate: "classic",
		DisableCover:  true,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS != nil {
		t.Errorf("DisableCover did not win over CoverTemplate; coverTS = %+v", cf.coverTS)
	}
}

func TestExport_DisableCover_BeatsManifestCover(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body\n"
	enabled := true
	m.templates = &fakeTemplateLoader{
		tpls: map[string]*template.Template{
			"tpl.yaml": {
				PDF: &template.PDFConfig{
					Cover: &template.PDFCoverConfig{Template: "classic", Enabled: &enabled},
				},
			},
		},
	}

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{DisableCover: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS != nil {
		t.Errorf("DisableCover did not suppress manifest cover; coverTS = %+v", cf.coverTS)
	}
}

func TestExport_DisableTheme_OverridesFrontmatter(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: technical\n---\n# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{DisableTheme: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "" {
		t.Errorf("converter got style = %q, want empty (theme disabled)", cf.style)
	}
}

func TestExport_DisableTheme_BeatsStyleOpt(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{
		Style:        "technical",
		DisableTheme: true,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "" {
		t.Errorf("DisableTheme did not win over Style; got %q", cf.style)
	}
}

func TestExport_DisableTheme_BeatsManifestStyle(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "# body\n"
	m.templates = &fakeTemplateLoader{
		tpls: map[string]*template.Template{
			"tpl.yaml": {PDF: &template.PDFConfig{Style: "academic"}},
		},
	}

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{DisableTheme: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "" {
		t.Errorf("DisableTheme did not suppress manifest style; got %q", cf.style)
	}
}

func TestExport_FrontmatterCoverEnabledFalse_SuppressesTemplate(t *testing.T) {
	// Before this fix: BuildInput drops Input.Cover (no data) but
	// ResolveCoverTemplateSet still wires WithTemplateSet for the
	// classic template — picoloom would render an empty cover page.
	// After: cover.enabled:false in frontmatter suppresses BOTH layers.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\ncover:\n  template: classic\n  enabled: false\n  title: T\n---\n# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS != nil {
		t.Errorf("cover.enabled:false should suppress coverTS; got %+v", cf.coverTS)
	}
}

func TestExport_DialogCoverPickOverridesFrontmatterDisabled(t *testing.T) {
	// Frontmatter says cover.enabled:false, but the user explicitly
	// picks "classic" in the dialog. The dialog's pick wins.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\ncover:\n  enabled: false\n---\n# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{CoverTemplate: "classic"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS == nil {
		t.Errorf("explicit CoverTemplate pick did not re-enable cover")
	}
}

func TestResolveExportDefaults_FrontmatterCoverDisabled(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\ncover:\n  template: classic\n  enabled: false\n---\n# body\n"

	got, err := m.ResolveExportDefaults("tpl.yaml", "x.meta.json")
	if err != nil {
		t.Fatalf("ResolveExportDefaults: %v", err)
	}
	if !got.CoverDisabled {
		t.Errorf("CoverDisabled = false, want true (frontmatter said enabled:false)")
	}
	if got.CoverTemplate != "" {
		t.Errorf("CoverTemplate = %q, want empty when disabled", got.CoverTemplate)
	}
}

func TestExport_NoDisables_LegacyPathStillResolves(t *testing.T) {
	// Sanity check: without either disable flag, the existing
	// frontmatter / manifest fall-through still works exactly as
	// before. Guards against accidentally inverting the new logic.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|x.meta.json"] = "---\nstyle: legal\ncover:\n  template: classic\n  title: T\n---\n# body\n"

	_, err := m.Export("tpl.yaml", "x.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "legal" {
		t.Errorf("style = %q, want legal (frontmatter fall-through)", cf.style)
	}
	if cf.coverTS == nil {
		t.Errorf("coverTS = nil, want classic cover from frontmatter")
	}
}
