package pdf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	picoloom "github.com/alnah/picoloom/v2"
)

func TestMapExportError_Codes(t *testing.T) {
	tests := []struct {
		name string
		in   error
		want ExportErrorCode
	}{
		{"nil → empty", nil, ""},

		{"engine inactive", ErrPDFNotActivated, CodeEngineInactive},
		{"engine inactive wrapped", fmt.Errorf("export: %w", ErrPDFNotActivated), CodeEngineInactive},

		{"cover not found", ErrCoverNotFound, CodeCoverTemplateInvalid},
		{"cover invalid", ErrCoverInvalid, CodeCoverTemplateInvalid},
		{"cover path invalid", ErrCoverPathInvalid, CodeCoverTemplateInvalid},
		{"picoloom cover render", picoloom.ErrCoverRender, CodeCoverTemplateInvalid},
		{"picoloom cover logo", picoloom.ErrCoverLogoNotFound, CodeCoverLogoMissing},
		{"picoloom cover logo wrapped twice",
			fmt.Errorf("a: %w", fmt.Errorf("b: %w", picoloom.ErrCoverLogoNotFound)),
			CodeCoverLogoMissing},

		{"signature image", picoloom.ErrSignatureImageNotFound, CodeSignatureImageMissing},
		{"signature render", picoloom.ErrSignatureRender, CodeSignatureImageMissing},

		{"invalid page size", picoloom.ErrInvalidPageSize, CodeDirectiveInvalid},
		{"invalid orientation", picoloom.ErrInvalidOrientation, CodeDirectiveInvalid},
		{"invalid margin", picoloom.ErrInvalidMargin, CodeDirectiveInvalid},
		{"invalid footer position", picoloom.ErrInvalidFooterPosition, CodeDirectiveInvalid},
		{"invalid watermark color", picoloom.ErrInvalidWatermarkColor, CodeDirectiveInvalid},
		{"invalid toc depth", picoloom.ErrInvalidTOCDepth, CodeDirectiveInvalid},
		{"invalid orphans", picoloom.ErrInvalidOrphans, CodeDirectiveInvalid},
		{"invalid widows", picoloom.ErrInvalidWidows, CodeDirectiveInvalid},

		{"style not found", picoloom.ErrStyleNotFound, CodeStyleNotFound},
		{"invalid asset path", picoloom.ErrInvalidAssetPath, CodeStyleNotFound},
		{"template set not found", picoloom.ErrTemplateSetNotFound, CodeStyleNotFound},
		{"incomplete template set", picoloom.ErrIncompleteTemplateSet, CodeStyleNotFound},

		{"browser connect", picoloom.ErrBrowserConnect, CodeBrowserUnreachable},
		{"page create", picoloom.ErrPageCreate, CodeBrowserUnreachable},
		{"page load", picoloom.ErrPageLoad, CodeBrowserUnreachable},

		{"deadline exceeded", context.DeadlineExceeded, CodeRenderTimeout},
		{"deadline wrapped",
			fmt.Errorf("convert: %w", context.DeadlineExceeded),
			CodeRenderTimeout},

		{"empty markdown", picoloom.ErrEmptyMarkdown, CodeEmptyMarkdown},
		{"empty pdf sentinel", errEmptyPDF, CodeEmptyMarkdown},

		{"html conversion", picoloom.ErrHTMLConversion, CodeHTMLConversionFailed},
		{"pdf generation", picoloom.ErrPDFGeneration, CodePDFGenerationFailed},

		{"save failed", &fakeSaveErr{}, CodeSaveFailed},

		{"unmapped error", errors.New("some random thing"), CodeUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MapExportError(tc.in)
			if got == nil {
				if tc.want != "" {
					t.Fatalf("MapExportError(%v) = nil, want code %q", tc.in, tc.want)
				}
				return
			}
			if got.Code != tc.want {
				t.Errorf("code = %q, want %q (err = %v)", got.Code, tc.want, tc.in)
			}
			if got.Message == "" {
				t.Errorf("message empty for %v", tc.in)
			}
		})
	}
}

func TestExportError_ErrorIsJSON(t *testing.T) {
	e := &ExportError{
		Code:    CodeCoverLogoMissing,
		Message: "logo file not found",
		Hint:    "drop the logo into pdf/covers/images/",
		Cause:   picoloom.ErrCoverLogoNotFound,
	}
	var got struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(e.Error()), &got); err != nil {
		t.Fatalf("Error() is not valid JSON: %v\n%q", err, e.Error())
	}
	if got.Code != string(CodeCoverLogoMissing) || got.Message == "" || got.Hint == "" {
		t.Errorf("envelope round-trip lost data: %+v", got)
	}
}

func TestExportError_UnwrapPreservesIs(t *testing.T) {
	e := &ExportError{
		Code:    CodeCoverLogoMissing,
		Message: "x",
		Cause:   picoloom.ErrCoverLogoNotFound,
	}
	if !errors.Is(e, picoloom.ErrCoverLogoNotFound) {
		t.Errorf("errors.Is must still match the underlying picoloom sentinel through Unwrap")
	}
}

func TestMapExportError_PreservesWrappedExportError(t *testing.T) {
	original := &ExportError{Code: CodeCoverLogoMissing, Message: "m"}
	wrapped := fmt.Errorf("outer: %w", original)
	got := MapExportError(wrapped)
	if got == nil || got.Code != CodeCoverLogoMissing {
		t.Errorf("MapExportError must surface an already-typed ExportError; got %+v", got)
	}
}

// fakeSaveErr stands in for the wrap site at render.go where SaveFile
// fails. The mapper recognizes it via the errSaveFailed sentinel that
// the production Manager.Export wraps SaveFile errors with.
type fakeSaveErr struct{}

func (f *fakeSaveErr) Error() string { return "save: disk full" }
func (f *fakeSaveErr) Unwrap() error { return errSaveFailed }
