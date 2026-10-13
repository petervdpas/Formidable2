package template

import (
	"errors"
	"fmt"
	"strings"
)

// ErrExists is returned when CreateFromYAML would replace a template that is already there.
var ErrExists = errors.New("template: already exists")

// ErrInvalidName is returned for a template filename that is not a plain *.yaml name.
var ErrInvalidName = errors.New("template: invalid filename")

// CheckFilename accepts a plain "<name>.yaml": no folder, no leading dot, nothing that could leave the templates folder.
func CheckFilename(name string) error {
	stem := strings.TrimSuffix(name, templateExt)
	if stem == "" || stem == name || strings.HasPrefix(name, ".") ||
		strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || strings.TrimSpace(name) != name {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

// ParseYAML reads template YAML that lives outside the templates folder (a copy a plugin ships), normalized and validated the way a save would.
func ParseYAML(src []byte) (*Template, error) {
	if strings.TrimSpace(string(src)) == "" {
		return nil, errors.New("template: empty yaml")
	}
	var t Template
	if err := unmarshalYAML(src, &t); err != nil {
		return nil, fmt.Errorf("template: not valid yaml: %w", err)
	}
	if t.Fields == nil {
		t.Fields = []Field{}
	}
	Normalize(&t)
	if errs := Validate(&t); len(errs) > 0 {
		return nil, &ValidationFailedError{Errors: errs}
	}
	return &t, nil
}

// CreateFromYAML adds a template from YAML text, create-only: an existing template is the user's and is never replaced.
// It goes through SaveTemplate, so the file is written, indexed and announced exactly like one made in the editor.
func (m *Manager) CreateFromYAML(name string, src []byte) error {
	if err := CheckFilename(name); err != nil {
		return err
	}
	t, err := ParseYAML(src)
	if err != nil {
		return err
	}
	m.createMu.Lock()
	defer m.createMu.Unlock()
	if m.fs.FileExists(m.fs.JoinPath(m.templatesDir, name)) {
		return fmt.Errorf("%w: %s", ErrExists, name)
	}
	// The file name is the identity here, whatever the shipped copy calls itself.
	t.Filename = name
	return m.SaveTemplate(name, t)
}

// Exists reports whether a template file with this name is in the templates folder.
func (m *Manager) Exists(name string) bool {
	return CheckFilename(name) == nil && m.fs.FileExists(m.fs.JoinPath(m.templatesDir, name))
}

// Source returns the YAML text of a template as it is on disk.
func (m *Manager) Source(name string) (string, error) {
	if err := CheckFilename(name); err != nil {
		return "", err
	}
	full := m.fs.JoinPath(m.templatesDir, name)
	if !m.fs.FileExists(full) {
		return "", fmt.Errorf("template: file not found: %s", name)
	}
	return m.fs.LoadFile(full)
}

// ErrNotFound is returned when ReplaceFromYAML is asked to replace a template that is not there.
var ErrNotFound = errors.New("template: not found")

// ReplaceFromYAML overwrites an existing template with YAML text. It is the deliberate counterpart of CreateFromYAML:
// only for a caller acting on the user's explicit request, since it discards whatever the template held.
// It goes through SaveTemplate, so update observers see the old and the new version as they do for an edit.
func (m *Manager) ReplaceFromYAML(name string, src []byte) error {
	if err := CheckFilename(name); err != nil {
		return err
	}
	t, err := ParseYAML(src)
	if err != nil {
		return err
	}
	m.createMu.Lock()
	defer m.createMu.Unlock()
	if !m.fs.FileExists(m.fs.JoinPath(m.templatesDir, name)) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	t.Filename = name
	return m.SaveTemplate(name, t)
}

// DiffersFromYAML reports whether the stored template differs from YAML text in anything but who authored it.
// Both sides are compared in the canonical written form, so key order, spacing and defaults a save fills in do not count as a difference.
func (m *Manager) DiffersFromYAML(name string, src []byte) (bool, error) {
	if err := CheckFilename(name); err != nil {
		return false, err
	}
	other, err := ParseYAML(src)
	if err != nil {
		return false, err
	}
	stored, err := m.LoadTemplate(name)
	if err != nil {
		return false, err
	}
	a, err := canonicalText(stored, name)
	if err != nil {
		return false, err
	}
	b, err := canonicalText(other, name)
	if err != nil {
		return false, err
	}
	return a != b, nil
}

// canonicalText is a template's canonical text without the fields that say whose copy it is. The template is copied first: LoadTemplate hands out a shared pointer.
func canonicalText(t *Template, name string) (string, error) {
	c := *t
	c.Filename, c.AuthorName, c.AuthorEmail = name, "", ""
	Normalize(&c)
	out, err := marshalYAML(&c)
	return string(out), err
}
