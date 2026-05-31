package pdf

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildFrontmatter_EmptyConfig(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{})
	if err != nil {
		t.Fatalf("Build empty: %v", err)
	}
	want := "---\n---\n"
	if got != want {
		t.Errorf("empty config = %q, want %q", got, want)
	}
}

func TestBuildFrontmatter_StyleOnly(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{Style: "technical"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got, "style: technical") {
		t.Errorf("style not emitted; got:\n%s", got)
	}
	for _, junk := range []string{"page:", "cover:", "toc:", "footer:", "signature:"} {
		if strings.Contains(got, junk) {
			t.Errorf("style-only output unexpectedly contains %q:\n%s", junk, got)
		}
	}
}

func TestBuildFrontmatter_CoverBlock(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Cover: &InjectCoverConfig{
			Template: "classic",
			Title:    "My Document",
			Author:   "Team",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"cover:",
		"enabled: true",
		"template: classic",
		"title: My Document",
		"author: Team",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Optional fields the user left empty must NOT clutter the output.
	for _, junk := range []string{"organization:", "subtitle:", "documentID:"} {
		if strings.Contains(got, junk) {
			t.Errorf("empty optional field leaked: %q\n%s", junk, got)
		}
	}
}

func TestBuildFrontmatter_PageBlock(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Page: &InjectPageConfig{Size: "a4", Orientation: "portrait", Margin: 1.0},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{"page:", "size: a4", "orientation: portrait", "margin: 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildFrontmatter_TOCBlock(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		TOC: &InjectTOCConfig{Title: "Contents", MinDepth: 1, MaxDepth: 3},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{"toc:", "enabled: true", "title: Contents", "minDepth: 1", "maxDepth: 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildFrontmatter_FooterBlock_PageNumberOn(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Footer: &InjectFooterConfig{
			Position:       "center",
			ShowPageNumber: true,
			Text:           "Confidential",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"footer:",
		"enabled: true",
		"position: center",
		"showPageNumber: true",
		"text: Confidential",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildFrontmatter_FooterBlock_PageNumberOffExplicit(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Footer: &InjectFooterConfig{ShowPageNumber: false},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// False must be emitted explicitly (the *bool pointer is non-nil).
	if !strings.Contains(got, "showPageNumber: false") {
		t.Errorf("expected showPageNumber: false to be explicit:\n%s", got)
	}
}

func TestBuildFrontmatter_SignatureBlock(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Signature: &InjectSignatureConfig{
			Name:  "Peter van de Pas",
			Email: "peter@example.org",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, want := range []string{
		"signature:",
		"enabled: true",
		"name: Peter van de Pas",
		"email: peter@example.org",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildFrontmatter_Keywords(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Keywords: []string{"Audit", "Governance", "Risk"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Round-trip the body to confirm shape; the literal yaml-emit
	// form is flow- or block-sequence depending on yaml.v3 mood, so
	// we don't lock to a specific layout.
	bodyOnly := strings.TrimPrefix(strings.TrimSuffix(got, "---\n"), "---\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(bodyOnly), &fm); err != nil {
		t.Fatalf("emitted YAML not parseable: %v\n%s", err, got)
	}
	want := []string{"Audit", "Governance", "Risk"}
	if len(fm.Keywords) != len(want) {
		t.Fatalf("Keywords len = %d, want %d (%+v)", len(fm.Keywords), len(want), fm.Keywords)
	}
	for i, w := range want {
		if fm.Keywords[i] != w {
			t.Errorf("Keywords[%d] = %q, want %q", i, fm.Keywords[i], w)
		}
	}
}

func TestBuildFrontmatter_KeywordsHelperEmittedRaw(t *testing.T) {
	// A wholly-handlebars element should land at column 0 as a raw
	// line - no `- ` prefix, no single-quoting - so the helper's
	// multi-line expansion plugs into the block sequence cleanly.
	got, err := BuildFrontmatter(InjectConfig{
		Keywords: []string{`{{yamlList (fieldRaw "adapter-tags")}}`},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got, "\n{{yamlList (fieldRaw \"adapter-tags\")}}\n") {
		t.Errorf("helper not emitted at column 0:\n%s", got)
	}
	if strings.Contains(got, `'{{yamlList`) {
		t.Errorf("helper single-quoted (should be raw):\n%s", got)
	}
	if strings.Contains(got, `- {{yamlList`) || strings.Contains(got, `- '{{yamlList`) {
		t.Errorf("helper still has `- ` list-item prefix:\n%s", got)
	}
}

func TestBuildFrontmatter_KeywordsMixedLiteralAndHelper(t *testing.T) {
	// Literals stay as normal `- ITEM` lines; the helper element
	// drops to a raw line. Order is preserved.
	got, err := BuildFrontmatter(InjectConfig{
		Keywords: []string{
			"Adapter",
			`{{yamlList (fieldRaw "x")}}`,
			"Compliance",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Order check: Adapter before helper, helper before Compliance.
	iAdapter := strings.Index(got, "- Adapter")
	iHelper := strings.Index(got, "{{yamlList (fieldRaw \"x\")}}")
	iCompliance := strings.Index(got, "- Compliance")
	if iAdapter < 0 || iHelper < 0 || iCompliance < 0 {
		t.Fatalf("missing items in output:\n%s", got)
	}
	if !(iAdapter < iHelper && iHelper < iCompliance) {
		t.Errorf("order broken (Adapter=%d, helper=%d, Compliance=%d):\n%s",
			iAdapter, iHelper, iCompliance, got)
	}
}

func TestBuildFrontmatter_KeywordsHelperOnly_NoOtherBlocks(t *testing.T) {
	// A keywords-only config with a helper invocation must not collapse
	// to "---\n---\n" (the empty-frontmatter guard) - keywords ARE
	// content.
	got, err := BuildFrontmatter(InjectConfig{
		Keywords: []string{`{{yamlList (fieldRaw "x")}}`},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got == "---\n---\n" {
		t.Errorf("empty-frontmatter guard ate the keywords block")
	}
	if !strings.HasPrefix(got, "---\nkeywords:\n") {
		t.Errorf("keywords block not at top of frontmatter:\n%s", got)
	}
}

func TestBuildFrontmatter_KeywordsAtColumnZero(t *testing.T) {
	// The whole keywords block lands at column 0 so a helper's
	// multi-line expansion at render time doesn't break the parent
	// block by mixing indented + non-indented list items.
	got, err := BuildFrontmatter(InjectConfig{
		Style:    "technical",
		Keywords: []string{"Audit", "Governance"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got, "\nkeywords:\n- Audit\n- Governance\n") {
		t.Errorf("keywords block not column-0 block-sequence:\n%s", got)
	}
}

func TestBuildFrontmatter_KeywordsEmptyOmitted(t *testing.T) {
	got, _ := BuildFrontmatter(InjectConfig{Style: "x"})
	if strings.Contains(got, "keywords:") {
		t.Errorf("empty Keywords leaked into output:\n%s", got)
	}
}

func TestBuildFrontmatter_BlockOrderIsCanonical(t *testing.T) {
	got, err := BuildFrontmatter(InjectConfig{
		Style:     "academic",
		Page:      &InjectPageConfig{Size: "a4"},
		Cover:     &InjectCoverConfig{Title: "X"},
		TOC:       &InjectTOCConfig{},
		Footer:    &InjectFooterConfig{Position: "left"},
		Signature: &InjectSignatureConfig{Name: "Z"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	order := []string{"style:", "page:", "cover:", "toc:", "footer:", "signature:"}
	last := -1
	for _, k := range order {
		idx := strings.Index(got, k)
		if idx < 0 {
			t.Errorf("missing block %q in:\n%s", k, got)
			continue
		}
		if idx < last {
			t.Errorf("block order broken: %q appears before previous block. Output:\n%s", k, got)
		}
		last = idx
	}
}

func TestBuildFrontmatter_DeterministicOutput(t *testing.T) {
	cfg := InjectConfig{
		Style: "technical",
		Cover: &InjectCoverConfig{Title: "X", Author: "Y"},
		Page:  &InjectPageConfig{Size: "a4"},
	}
	a, _ := BuildFrontmatter(cfg)
	b, _ := BuildFrontmatter(cfg)
	if a != b {
		t.Errorf("non-deterministic output:\na = %q\nb = %q", a, b)
	}
}

func TestBuildFrontmatter_HasOuterFences(t *testing.T) {
	got, _ := BuildFrontmatter(InjectConfig{Style: "x"})
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("missing leading fence: %q", got)
	}
	if !strings.HasSuffix(got, "---\n") {
		t.Errorf("missing trailing fence: %q", got)
	}
}

// ---------- enum registry tests ----------

func TestService_ListPageSizes_CanonicalSet(t *testing.T) {
	svc := &Service{}
	got := svc.ListPageSizes()
	want := []string{"a4", "letter", "legal"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestService_ListPageOrientations_CanonicalSet(t *testing.T) {
	svc := &Service{}
	got := svc.ListPageOrientations()
	want := []string{"portrait", "landscape"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestService_ListFooterPositions_CanonicalSet(t *testing.T) {
	svc := &Service{}
	got := svc.ListFooterPositions()
	want := []string{"left", "center", "right"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestService_RegistriesReturnCopies(t *testing.T) {
	svc := &Service{}
	for _, c := range []struct {
		name string
		f    func() string
	}{
		{"page_sizes", func() string {
			a := svc.ListPageSizes()
			a[0].Name = "MUTATED"
			b := svc.ListPageSizes()
			return b[0].Name
		}},
		{"orientations", func() string {
			a := svc.ListPageOrientations()
			a[0].Name = "MUTATED"
			b := svc.ListPageOrientations()
			return b[0].Name
		}},
		{"footer_positions", func() string {
			a := svc.ListFooterPositions()
			a[0].Name = "MUTATED"
			b := svc.ListFooterPositions()
			return b[0].Name
		}},
	} {
		if got := c.f(); got == "MUTATED" {
			t.Errorf("%s: caller mutation leaked", c.name)
		}
	}
}

// ---------- realistic-input coverage ----------

func TestBuildFrontmatter_AllFieldsPopulated(t *testing.T) {
	// Mirrors what a real user filling every field in the Inject
	// wizard would produce - full cover, page, toc, footer, signature
	// blocks all populated. Catches block-emit ordering issues and
	// missing-field bugs that single-block tests miss.
	got, err := BuildFrontmatter(InjectConfig{
		Style: "technical",
		Page:  &InjectPageConfig{Size: "a4", Orientation: "portrait", Margin: 1.0},
		Cover: &InjectCoverConfig{
			Template:     "classic",
			Title:        "Audit Control",
			Subtitle:     "QZM 2026",
			Author:       "Team Integration Services",
			AuthorTitle:  "Architect",
			Organization: "Northwind",
			Date:         "2026-05-16",
			Version:      "1.0",
			ClientName:   "Hogeschool",
			ProjectName:  "QZM",
			DocumentType: "Audit Report",
			DocumentID:   "AC-001",
			Description:  "Annual review.",
			Department:   "ICT",
			Logo:         "formidable.svg",
		},
		TOC: &InjectTOCConfig{Title: "Inhoudsopgave", MinDepth: 1, MaxDepth: 3},
		Footer: &InjectFooterConfig{
			Position:       "center",
			ShowPageNumber: true,
			Text:           "Confidential",
			Date:           "2026-05-16",
			Status:         "Final",
			DocumentID:     "AC-001",
		},
		Signature: &InjectSignatureConfig{
			Name:         "Peter van de Pas",
			Title:        "Architect",
			Email:        "peter@example.org",
			Organization: "Northwind",
			ImagePath:    "/home/peter/sig.png",
			Phone:        "+31 0000000",
			Address:      "Eindhoven",
			Department:   "ICT",
		},
	})
	if err != nil {
		t.Fatalf("Build all-fields: %v", err)
	}
	// Sample assertions across every block - if any go missing we
	// know the emit pass dropped a section.
	mustContain := []string{
		"style: technical",
		"page:", "size: a4", "orientation: portrait",
		"cover:", "template: classic", "title: Audit Control",
		"organization: Northwind", "logo: formidable.svg",
		"toc:", "title: Inhoudsopgave", "maxDepth: 3",
		"footer:", "position: center", "showPageNumber: true",
		"signature:", "name: Peter van de Pas", "email: peter@example.org",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in all-fields output:\n%s", s, got)
		}
	}
}

func TestBuildFrontmatter_HandlebarsInValuesRoundTrip(t *testing.T) {
	// Realistic: user copies a Handlebars-rendering value out of an
	// existing template into the wizard's Title field. The dialog
	// passes it through verbatim, the YAML emitter must NOT mangle
	// it. Today this means yaml.v3 emits it as an unquoted scalar
	// (mid-value `{` is fine in unquoted scalars).
	got, err := BuildFrontmatter(InjectConfig{
		Cover: &InjectCoverConfig{
			Title: `Audit Control {{field "id"}}`,
		},
	})
	if err != nil {
		t.Fatalf("Build handlebars: %v", err)
	}
	if !strings.Contains(got, `title: 'Audit Control {{field "id"}}'`) &&
		!strings.Contains(got, `title: Audit Control {{field "id"}}`) {
		t.Errorf("Handlebars not preserved verbatim:\n%s", got)
	}
}

func TestBuildFrontmatter_YAMLSpecialCharsInTitleQuoted(t *testing.T) {
	// `Title: Foo: Bar` - embedded colon. yaml.v3 must quote on emit
	// so the round-trip parse doesn't see it as a nested key.
	got, err := BuildFrontmatter(InjectConfig{
		Cover: &InjectCoverConfig{Title: "Foo: Bar"},
	})
	if err != nil {
		t.Fatalf("Build colon-title: %v", err)
	}
	// Just verify yaml.Marshal handled it - re-parsing the body
	// (sans fences) must yield Foo: Bar verbatim.
	if !strings.Contains(got, "Foo: Bar") {
		t.Errorf("colon title lost:\n%s", got)
	}
	// Sanity: re-parse the emitted YAML body and verify cover.title.
	bodyOnly := strings.TrimPrefix(strings.TrimSuffix(got, "---\n"), "---\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(bodyOnly), &fm); err != nil {
		t.Fatalf("emitted YAML is not valid: %v\n%s", err, got)
	}
	if fm.Cover == nil || fm.Cover.Title != "Foo: Bar" {
		t.Errorf("round-trip title = %v, want \"Foo: Bar\"", fm.Cover)
	}
}

func TestBuildFrontmatter_TitleWithBracketsAndBraces(t *testing.T) {
	// Pathological scalar - flow-sequence + flow-mapping chars.
	got, err := BuildFrontmatter(InjectConfig{
		Cover: &InjectCoverConfig{Title: "[draft] {classified}"},
	})
	if err != nil {
		t.Fatalf("Build brackets: %v", err)
	}
	bodyOnly := strings.TrimPrefix(strings.TrimSuffix(got, "---\n"), "---\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(bodyOnly), &fm); err != nil {
		t.Fatalf("emitted YAML not parseable: %v\n%s", err, got)
	}
	if fm.Cover == nil || fm.Cover.Title != "[draft] {classified}" {
		t.Errorf("round-trip title = %q, want %q",
			fmCoverTitle(fm), "[draft] {classified}")
	}
}

func TestBuildFrontmatter_UnicodeValues(t *testing.T) {
	// Multilingual content - Dutch dashes + Japanese + emoji - must
	// round-trip. yaml.v3 escapes some codepoints (e.g. emoji as
	// \UXXXXXXXX) on emit; the contract is "parse recovers the
	// original", not "literal bytes in the output".
	got, err := BuildFrontmatter(InjectConfig{
		Cover: &InjectCoverConfig{
			Title:    "Datastroom - definitie",
			Subtitle: "デザイン文書",
			Author:   "Peter - Northwind 🎓",
		},
	})
	if err != nil {
		t.Fatalf("Build unicode: %v", err)
	}
	bodyOnly := strings.TrimPrefix(strings.TrimSuffix(got, "---\n"), "---\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(bodyOnly), &fm); err != nil {
		t.Fatalf("emitted YAML not parseable: %v\n%s", err, got)
	}
	if fm.Cover == nil {
		t.Fatalf("Cover nil after round-trip")
	}
	if fm.Cover.Title != "Datastroom - definitie" {
		t.Errorf("Title round-trip lost: %q", fm.Cover.Title)
	}
	if fm.Cover.Subtitle != "デザイン文書" {
		t.Errorf("Subtitle round-trip lost: %q", fm.Cover.Subtitle)
	}
	if fm.Cover.Author != "Peter - Northwind 🎓" {
		t.Errorf("Author round-trip lost: %q", fm.Cover.Author)
	}
}

func fmCoverTitle(fm Frontmatter) string {
	if fm.Cover == nil {
		return ""
	}
	return fm.Cover.Title
}

func TestService_BuildFrontmatter_DelegatesToManager(t *testing.T) {
	svc := &Service{}
	got, err := svc.BuildFrontmatter(InjectConfig{Style: "delegated"})
	if err != nil {
		t.Fatalf("svc.Build: %v", err)
	}
	if !strings.Contains(got, "style: delegated") {
		t.Errorf("missing style in service delegate output:\n%s", got)
	}
}
