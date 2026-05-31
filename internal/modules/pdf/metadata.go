package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpulib "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDFMetadata is injected into a PDF's Info dictionary after picoloom
// renders. Chrome's PrintToPDF only reads the document <title>, so the
// rest has to be set on the PDF via pdfcpu. Empty fields are skipped.
type PDFMetadata struct {
	Title    string
	Author   string
	Subject  string
	Keywords []string
}

// HasContent reports whether anything is worth a post-process pass, so
// callers skip pdfcpu's expensive read+optimize round-trip on a blank
// metadata struct.
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

// InjectPDFMetadata sets the Info-dictionary fields from md and returns
// the rewritten PDF. Empty fields are skipped (no overwrite, no remove).
// Returns the input unchanged when md.HasContent() is false, so callers
// can route through unconditionally.
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

// buildPDFMetadata projects a merged Frontmatter into the Info-dictionary
// subset: Title/Author from the cover, Subject from Cover.Description,
// Keywords from the top-level slice.
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

// readPDFMetadata reads back the Info-dictionary fields, for tests.
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
