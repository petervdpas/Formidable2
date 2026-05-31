package pdf

import (
	"context"
	"encoding/json"
	"errors"

	picoloom "github.com/alnah/picoloom/v2"
)

type ExportErrorCode string

const (
	CodeEngineInactive        ExportErrorCode = "engine_inactive"
	CodeRenderFailed          ExportErrorCode = "render_failed"
	CodeCoverLogoMissing      ExportErrorCode = "cover_logo_missing"
	CodeCoverTemplateInvalid  ExportErrorCode = "cover_template_invalid"
	CodeSignatureImageMissing ExportErrorCode = "signature_image_missing"
	CodeDirectiveInvalid      ExportErrorCode = "directive_invalid"
	CodeStyleNotFound         ExportErrorCode = "style_not_found"
	CodeBrowserUnreachable    ExportErrorCode = "browser_unreachable"
	CodeRenderTimeout         ExportErrorCode = "render_timeout"
	CodeEmptyMarkdown         ExportErrorCode = "empty_markdown"
	CodeHTMLConversionFailed  ExportErrorCode = "html_conversion_failed"
	CodePDFGenerationFailed   ExportErrorCode = "pdf_generation_failed"
	CodeSaveFailed            ExportErrorCode = "save_failed"
	CodeUnknown               ExportErrorCode = "unknown"
)

// errEmptyPDF and errSaveFailed are internal sentinels so
// MapExportError can recognise the wrap site without string-matching.
var (
	errEmptyPDF   = errors.New("pdf: converter returned empty PDF")
	errSaveFailed = errors.New("pdf: save failed")
)

// ExportError is the typed envelope Service.ExportPDF surfaces. Its
// Error() string is JSON so {code, message, hint} survives the Wails
// boundary for both the frontend and a future Lua host to parse.
type ExportError struct {
	Code    ExportErrorCode `json:"code"`
	Message string          `json:"message"`
	Hint    string          `json:"hint,omitempty"`
	Cause   error           `json:"-"`
}

func (e *ExportError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return string(e.Code) + ": " + e.Message
	}
	return string(b)
}

func (e *ExportError) Unwrap() error { return e.Cause }

// MapExportError converts any Export-pipeline error into a typed
// ExportError (nil iff err is nil; already-typed errors pass through).
// Mapping order matters: more-specific sentinels are checked first so
// e.g. ErrCoverLogoNotFound never falls into the ErrCoverRender bucket.
func MapExportError(err error) *ExportError {
	if err == nil {
		return nil
	}

	var ee *ExportError
	if errors.As(err, &ee) {
		return ee
	}

	switch {
	case errors.Is(err, ErrPDFNotActivated):
		return newExportError(CodeEngineInactive, err,
			"PDF engine is not active. Activate it on the Information page.")

	case errors.Is(err, picoloom.ErrCoverLogoNotFound):
		return newExportError(CodeCoverLogoMissing, err,
			"Drop the logo into <AppRoot>/pdf/covers/images/ or fix the cover.logo path.")

	case errors.Is(err, ErrCoverNotFound),
		errors.Is(err, ErrCoverInvalid),
		errors.Is(err, ErrCoverPathInvalid),
		errors.Is(err, picoloom.ErrCoverRender):
		return newExportError(CodeCoverTemplateInvalid, err,
			"Open the cover and re-run validation; check the magic-line header.")

	case errors.Is(err, picoloom.ErrSignatureImageNotFound),
		errors.Is(err, picoloom.ErrSignatureRender):
		return newExportError(CodeSignatureImageMissing, err,
			"Fix the signature.image path or remove the signature block.")

	case errors.Is(err, picoloom.ErrInvalidPageSize),
		errors.Is(err, picoloom.ErrInvalidOrientation),
		errors.Is(err, picoloom.ErrInvalidMargin),
		errors.Is(err, picoloom.ErrInvalidFooterPosition),
		errors.Is(err, picoloom.ErrInvalidWatermarkColor),
		errors.Is(err, picoloom.ErrInvalidTOCDepth),
		errors.Is(err, picoloom.ErrInvalidOrphans),
		errors.Is(err, picoloom.ErrInvalidWidows):
		return newExportError(CodeDirectiveInvalid, err,
			"A frontmatter directive has an invalid value; see the directive reference.")

	case errors.Is(err, picoloom.ErrStyleNotFound),
		errors.Is(err, picoloom.ErrInvalidAssetPath),
		errors.Is(err, picoloom.ErrTemplateSetNotFound),
		errors.Is(err, picoloom.ErrIncompleteTemplateSet):
		return newExportError(CodeStyleNotFound, err,
			"Pick a different theme, or check the custom CSS path.")

	case errors.Is(err, picoloom.ErrBrowserConnect),
		errors.Is(err, picoloom.ErrPageCreate),
		errors.Is(err, picoloom.ErrPageLoad):
		return newExportError(CodeBrowserUnreachable, err,
			"Re-probe Chrome on the Information page; reinstall if the binary moved.")

	case errors.Is(err, context.DeadlineExceeded):
		return newExportError(CodeRenderTimeout, err,
			"Rendering took too long. Try a smaller document or split into parts.")

	case errors.Is(err, picoloom.ErrEmptyMarkdown),
		errors.Is(err, errEmptyPDF):
		return newExportError(CodeEmptyMarkdown, err,
			"The document body is empty after frontmatter.")

	case errors.Is(err, picoloom.ErrHTMLConversion):
		return newExportError(CodeHTMLConversionFailed, err,
			"Markdown could not be converted to HTML; check the document for malformed syntax.")

	case errors.Is(err, picoloom.ErrPDFGeneration):
		return newExportError(CodePDFGenerationFailed, err,
			"Chrome failed to produce the PDF; see Information → Logging for details.")

	case errors.Is(err, errSaveFailed):
		return newExportError(CodeSaveFailed, err,
			"Could not write the PDF to disk. Check the output folder and free space.")
	}

	return newExportError(CodeUnknown, err, "")
}

func newExportError(code ExportErrorCode, cause error, hint string) *ExportError {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	return &ExportError{Code: code, Message: msg, Hint: hint, Cause: cause}
}
