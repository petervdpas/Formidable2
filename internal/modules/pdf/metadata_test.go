package pdf

import (
	"bytes"
	"reflect"
	"sort"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpulib "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// minimalPDF builds a blank A4 PDF in memory. Used as the input to
// metadata-injection tests so we exercise the real pdfcpu read+write
// path without depending on Chrome / picoloom.
func minimalPDF(t *testing.T) []byte {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := pdfcpulib.CreateContextWithXRefTable(conf, types.PaperSize["A4"])
	if err != nil {
		t.Fatalf("minimalPDF: CreateContextWithXRefTable: %v", err)
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatalf("minimalPDF: WriteContext: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("minimalPDF: produced empty bytes")
	}
	return buf.Bytes()
}

func TestBuildPDFMetadata(t *testing.T) {
	cases := []struct {
		name string
		fm   Frontmatter
		want PDFMetadata
	}{
		{
			"empty",
			Frontmatter{},
			PDFMetadata{},
		},
		{
			"keywords-only",
			Frontmatter{Keywords: []string{"a", "b"}},
			PDFMetadata{Keywords: []string{"a", "b"}},
		},
		{
			"cover-only",
			Frontmatter{Cover: &CoverFM{Title: "T", Author: "A", Description: "S"}},
			PDFMetadata{Title: "T", Author: "A", Subject: "S"},
		},
		{
			"all-fields",
			Frontmatter{
				Cover:    &CoverFM{Title: "T", Author: "A", Description: "S"},
				Keywords: []string{"k1"},
			},
			PDFMetadata{Title: "T", Author: "A", Subject: "S", Keywords: []string{"k1"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPDFMetadata(tc.fm)
			if got.Title != tc.want.Title || got.Author != tc.want.Author || got.Subject != tc.want.Subject {
				t.Errorf("scalars = %+v, want %+v", got, tc.want)
			}
			if !reflect.DeepEqual(got.Keywords, tc.want.Keywords) {
				t.Errorf("Keywords = %+v, want %+v", got.Keywords, tc.want.Keywords)
			}
		})
	}
}

func TestPDFMetadata_HasContent(t *testing.T) {
	cases := []struct {
		name string
		md   PDFMetadata
		want bool
	}{
		{"zero", PDFMetadata{}, false},
		{"only-empty-keyword", PDFMetadata{Keywords: []string{""}}, false},
		{"title", PDFMetadata{Title: "T"}, true},
		{"author", PDFMetadata{Author: "A"}, true},
		{"subject", PDFMetadata{Subject: "S"}, true},
		{"keywords", PDFMetadata{Keywords: []string{"k1"}}, true},
		{"keyword-with-empty-prefix", PDFMetadata{Keywords: []string{"", "k1"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.md.HasContent(); got != tc.want {
				t.Errorf("HasContent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInjectPDFMetadata_EmptyInputPassesThrough(t *testing.T) {
	// HasContent == false → no work, return input verbatim. Even when
	// input is empty/invalid.
	out, err := InjectPDFMetadata(nil, PDFMetadata{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil pass-through", out)
	}
}

func TestInjectPDFMetadata_EmptyInputErrorsWhenMetadataPresent(t *testing.T) {
	_, err := InjectPDFMetadata(nil, PDFMetadata{Title: "X"})
	if err == nil {
		t.Errorf("err = nil, want error for empty input")
	}
}

func TestInjectPDFMetadata_RoundTripTitleAuthorSubject(t *testing.T) {
	in := minimalPDF(t)
	out, err := InjectPDFMetadata(in, PDFMetadata{
		Title:   "Audit Control",
		Author:  "Team Integration Services",
		Subject: "Annual review",
	})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("Inject: empty output")
	}
	got, err := readPDFMetadata(out)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Title != "Audit Control" {
		t.Errorf("Title = %q, want %q", got.Title, "Audit Control")
	}
	if got.Author != "Team Integration Services" {
		t.Errorf("Author = %q, want %q", got.Author, "Team Integration Services")
	}
	if got.Subject != "Annual review" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Annual review")
	}
}

func TestInjectPDFMetadata_RoundTripKeywords(t *testing.T) {
	in := minimalPDF(t)
	want := []string{"Audit", "Governance", "Risk"}
	out, err := InjectPDFMetadata(in, PDFMetadata{Keywords: want})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got, err := readPDFMetadata(out)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	sort.Strings(got.Keywords)
	gotCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	if !reflect.DeepEqual(got.Keywords, gotCopy) {
		t.Errorf("Keywords = %v, want %v", got.Keywords, gotCopy)
	}
}

func TestInjectPDFMetadata_UnicodeRoundTrip(t *testing.T) {
	// PDF /Title etc are written as UTF-16BE with a BOM by pdfcpu.
	// Round-trip must preserve the Unicode codepoints — the test that
	// would catch a botched encoding.
	in := minimalPDF(t)
	out, err := InjectPDFMetadata(in, PDFMetadata{
		Title:    "Datastroom — definitie",
		Author:   "Peter — Fontys 🎓",
		Keywords: []string{"audit", "compliance", "デザイン"},
	})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got, err := readPDFMetadata(out)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Title != "Datastroom — definitie" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Author != "Peter — Fontys 🎓" {
		t.Errorf("Author = %q", got.Author)
	}
	wantKeyword := "デザイン"
	found := false
	for _, k := range got.Keywords {
		if k == wantKeyword {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Keywords missing %q, have %v", wantKeyword, got.Keywords)
	}
}

func TestInjectPDFMetadata_EmptyKeywordsFiltered(t *testing.T) {
	in := minimalPDF(t)
	// All-empty list — should NOT trigger a properties pass either.
	out, err := InjectPDFMetadata(in, PDFMetadata{Keywords: []string{"", " ", ""}})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}
	if len(out) != len(in) || !bytes.Equal(out, in) {
		// Pass-through path returns input verbatim; only the empty-
		// keyword sentinel and empty Title/Author/Subject → no work.
		// HasContent == false here.
		t.Errorf("expected pass-through; got %d bytes (input %d bytes)", len(out), len(in))
	}
}

func TestInjectPDFMetadata_GarbageInputErrors(t *testing.T) {
	// Anything that isn't a valid PDF must error rather than corrupt
	// silently.
	_, err := InjectPDFMetadata([]byte("not a pdf"), PDFMetadata{Title: "X"})
	if err == nil {
		t.Errorf("expected error on non-PDF input")
	}
}
