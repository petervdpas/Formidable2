package pdf

import (
	"strings"
	"testing"
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
