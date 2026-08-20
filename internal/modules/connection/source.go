package connection

import (
	"fmt"
	"strings"
)

// The panel shows the uploaded document itself, not just what was parsed out of
// it. Reading a spec is half of authoring a binding: an operation id, a pointer
// into a response, a parameter name are all things you look up in the source.

// maxSourceBytes caps what crosses the boundary for display. A published
// document can run to megabytes, and handing one of those to the editor would
// freeze the panel rather than inform anyone.
const maxSourceBytes = 512 * 1024

// SpecSource is an uploaded document as stored, for display.
type SpecSource struct {
	File    string `json:"file"`
	Content string `json:"content"`

	// Language names the syntax to highlight, from the extension the upload
	// landed under.
	Language string `json:"language"`

	// Bytes is the document's real size, which stays honest when Content is
	// cut, so the panel can say how much is not shown.
	Bytes     int  `json:"bytes"`
	Truncated bool `json:"truncated,omitempty"`
}

// SpecSource reads one stored document. The filename is checked the same way
// every other spec reference is, so a hand-edited definition cannot read a file
// outside the specs folder.
func (m *Manager) SpecSource(specFile string) (SpecSource, error) {
	if err := checkSpecFile(specFile); err != nil {
		return SpecSource{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil {
		return SpecSource{}, fmt.Errorf("connection: no filesystem")
	}
	raw, err := m.fs.LoadFile(specPath(specFile))
	if err != nil {
		return SpecSource{}, fmt.Errorf("connection: cannot read spec %q: %w", specFile, err)
	}

	src := SpecSource{
		File:     specFile,
		Content:  raw,
		Language: sourceLanguage(specFile),
		Bytes:    len(raw),
	}
	if len(raw) > maxSourceBytes {
		src.Content = raw[:maxSourceBytes]
		src.Truncated = true
	}
	return src, nil
}

// sourceLanguage maps the stored extension onto a syntax name. The upload is
// kept verbatim under the extension it arrived with, so the name is already
// decided by then.
func sourceLanguage(file string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(file), ".json"):
		return "json"
	default:
		return "yaml"
	}
}
