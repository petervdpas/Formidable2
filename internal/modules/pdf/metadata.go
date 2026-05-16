package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpulib "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDFMetadata is what we inject into a PDF's Info dictionary as a
// post-process step after picoloom renders. Chrome's CDP PrintToPDF
// only reads the document `<title>`; everything else has to be set
// directly on the PDF object via pdfcpu. Empty fields are skipped so a
// metadata pass with one populated field doesn't blank the rest.
type PDFMetadata struct {
	Title    string
	Author   string
	Subject  string
	Keywords []string
}

// HasContent reports whether the metadata struct carries anything
// worth a post-process pass. Callers use this to skip pdfcpu entirely
// when the merged frontmatter has nothing to project — pdfcpu's
// read+optimize round-trip is expensive and there's no point doing it
// for a blank metadata struct.
func (m PDFMetadata) HasContent() bool {
	if strings.TrimSpace(m.Title) != "" ||
		strings.TrimSpace(m.Author) != "" ||
		strings.TrimSpace(m.Subject) != "" {
		return true
	}
	for _, k := range m.Keywords {
		if strings.TrimSpace(k) != "" {
			return true
		}
	}
	return false
}

// InjectPDFMetadata reads in, sets the PDF Info-dictionary fields
// described by md, and returns the rewritten PDF bytes. Empty fields
// in md are skipped — they neither overwrite existing entries nor
// remove them. Keywords are added; existing keywords on the input PDF
// stay (picoloom doesn't author any, so this is a non-issue in
// practice).
//
// Returns the original bytes unchanged when md.HasContent() is false,
// so callers can unconditionally route through this function.
func InjectPDFMetadata(in []byte, md PDFMetadata) ([]byte, error) {
	if !md.HasContent() {
		return in, nil
	}
	if len(in) == 0 {
		return nil, fmt.Errorf("pdf: inject metadata: empty input")
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(in), conf)
	if err != nil {
		return nil, fmt.Errorf("pdf: inject metadata: read: %w", err)
	}

	props := map[string]string{}
	if md.Title != "" {
		props["Title"] = md.Title
	}
	if md.Author != "" {
		props["Author"] = md.Author
	}
	if md.Subject != "" {
		props["Subject"] = md.Subject
	}
	if len(props) > 0 {
		if err := pdfcpulib.PropertiesAdd(ctx, props); err != nil {
			return nil, fmt.Errorf("pdf: inject metadata: properties: %w", err)
		}
	}

	if len(md.Keywords) > 0 {
		nonEmpty := make([]string, 0, len(md.Keywords))
		for _, k := range md.Keywords {
			if t := strings.TrimSpace(k); t != "" {
				nonEmpty = append(nonEmpty, t)
			}
		}
		if len(nonEmpty) > 0 {
			if err := pdfcpulib.KeywordsAdd(ctx, nonEmpty); err != nil {
				return nil, fmt.Errorf("pdf: inject metadata: keywords: %w", err)
			}
		}
	}

	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		return nil, fmt.Errorf("pdf: inject metadata: write: %w", err)
	}
	return buf.Bytes(), nil
}

// buildPDFMetadata projects a merged Frontmatter into the subset of
// fields we push to the PDF Info dictionary. The mapping mirrors the
// old eisvogel + hypersetup.latex behaviour: Title/Author come from
// the cover (the same fields that already render onto the cover
// page), Subject defers to Cover.Description (best-effort textual
// summary), Keywords are taken verbatim from the top-level Keywords
// slice.
//
// Empty fields stay empty — the helper produces a struct that
// HasContent() can short-circuit on so the pdfcpu read+write pass is
// skipped entirely when nothing needs writing.
func buildPDFMetadata(fm Frontmatter) PDFMetadata {
	md := PDFMetadata{
		Keywords: append([]string(nil), fm.Keywords...),
	}
	if fm.Cover != nil {
		md.Title = fm.Cover.Title
		md.Author = fm.Cover.Author
		md.Subject = fm.Cover.Description
	}
	return md
}

// readPDFMetadata is a tiny test helper that reads back the Info
// dictionary fields we wrote. Lives next to the writer so the two
// stay shape-coupled; not exported because production code has no
// reason to round-trip its own output.
func readPDFMetadata(in []byte) (PDFMetadata, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(in), conf)
	if err != nil {
		return PDFMetadata{}, err
	}
	out := PDFMetadata{
		Title:   ctx.Title,
		Author:  ctx.Author,
		Subject: ctx.Subject,
	}
	for k, on := range ctx.KeywordList {
		if on {
			out.Keywords = append(out.Keywords, k)
		}
	}
	return out, nil
}

