package codeformatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// formatJSON re-indents src with two spaces. json.Indent rewrites whitespace
// only: key order, number spelling and string escapes come out exactly as they
// went in, so a formatted document is the same document. Malformed JSON returns
// the text untouched plus ErrMalformed.
func formatJSON(src string) (string, error) {
	if strings.TrimSpace(src) == "" {
		return src, nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(strings.TrimSpace(src)), "", "  "); err != nil {
		return src, fmt.Errorf("%w: json: %v", ErrMalformed, err)
	}
	return buf.String() + "\n", nil
}
