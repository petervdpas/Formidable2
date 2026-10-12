package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Diagram fields are kept in the template's images folder under
// template.DiagramImageName so every render target can include them like any
// image. Storage only keeps the file; the render step draws it and hands the
// SVG over, so a referenced image always matches what was rendered.

// WriteDiagramImage stores a generated diagram image, skipping the write when
// the content is unchanged; empty svg removes the file. Only generated names
// are accepted.
func (m *Manager) WriteDiagramImage(templateFilename, name, svg string) error {
	if !template.IsDiagramImageName(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("storage: %q is not a diagram image name", name)
	}
	dir := filepath.Join(m.templateDir(templateFilename), imagesDir)
	full := filepath.Join(dir, name)
	if svg == "" {
		if !m.fs.FileExists(full) {
			return nil
		}
		return m.fs.DeleteFile(full)
	}
	if m.fs.FileExists(full) {
		if cur, err := m.fs.LoadFile(full); err == nil && cur == svg {
			return nil
		}
	}
	if err := m.fs.EnsureDirectory(dir); err != nil {
		return err
	}
	return m.fs.SaveFile(full, svg)
}

// removeDiagramImages deletes a record's diagram files, by exact name, for the
// template's current diagram fields.
func (m *Manager) removeDiagramImages(templateFilename, datafile string) error {
	var firstErr error
	for _, key := range template.DiagramFieldKeys(m.fieldsFor(templateFilename)) {
		if name := template.DiagramImageName(templateFilename, datafile, key); name != "" {
			if err := m.WriteDiagramImage(templateFilename, name, ""); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// PruneDiagramImages removes generated diagram files that belong to no current
// record + diagram field of the template (a removed field, a deleted record).
func (m *Manager) PruneDiagramImages(templateFilename string) error {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil {
		return err
	}
	if tpl == nil {
		return fmt.Errorf("storage: template %q not found", templateFilename)
	}
	dir := filepath.Join(m.templateDir(templateFilename), imagesDir)
	if !m.fs.FileExists(dir) {
		return nil
	}
	files, err := m.ListForms(templateFilename)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, datafile := range files {
		for _, key := range template.DiagramFieldKeys(tpl.Fields) {
			keep[template.DiagramImageName(templateFilename, datafile, key)] = true
		}
	}
	names, err := m.fs.ListFiles(dir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, n := range names {
		if template.IsDiagramImageName(n) && !keep[n] {
			if err := m.fs.DeleteFile(filepath.Join(dir, n)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
