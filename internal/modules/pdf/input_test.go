package pdf

import (
	"reflect"
	"strings"
	"testing"

	picoloom "github.com/alnah/picoloom/v2"
)

// ---------- helpers ----------

func ptrBool(b bool) *bool { return &b }

// ---------- ParseFrontmatter ----------

func TestParseFrontmatter_EmptyInput(t *testing.T) {
	fm, body, err := ParseFrontmatter("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if !reflect.DeepEqual(fm, Frontmatter{}) {
		t.Errorf("fm = %+v, want zero Frontmatter", fm)
	}
}

func TestParseFrontmatter_NoLeadingDelimiter(t *testing.T) {
	md := "# Hello\n\nbody text"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if body != md {
		t.Errorf("body = %q, want input verbatim", body)
	}
	if !reflect.DeepEqual(fm, Frontmatter{}) {
		t.Errorf("fm = %+v, want zero", fm)
	}
}

func TestParseFrontmatter_ValidBlockAndBody(t *testing.T) {
	md := "---\nstyle: technical\ncover:\n  title: Hello\n  enabled: true\n---\n# Body\n"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if body != "# Body\n" {
		t.Errorf("body = %q, want %q", body, "# Body\n")
	}
	if fm.Style != "technical" {
		t.Errorf("Style = %q, want technical", fm.Style)
	}
	if fm.Cover == nil || fm.Cover.Title != "Hello" {
		t.Fatalf("Cover = %+v, want title=Hello", fm.Cover)
	}
	if fm.Cover.Enabled == nil || !*fm.Cover.Enabled {
		t.Errorf("Cover.Enabled = %v, want true", fm.Cover.Enabled)
	}
}

func TestParseFrontmatter_NoBodyAfterClose(t *testing.T) {
	md := "---\nstyle: technical\n---\n"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if fm.Style != "technical" {
		t.Errorf("Style = %q, want technical", fm.Style)
	}
}

func TestParseFrontmatter_MultipleHorizontalRulesInBody(t *testing.T) {
	md := "---\nstyle: technical\n---\nintro\n\n---\nthis is a hr in the body\n---\n"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(body, "this is a hr in the body") {
		t.Errorf("body did not preserve interior hr block: %q", body)
	}
	if fm.Style != "technical" {
		t.Errorf("Style = %q, want technical", fm.Style)
	}
}

func TestParseFrontmatter_MalformedYAML(t *testing.T) {
	md := "---\nstyle: [unterminated\n---\n# body\n"
	fm, body, err := ParseFrontmatter(md)
	if err == nil {
		t.Errorf("err = nil, want malformed-yaml error")
	}
	if !reflect.DeepEqual(fm, Frontmatter{}) {
		t.Errorf("fm = %+v, want zero on parse failure", fm)
	}
	if body != md {
		t.Errorf("body fell out of sync on parse failure: got %q", body)
	}
}

func TestParseFrontmatter_MissingClosingDelimiter(t *testing.T) {
	md := "---\nstyle: technical\n# never closed\n"
	fm, body, err := ParseFrontmatter(md)
	if err == nil {
		t.Errorf("err = nil, want missing-close error")
	}
	if !reflect.DeepEqual(fm, Frontmatter{}) {
		t.Errorf("fm = %+v, want zero on missing close", fm)
	}
	if body != md {
		t.Errorf("body = %q, want input verbatim on missing close", body)
	}
}

func TestParseFrontmatter_Keywords(t *testing.T) {
	md := "---\nkeywords:\n  - Audit\n  - Governance\n  - Risk\n---\nbody\n"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if body != "body\n" {
		t.Errorf("body = %q", body)
	}
	want := []string{"Audit", "Governance", "Risk"}
	if !reflect.DeepEqual(fm.Keywords, want) {
		t.Errorf("Keywords = %+v, want %+v", fm.Keywords, want)
	}
}

func TestParseFrontmatter_KeywordsFlowSequence(t *testing.T) {
	md := "---\nkeywords: [a, b, c]\n---\nbody\n"
	fm, _, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !reflect.DeepEqual(fm.Keywords, []string{"a", "b", "c"}) {
		t.Errorf("Keywords = %+v", fm.Keywords)
	}
}

func TestParseFrontmatter_UnknownKeysIgnored(t *testing.T) {
	md := "---\nstyle: technical\ngarbage_field: 42\nanother: [1,2,3]\n---\n# body\n"
	fm, body, err := ParseFrontmatter(md)
	if err != nil {
		t.Fatalf("err = %v, want unknown keys to be silently ignored", err)
	}
	if fm.Style != "technical" {
		t.Errorf("Style = %q, want technical", fm.Style)
	}
	if body != "# body\n" {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_TypeMismatch(t *testing.T) {
	// page.margin is float64; passing a string should fail.
	md := "---\npage:\n  margin: \"big\"\n---\nbody\n"
	fm, body, err := ParseFrontmatter(md)
	if err == nil {
		t.Errorf("err = nil, want type-mismatch error")
	}
	if !reflect.DeepEqual(fm, Frontmatter{}) {
		t.Errorf("fm = %+v, want zero on type mismatch", fm)
	}
	if body != md {
		t.Errorf("body = %q, want input verbatim on type mismatch", body)
	}
}

// ---------- Merge ----------

func TestMerge_EmptyLayers(t *testing.T) {
	got := Merge()
	if !reflect.DeepEqual(got, Frontmatter{}) {
		t.Errorf("Merge() = %+v, want zero", got)
	}
}

func TestMerge_SingleLayerEcho(t *testing.T) {
	in := Frontmatter{
		Style: "technical",
		Cover: &CoverFM{Title: "T", Enabled: ptrBool(true)},
	}
	got := Merge(in)
	if got.Style != "technical" {
		t.Errorf("Style = %q", got.Style)
	}
	if got.Cover == nil || got.Cover.Title != "T" {
		t.Errorf("Cover = %+v", got.Cover)
	}
}

func TestMerge_HigherPriorityWins(t *testing.T) {
	// Layers: frontmatter (highest), formMeta, manifest, globalConfig (lowest).
	high := Frontmatter{Style: "high"}
	low := Frontmatter{Style: "low"}
	got := Merge(high, low)
	if got.Style != "high" {
		t.Errorf("Style = %q, want high", got.Style)
	}
}

func TestMerge_EmptyHigherInheritsLower(t *testing.T) {
	high := Frontmatter{}
	low := Frontmatter{Style: "low"}
	got := Merge(high, low)
	if got.Style != "low" {
		t.Errorf("Style = %q, want low", got.Style)
	}
}

func TestMerge_ThreeLayersMiddleWinsWhereSet(t *testing.T) {
	// Top layer empty, middle sets Style, bottom sets Cover.
	top := Frontmatter{}
	mid := Frontmatter{Style: "mid"}
	bot := Frontmatter{Cover: &CoverFM{Title: "bot", Enabled: ptrBool(true)}}
	got := Merge(top, mid, bot)
	if got.Style != "mid" {
		t.Errorf("Style = %q, want mid", got.Style)
	}
	if got.Cover == nil || got.Cover.Title != "bot" {
		t.Errorf("Cover = %+v, want title=bot", got.Cover)
	}
}

func TestMerge_FieldLevelCoverCascade(t *testing.T) {
	// Higher layer sets Title only; lower layer sets Subtitle + Author.
	high := Frontmatter{Cover: &CoverFM{Title: "Hi"}}
	low := Frontmatter{Cover: &CoverFM{Subtitle: "Sub", Author: "Alice"}}
	got := Merge(high, low)
	if got.Cover == nil {
		t.Fatalf("Cover = nil")
	}
	if got.Cover.Title != "Hi" {
		t.Errorf("Title = %q, want Hi", got.Cover.Title)
	}
	if got.Cover.Subtitle != "Sub" {
		t.Errorf("Subtitle = %q, want Sub (inherited)", got.Cover.Subtitle)
	}
	if got.Cover.Author != "Alice" {
		t.Errorf("Author = %q, want Alice (inherited)", got.Cover.Author)
	}
}

func TestMerge_BoolPointerOverride(t *testing.T) {
	// Higher *false beats lower *true.
	high := Frontmatter{Cover: &CoverFM{Enabled: ptrBool(false)}}
	low := Frontmatter{Cover: &CoverFM{Enabled: ptrBool(true)}}
	got := Merge(high, low)
	if got.Cover == nil || got.Cover.Enabled == nil || *got.Cover.Enabled != false {
		t.Errorf("Enabled = %v, want explicit false", got.Cover.Enabled)
	}
}

func TestMerge_BoolPointerNilInherits(t *testing.T) {
	high := Frontmatter{Cover: &CoverFM{Title: "x"}}
	low := Frontmatter{Cover: &CoverFM{Enabled: ptrBool(true)}}
	got := Merge(high, low)
	if got.Cover == nil || got.Cover.Enabled == nil || *got.Cover.Enabled != true {
		t.Errorf("Enabled = %v, want inherited true", got.Cover.Enabled)
	}
}

func TestMerge_NilHigherSubBlockInherits(t *testing.T) {
	high := Frontmatter{Style: "s"}
	low := Frontmatter{Cover: &CoverFM{Title: "lo"}}
	got := Merge(high, low)
	if got.Cover == nil || got.Cover.Title != "lo" {
		t.Errorf("Cover = %+v, want title=lo", got.Cover)
	}
}

func TestMerge_LinksSliceAtomic(t *testing.T) {
	// Slices override atomically. Higher non-empty replaces lower.
	high := Frontmatter{Signature: &SignatureFM{Links: []LinkFM{{Label: "H", URL: "h"}}}}
	low := Frontmatter{Signature: &SignatureFM{Links: []LinkFM{{Label: "L1", URL: "l1"}, {Label: "L2", URL: "l2"}}}}
	got := Merge(high, low)
	if got.Signature == nil || len(got.Signature.Links) != 1 || got.Signature.Links[0].Label != "H" {
		t.Errorf("Links = %+v, want [{H, h}]", got.Signature.Links)
	}
}

func TestMerge_LinksSliceEmptyInherits(t *testing.T) {
	high := Frontmatter{Signature: &SignatureFM{Name: "Bob"}}
	low := Frontmatter{Signature: &SignatureFM{Links: []LinkFM{{Label: "L1", URL: "l1"}}}}
	got := Merge(high, low)
	if got.Signature == nil || len(got.Signature.Links) != 1 || got.Signature.Links[0].Label != "L1" {
		t.Errorf("Links = %+v, want [{L1, l1}] (inherited)", got.Signature.Links)
	}
}

func TestMerge_KeywordsSliceAtomic(t *testing.T) {
	// Atomic-replace mirrors Signature.Links: higher non-empty wins.
	high := Frontmatter{Keywords: []string{"H1", "H2"}}
	low := Frontmatter{Keywords: []string{"L1", "L2", "L3"}}
	got := Merge(high, low)
	if !reflect.DeepEqual(got.Keywords, []string{"H1", "H2"}) {
		t.Errorf("Keywords = %+v, want [H1 H2]", got.Keywords)
	}
}

func TestMerge_KeywordsEmptyInherits(t *testing.T) {
	high := Frontmatter{Style: "x"}
	low := Frontmatter{Keywords: []string{"A", "B"}}
	got := Merge(high, low)
	if !reflect.DeepEqual(got.Keywords, []string{"A", "B"}) {
		t.Errorf("Keywords = %+v, want [A B] inherited", got.Keywords)
	}
}

func TestMerge_PageBreaksIntCascade(t *testing.T) {
	high := Frontmatter{PageBreaks: &PageBreaksFM{Orphans: 4}}
	low := Frontmatter{PageBreaks: &PageBreaksFM{Orphans: 2, Widows: 3, BeforeH1: ptrBool(true)}}
	got := Merge(high, low)
	if got.PageBreaks == nil {
		t.Fatalf("PageBreaks = nil")
	}
	if got.PageBreaks.Orphans != 4 {
		t.Errorf("Orphans = %d, want 4", got.PageBreaks.Orphans)
	}
	if got.PageBreaks.Widows != 3 {
		t.Errorf("Widows = %d, want 3 (inherited)", got.PageBreaks.Widows)
	}
	if got.PageBreaks.BeforeH1 == nil || !*got.PageBreaks.BeforeH1 {
		t.Errorf("BeforeH1 = %v, want inherited true", got.PageBreaks.BeforeH1)
	}
}

// TestMerge_OverridePropertyEveryKey is the property-test the design doc
// calls for: every settable scalar field, set in one layer alone, must
// surface as the merge result.
func TestMerge_OverridePropertyEveryKey(t *testing.T) {
	cases := []struct {
		name  string
		layer Frontmatter
		check func(*testing.T, Frontmatter)
	}{
		{"Style", Frontmatter{Style: "X"}, func(t *testing.T, m Frontmatter) {
			if m.Style != "X" {
				t.Errorf("Style = %q", m.Style)
			}
		}},
		{"Page.Size", Frontmatter{Page: &PageFM{Size: "a4"}}, func(t *testing.T, m Frontmatter) {
			if m.Page == nil || m.Page.Size != "a4" {
				t.Errorf("Page.Size = %+v", m.Page)
			}
		}},
		{"Page.Orientation", Frontmatter{Page: &PageFM{Orientation: "landscape"}}, func(t *testing.T, m Frontmatter) {
			if m.Page == nil || m.Page.Orientation != "landscape" {
				t.Errorf("Page.Orientation = %+v", m.Page)
			}
		}},
		{"Page.Margin", Frontmatter{Page: &PageFM{Margin: 0.75}}, func(t *testing.T, m Frontmatter) {
			if m.Page == nil || m.Page.Margin != 0.75 {
				t.Errorf("Page.Margin = %+v", m.Page)
			}
		}},
		{"Cover.Title", Frontmatter{Cover: &CoverFM{Title: "T"}}, func(t *testing.T, m Frontmatter) {
			if m.Cover == nil || m.Cover.Title != "T" {
				t.Errorf("Cover.Title = %+v", m.Cover)
			}
		}},
		{"Cover.Enabled", Frontmatter{Cover: &CoverFM{Enabled: ptrBool(false)}}, func(t *testing.T, m Frontmatter) {
			if m.Cover == nil || m.Cover.Enabled == nil || *m.Cover.Enabled != false {
				t.Errorf("Cover.Enabled = %+v", m.Cover)
			}
		}},
		{"TOC.MinDepth", Frontmatter{TOC: &TOCFM{MinDepth: 2}}, func(t *testing.T, m Frontmatter) {
			if m.TOC == nil || m.TOC.MinDepth != 2 {
				t.Errorf("TOC.MinDepth = %+v", m.TOC)
			}
		}},
		{"Footer.Position", Frontmatter{Footer: &FooterFM{Position: "left"}}, func(t *testing.T, m Frontmatter) {
			if m.Footer == nil || m.Footer.Position != "left" {
				t.Errorf("Footer.Position = %+v", m.Footer)
			}
		}},
		{"Watermark.Text", Frontmatter{Watermark: &WatermarkFM{Text: "DRAFT"}}, func(t *testing.T, m Frontmatter) {
			if m.Watermark == nil || m.Watermark.Text != "DRAFT" {
				t.Errorf("Watermark.Text = %+v", m.Watermark)
			}
		}},
		{"PageBreaks.BeforeH1", Frontmatter{PageBreaks: &PageBreaksFM{BeforeH1: ptrBool(true)}}, func(t *testing.T, m Frontmatter) {
			if m.PageBreaks == nil || m.PageBreaks.BeforeH1 == nil || !*m.PageBreaks.BeforeH1 {
				t.Errorf("PageBreaks.BeforeH1 = %+v", m.PageBreaks)
			}
		}},
		{"Signature.Name", Frontmatter{Signature: &SignatureFM{Name: "Alice"}}, func(t *testing.T, m Frontmatter) {
			if m.Signature == nil || m.Signature.Name != "Alice" {
				t.Errorf("Signature.Name = %+v", m.Signature)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Single-layer: result must echo the value.
			got := Merge(tc.layer)
			tc.check(t, got)
			// Four-layer with set value in highest priority + three empty: same result.
			got = Merge(tc.layer, Frontmatter{}, Frontmatter{}, Frontmatter{})
			tc.check(t, got)
			// Four-layer with set value in LOWEST priority + three empty: still surfaces.
			got = Merge(Frontmatter{}, Frontmatter{}, Frontmatter{}, tc.layer)
			tc.check(t, got)
		})
	}
}

// ---------- BuildInput ----------

func TestBuildInput_EmptyFrontmatterMinimal(t *testing.T) {
	in := BuildInput(Frontmatter{}, "# Hello")
	if in.Markdown != "# Hello" {
		t.Errorf("Markdown = %q", in.Markdown)
	}
	if in.Cover != nil || in.TOC != nil || in.Footer != nil ||
		in.Signature != nil || in.Watermark != nil ||
		in.PageBreaks != nil || in.Page != nil {
		t.Errorf("non-nil sub-block in minimal Input: %+v", in)
	}
}

func TestBuildInput_CoverEnabledTrue(t *testing.T) {
	fm := Frontmatter{Cover: &CoverFM{
		Enabled:      ptrBool(true),
		Title:        "T",
		Subtitle:     "S",
		Author:       "Alice",
		Organization: "Org",
		Date:         "2026-05-15",
		Logo:         "./logo.png",
	}}
	in := BuildInput(fm, "body")
	if in.Cover == nil {
		t.Fatalf("Cover = nil, want non-nil")
	}
	if in.Cover.Title != "T" || in.Cover.Subtitle != "S" ||
		in.Cover.Author != "Alice" || in.Cover.Organization != "Org" ||
		in.Cover.Date != "2026-05-15" || in.Cover.Logo != "./logo.png" {
		t.Errorf("Cover = %+v", in.Cover)
	}
}

func TestBuildInput_CoverEnabledFalse(t *testing.T) {
	fm := Frontmatter{Cover: &CoverFM{Enabled: ptrBool(false), Title: "T"}}
	in := BuildInput(fm, "body")
	if in.Cover != nil {
		t.Errorf("Cover = %+v, want nil when Enabled=false", in.Cover)
	}
}

func TestBuildInput_CoverEnabledNilDefaultsToOn(t *testing.T) {
	// Block present, no explicit enabled → opt-in by presence.
	fm := Frontmatter{Cover: &CoverFM{Title: "T"}}
	in := BuildInput(fm, "body")
	if in.Cover == nil || in.Cover.Title != "T" {
		t.Errorf("Cover = %+v, want non-nil with title=T", in.Cover)
	}
}

func TestBuildInput_TOCBlock(t *testing.T) {
	fm := Frontmatter{TOC: &TOCFM{Enabled: ptrBool(true), Title: "Contents", MinDepth: 2, MaxDepth: 3}}
	in := BuildInput(fm, "body")
	if in.TOC == nil {
		t.Fatalf("TOC = nil")
	}
	if in.TOC.Title != "Contents" || in.TOC.MinDepth != 2 || in.TOC.MaxDepth != 3 {
		t.Errorf("TOC = %+v", in.TOC)
	}
}

func TestBuildInput_FooterBlock(t *testing.T) {
	fm := Frontmatter{Footer: &FooterFM{
		Enabled:        ptrBool(true),
		Position:       "right",
		ShowPageNumber: ptrBool(true),
		Text:           "© Fontys",
		DocumentID:     "DOC-001",
		Date:           "2026-05-15",
		Status:         "DRAFT",
	}}
	in := BuildInput(fm, "body")
	if in.Footer == nil {
		t.Fatalf("Footer = nil")
	}
	if in.Footer.Position != "right" || !in.Footer.ShowPageNumber ||
		in.Footer.Text != "© Fontys" || in.Footer.DocumentID != "DOC-001" ||
		in.Footer.Date != "2026-05-15" || in.Footer.Status != "DRAFT" {
		t.Errorf("Footer = %+v", in.Footer)
	}
}

func TestBuildInput_WatermarkBlock(t *testing.T) {
	fm := Frontmatter{Watermark: &WatermarkFM{
		Enabled: ptrBool(true), Text: "DRAFT", Color: "#888888", Opacity: 0.10, Angle: -45,
	}}
	in := BuildInput(fm, "body")
	if in.Watermark == nil {
		t.Fatalf("Watermark = nil")
	}
	if in.Watermark.Text != "DRAFT" || in.Watermark.Color != "#888888" ||
		in.Watermark.Opacity != 0.10 || in.Watermark.Angle != -45 {
		t.Errorf("Watermark = %+v", in.Watermark)
	}
}

func TestBuildInput_PageBlock(t *testing.T) {
	fm := Frontmatter{Page: &PageFM{Size: "a4", Orientation: "portrait", Margin: 0.75}}
	in := BuildInput(fm, "body")
	if in.Page == nil {
		t.Fatalf("Page = nil")
	}
	if in.Page.Size != "a4" || in.Page.Orientation != "portrait" || in.Page.Margin != 0.75 {
		t.Errorf("Page = %+v", in.Page)
	}
}

func TestBuildInput_PageBreaksBlock(t *testing.T) {
	fm := Frontmatter{PageBreaks: &PageBreaksFM{
		Enabled:  ptrBool(true),
		BeforeH1: ptrBool(true),
		BeforeH2: ptrBool(false),
		Orphans:  3,
		Widows:   2,
	}}
	in := BuildInput(fm, "body")
	if in.PageBreaks == nil {
		t.Fatalf("PageBreaks = nil")
	}
	if !in.PageBreaks.BeforeH1 || in.PageBreaks.BeforeH2 {
		t.Errorf("PageBreaks BeforeH1/H2 = %+v", in.PageBreaks)
	}
	if in.PageBreaks.Orphans != 3 || in.PageBreaks.Widows != 2 {
		t.Errorf("PageBreaks orphans/widows = %+v", in.PageBreaks)
	}
}

func TestBuildInput_SignatureBlock(t *testing.T) {
	fm := Frontmatter{Signature: &SignatureFM{
		Enabled: ptrBool(true), Name: "Alice", Email: "alice@example.com",
		Links: []LinkFM{{Label: "Web", URL: "https://example.com"}},
	}}
	in := BuildInput(fm, "body")
	if in.Signature == nil {
		t.Fatalf("Signature = nil")
	}
	if in.Signature.Name != "Alice" || in.Signature.Email != "alice@example.com" {
		t.Errorf("Signature = %+v", in.Signature)
	}
	if len(in.Signature.Links) != 1 || in.Signature.Links[0].Label != "Web" {
		t.Errorf("Signature.Links = %+v", in.Signature.Links)
	}
}

// TestBuildInput_PicoloomShape sanity-checks that the projected Input
// is consumable as a picoloom.Input value — the type wiring is
// intentional, not coincidental.
func TestBuildInput_PicoloomShape(t *testing.T) {
	fm := Frontmatter{Cover: &CoverFM{Title: "T"}}
	var got picoloom.Input = BuildInput(fm, "body")
	if got.Markdown != "body" {
		t.Errorf("Markdown = %q", got.Markdown)
	}
}
